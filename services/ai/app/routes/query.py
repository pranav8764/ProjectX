import re
import uuid
import logging
from typing import Any, Dict

import app.database as database
import app.utils.ai_clients as ai_clients
from app.database import document_access_select, get_document_access_columns
from app.schemas import CopilotQueryRequest
from app.utils.helpers import (
    answer_has_citation,
    document_row_access_allowed,
    extract_asset_tags,
    graceful_query_response,
    normalize_filter_values,
    optional_uuid,
    query_looks_unsafe,
    resolve_request_role,
)
from fastapi import APIRouter

logger = logging.getLogger(__name__)

router = APIRouter()


@router.post("/query")
async def rag_query(request: CopilotQueryRequest):
    """Executes stateful LangGraph Copilot workflow incorporating memory, routing, and guardrails"""
    if not database.db_pool:
        return graceful_query_response(request.question, "Database connection unavailable")

    try:
        plant_uuid = optional_uuid(request.plantId)
        if not plant_uuid:
            return graceful_query_response(request.question, "Invalid or missing plantId")

        # Resolve org_id
        async with database.db_pool.acquire() as conn:
            org_row = await conn.fetchrow(
                "SELECT organization_id FROM identity.plants WHERE id = $1",
                plant_uuid
            )
            if not org_row:
                return graceful_query_response(request.question, "Plant not found")
            org_id = optional_uuid(request.organizationId) or org_row["organization_id"]

        # LangGraph Threading (Short-Term Memory Session Key)
        # Use userId, filters thread ID hint, or dynamic fallback
        thread_id = str(request.userId or uuid.uuid4())
        filters = request.filters or {}
        if isinstance(filters, dict) and filters.get("threadId"):
            thread_id = str(filters["threadId"])
            
        config = {"configurable": {"thread_id": thread_id}}

        # Initialize input state
        from langchain_core.messages import HumanMessage
        inputs = {
            "messages": [HumanMessage(content=request.question)],
            "plant_id": str(plant_uuid),
            "organization_id": str(org_id),
            "user_id": request.userId,
            "user_role": request.userRole or "viewer",
            "filters": filters,
            "retrieved_chunks": [],
            "rbac_allowed_chunks": [],
            "missing_info": [],
            "validation_attempts": 0,
            "validation_errors": []
        }

        # Use the graph compiled once at startup; compile lazily only if startup was skipped.
        import app.graph as graph_module
        graph = graph_module.copilot_graph
        if graph is None:
            graph = await graph_module.init_copilot_graph()

        state_result = await graph.ainvoke(inputs, config=config)

        # Extract final outputs
        answer = state_result.get("answer") or "Not enough evidence is available to answer safely right now."
        confidence = state_result.get("confidence") if state_result.get("confidence") is not None else 0.0
        citations = state_result.get("citations") or []
        missing_info = state_result.get("missing_info") or []
        related_assets = extract_asset_tags(request.question)

        # Log query history and citations to RAG schema tables
        query_uuid = uuid.uuid4()
        async with database.db_pool.acquire() as conn:
            await conn.execute("""
                INSERT INTO rag.queries (id, organization_id, plant_id, user_id, query_text, answer_text, confidence)
                VALUES ($1, $2, $3, $4, $5, $6, $7)
            """, query_uuid, org_id, plant_uuid, optional_uuid(request.userId), request.question, answer, confidence)

            for citation in citations:
                try:
                    await conn.execute("""
                        INSERT INTO rag.citations (query_id, document_id, page_no, quoted_text)
                        VALUES ($1, $2, $3, $4)
                    """, query_uuid, uuid.UUID(citation["documentId"]), citation["page"], citation["snippet"])
                except Exception as cit_err:
                    logger.warning(f"Failed to save query citation to DB: {cit_err}")

        return {
            "answer": answer,
            "confidence": confidence,
            "citations": citations,
            "relatedAssets": related_assets,
            "missingInfo": missing_info
        }

    except Exception as e:
        logger.error(f"Error executing LangGraph Copilot workflow: {e}", exc_info=True)
        return graceful_query_response(request.question, str(e))
