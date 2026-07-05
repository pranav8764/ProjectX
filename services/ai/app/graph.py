import re
import uuid
import logging
from typing import Dict, Any, List, Optional
from langgraph.graph import StateGraph, START, END
from langgraph.checkpoint.memory import MemorySaver

from app.state import CopilotState
import app.database as database
from app.database import get_document_access_columns, document_access_select
import app.utils.ai_clients as ai_clients
from app.utils.helpers import (
    optional_uuid,
    normalize_role_name,
    document_row_access_allowed,
    query_looks_unsafe,
    answer_has_citation,
    normalize_filter_values
)

logger = logging.getLogger("plantbrain-ai.graph")

# ==========================================
# GRAPH NODES IMPLEMENTATION
# ==========================================

async def guardrail_node(state: CopilotState) -> Dict[str, Any]:
    """Inspects query for unsafe keywords and short-circuits execution if flagged"""
    question = state["messages"][-1].content if state.get("messages") else ""
    if query_looks_unsafe(question):
        logger.warning(f"Unsafe query detected: '{question}'")
        return {
            "answer": "I cannot provide unsafe operational bypasses or legal/compliance certification. Review the authorized SOP and have a qualified supervisor approve any critical action.",
            "confidence": 0.0,
            "citations": [],
            "missing_info": ["Query requested unsafe, unsupported, or certification-like guidance."],
            "validation_errors": []
        }
    return {}

def route_after_guardrail(state: CopilotState) -> str:
    """Routes to END if query is flagged unsafe, otherwise to router"""
    if state.get("answer"):
        return END
    return "router"

async def intent_router_node(state: CopilotState) -> Dict[str, Any]:
    """Classifies user intent to decide whether document retrieval is required"""
    question = state["messages"][-1].content if state.get("messages") else ""
    
    # Simple history compilation
    history_str = ""
    for msg in state.get("messages", [])[:-1]:
        role = "User" if msg.type == "human" else "AI"
        history_str += f"{role}: {msg.content}\n"
        
    prompt = f"""
Classify the user's latest input into one of two categories:
- "RAG_QUERY": The user is asking a technical question about assets, maintenance, SOPs, operations, guidelines, or manuals.
- "GENERAL": The user is greeting, saying thanks, asking general conversational questions, or chat-related queries.

Chat history:
{history_str}
User latest input: "{question}"

Output ONLY the category name: either "RAG_QUERY" or "GENERAL". Do not include any other text, quotes, or markdown.
"""
    intent_output = await ai_clients.generate_text_llm(prompt)
    intent = intent_output.strip().upper()
    
    resolved_intent = "GENERAL" if intent == "GENERAL" else "RAG_QUERY"
    logger.info(f"Classified query intent as: {resolved_intent}")
    return {"query_intent": resolved_intent}

def route_by_intent(state: CopilotState) -> str:
    """Conditional edge routing based on classified query intent"""
    if state.get("query_intent") == "RAG_QUERY":
        return "retrieve"
    return "generate"

async def retrieval_node(state: CopilotState) -> Dict[str, Any]:
    """Performs hybrid vector pgvector search and keyword matching on PostgreSQL"""
    question = state["messages"][-1].content if state.get("messages") else ""
    plant_uuid = optional_uuid(state.get("plant_id"))
    org_id = optional_uuid(state.get("organization_id"))
    
    if not database.db_pool:
        logger.error("Database pool is unavailable for retrieval_node")
        return {"retrieved_chunks": []}
        
    if not plant_uuid:
        return {"retrieved_chunks": []}
        
    # Generate 1536-dimensional query embedding
    query_embedding = await ai_clients.get_gemini_embedding_1536(question)
    
    filters = state.get("filters") or {}
    document_type_filters = normalize_filter_values(filters.get("documentTypes") or filters.get("documentType"))
    date_from = filters.get("dateFrom")
    date_to = filters.get("dateTo")
    
    async with database.db_pool.acquire() as conn:
        # Resolve plant org context if not set
        if not org_id:
            org_row = await conn.fetchrow("SELECT organization_id FROM identity.plants WHERE id = $1", plant_uuid)
            org_id = org_row["organization_id"] if org_row else None
            
        access_columns = await get_document_access_columns(conn)
        access_select = document_access_select(access_columns)
        
        # 1. Retrieve Candidate Chunks by Cosine Distance
        chunks_rows = await conn.fetch(f"""
            SELECT c.id, c.document_id, c.page_no, c.chunk_text, d.title,
                   {access_select},
                   (c.embedding <=> $1) as distance
            FROM ingestion.document_chunks c
            JOIN document.documents d ON c.document_id = d.id
            WHERE d.plant_id = $2
              AND d.organization_id = $3
              AND d.status <> 'ARCHIVED'
              AND c.document_version_id = d.current_version_id
              AND (cardinality($4::text[]) = 0 OR d.document_type = ANY($4::text[]))
              AND ($5::timestamptz IS NULL OR d.created_at >= $5::timestamptz)
              AND ($6::timestamptz IS NULL OR d.created_at <= $6::timestamptz)
            ORDER BY distance ASC
            LIMIT 6
        """, query_embedding, plant_uuid, org_id, document_type_filters, date_from, date_to)
        
        # 2. Keyword exact tag search fallback using single optimized unnest query
        keyword_chunks = []
        extracted_tags = re.findall(r"\b[A-Z]+[-\s]*\d+[A-Z]*\b", question.upper())
        asset_filter = filters.get("assetTag")
        if asset_filter:
            extracted_tags.append(asset_filter.upper())
            
        if extracted_tags:
            tag_wildcards = [f"%{tag}%" for tag in extracted_tags]
            k_rows = await conn.fetch(f"""
                SELECT c.id, c.document_id, c.page_no, c.chunk_text, d.title,
                       {access_select},
                       0.0 as distance
                FROM ingestion.document_chunks c
                JOIN document.documents d ON c.document_id = d.id
                WHERE d.plant_id = $1
                  AND d.organization_id = $3
                  AND d.status <> 'ARCHIVED'
                  AND c.document_version_id = d.current_version_id
                  AND (cardinality($4::text[]) = 0 OR d.document_type = ANY($4::text[]))
                  AND ($5::timestamptz IS NULL OR d.created_at >= $5::timestamptz)
                  AND ($6::timestamptz IS NULL OR d.created_at <= $6::timestamptz)
                  AND (
                    EXISTS (
                      SELECT 1 FROM unnest($2::text[]) AS val WHERE c.chunk_text ILIKE val OR d.title ILIKE val
                    )
                  )
                LIMIT 6
            """, plant_uuid, tag_wildcards, org_id, document_type_filters, date_from, date_to)
            keyword_chunks.extend(k_rows)
            
        # Combine searches
        seen = set()
        retrieved = []
        for r in (keyword_chunks + list(chunks_rows)):
            if r["id"] not in seen:
                seen.add(r["id"])
                retrieved.append(dict(r))
                
    return {"retrieved_chunks": retrieved}

async def rbac_node(state: CopilotState) -> Dict[str, Any]:
    """Filters retrieved chunks using Role-Based Access Control configuration"""
    user_role = state.get("user_role") or "viewer"
    plant_uuid = optional_uuid(state.get("plant_id"))
    org_uuid = optional_uuid(state.get("organization_id"))
    user_uuid = optional_uuid(state.get("user_id"))
    
    role_to_use = user_role
    if user_uuid and database.db_pool:
        try:
            async with database.db_pool.acquire() as conn:
                role_row = await conn.fetchrow("""
                    SELECT r.name AS role
                    FROM identity.memberships m
                    JOIN identity.roles r ON m.role_id = r.id
                    WHERE m.user_id = $1
                      AND m.organization_id = $2
                      AND (m.plant_id IS NULL OR m.plant_id = $3)
                    ORDER BY CASE WHEN m.plant_id = $3 THEN 0 ELSE 1 END
                    LIMIT 1
                """, user_uuid, org_uuid, plant_uuid)
                if role_row and role_row["role"]:
                    role_to_use = normalize_role_name(role_row["role"])
        except Exception as e:
            logger.warning(f"Unable to resolve user role for RAG graph node filtering: {e}")
            
    allowed_chunks = []
    restricted_matches = 0
    for chunk in state.get("retrieved_chunks", []):
        if document_row_access_allowed(chunk, role_to_use):
            allowed_chunks.append(chunk)
        else:
            restricted_matches += 1
            
    allowed_chunks = allowed_chunks[:6]
    missing_info = list(state.get("missing_info") or [])
    if restricted_matches > 0:
        missing_info.append("Some matching sources are restricted for the current user role.")
        
    return {
        "rbac_allowed_chunks": allowed_chunks,
        "missing_info": missing_info
    }

async def generation_node(state: CopilotState) -> Dict[str, Any]:
    """Calls Gemini/Groq LLM to generate cited technical answers or casual chat"""
    intent = state.get("query_intent") or "GENERAL"
    question = state["messages"][-1].content if state.get("messages") else ""
    attempts = state.get("validation_attempts") or 0
    attempts += 1
    
    validation_errors = state.get("validation_errors") or []
    
    if intent == "GENERAL":
        # Multi-turn chat generation
        chat_history_str = ""
        for msg in state.get("messages", [])[:-1]:
            role = "User" if msg.type == "human" else "AI"
            chat_history_str += f"{role}: {msg.content}\n"
            
        prompt = f"""
You are PlantBrainAI, a smart engineering virtual assistant. Answer the user politely.
If they are chatting, maintain conversational context. Keep responses concise and clear.

Chat history:
{chat_history_str}
User latest input: {question}
AI:
"""
        answer = await ai_clients.generate_text_llm(prompt)
        return {
            "answer": answer,
            "confidence": 1.0,
            "citations": [],
            "validation_attempts": attempts
        }
        
    # RAG technical flow
    chunks = state.get("rbac_allowed_chunks") or []
    if not chunks:
        return {
            "answer": "No accessible documentation or evidence was found for the query in this plant's database.",
            "confidence": 0.0,
            "citations": [],
            "validation_attempts": attempts
        }
        
    context_blocks = []
    citations = []
    for idx, r in enumerate(chunks):
        context_blocks.append(f"[{idx+1}] Doc: {r['title']} (Page {r['page_no']})\n{r['chunk_text']}")
        citations.append({
            "documentId": str(r["document_id"]),
            "documentTitle": r["title"],
            "page": r["page_no"],
            "snippet": r["chunk_text"][:200] + "..."
        })
        
    context_str = "\n\n".join(context_blocks)
    
    error_feedback = ""
    if validation_errors:
        error_feedback = f"\nWARNING: Your previous answer failed citation validation with error: '{validation_errors[-1]}'. Make sure to explicitly write the bracketed index (e.g. [1], [2]) next to facts you extract from the document excerpts below."
        
    prompt = f"""
You are an expert industrial engineering AI. Answer the following technical question based ONLY on the provided document excerpts.
Every fact in your answer must cite the source excerpt index like [1] or [2] matching the provided texts.
If the information is not present, clearly state what information is missing.
{error_feedback}

Question: {question}

Excerpts:
{context_str}

Format your output exactly as:
ANSWER: <detailed answer citing [1], [2] etc>
CONFIDENCE: <estimated floating point score between 0.0 and 1.0>
MISSING_INFO: <comma separated details of any missing info or data gaps, or None>
"""
    llm_output = await ai_clients.generate_text_llm(prompt)
    
    answer = "Unable to process query."
    confidence = 0.5
    missing_info = list(state.get("missing_info") or [])
    
    try:
        ans_match = re.search(r"ANSWER:\s*(.*?)(?=CONFIDENCE:|$)", llm_output, re.DOTALL)
        conf_match = re.search(r"CONFIDENCE:\s*([\d\.]+)", llm_output)
        miss_match = re.search(r"MISSING_INFO:\s*(.*)", llm_output)
        
        if ans_match:
            answer = ans_match.group(1).strip()
        if conf_match:
            confidence = float(conf_match.group(1).strip())
        if miss_match:
            info_text = miss_match.group(1).strip()
            if info_text.lower() != "none" and info_text:
                for x in info_text.split(","):
                    val = x.strip()
                    if val and val not in missing_info:
                        missing_info.append(val)
    except Exception as parse_e:
        answer = llm_output
        
    return {
        "answer": answer,
        "confidence": confidence,
        "citations": citations,
        "missing_info": missing_info,
        "validation_attempts": attempts
    }

async def validation_node(state: CopilotState) -> Dict[str, Any]:
    """Validates citations in generated answers and triggers retries if validation fails"""
    intent = state.get("query_intent") or "GENERAL"
    if intent == "GENERAL":
        return {"validation_errors": []}
        
    answer = state.get("answer") or ""
    citations = state.get("citations") or []
    attempts = state.get("validation_attempts") or 0
    
    validation_errors = list(state.get("validation_errors") or [])
    
    if not answer_has_citation(answer, len(citations)):
        error_msg = f"The generated answer did not include valid source citations matching the retrieved chunks (expected bracketed citations up to [{len(citations)}])."
        validation_errors.append(error_msg)
        logger.warning(f"Citation validation failed on attempt {attempts}: {error_msg}")
        
        if attempts >= 3:
            logger.error("Max validation attempts reached. Returning default fallback answer.")
            return {
                "answer": "Not enough cited evidence is available to answer safely. Please review the source documents or refine the question.",
                "confidence": min(state.get("confidence") or 0.5, 0.2),
                "validation_errors": validation_errors
            }
    else:
        # Clear errors if valid
        return {"validation_errors": []}
        
    return {"validation_errors": validation_errors}

def route_after_generation(state: CopilotState) -> str:
    """Controls loops between generation and validation based on error presence"""
    validation_errors = state.get("validation_errors") or []
    attempts = state.get("validation_attempts") or 0
    
    if validation_errors and attempts < 3:
        if "Not enough cited evidence" not in state.get("answer", ""):
            logger.info(f"Routing back to generate node for citation correction (attempt {attempts+1})")
            return "generate"
            
    return END

# ==========================================
# GRAPH COMPILATION ENGINE
# ==========================================

def compile_copilot_graph(checkpointer: Optional[Any] = None) -> StateGraph:
    """Builds and compiles the Copilot workflow graph"""
    workflow = StateGraph(CopilotState)
    
    # Register Nodes
    workflow.add_node("guardrail", guardrail_node)
    workflow.add_node("router", intent_router_node)
    workflow.add_node("retrieve", retrieval_node)
    workflow.add_node("rbac", rbac_node)
    workflow.add_node("generate", generation_node)
    workflow.add_node("validate", validation_node)
    
    # Establish Edges
    workflow.add_edge(START, "guardrail")
    
    workflow.add_conditional_edges(
        "guardrail",
        route_after_guardrail,
        {
            END: END,
            "router": "router"
        }
    )
    
    workflow.add_conditional_edges(
        "router",
        route_by_intent,
        {
            "retrieve": "retrieve",
            "generate": "generate"
        }
    )
    
    workflow.add_edge("retrieve", "rbac")
    workflow.add_edge("rbac", "generate")
    workflow.add_edge("generate", "validate")
    
    workflow.add_conditional_edges(
        "validate",
        route_after_generation,
        {
            "generate": "generate",
            END: END
        }
    )
    
    return workflow.compile(checkpointer=checkpointer)
