import re
import uuid
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

router = APIRouter()


@router.post("/query")
async def rag_query(request: CopilotQueryRequest):
    """Executes hybrid pgvector search and synthesizes answers with citations"""
    if not database.db_pool:
        return graceful_query_response(
            request.question, "Database connection unavailable"
        )

    if query_looks_unsafe(request.question):
        return {
            "answer": "I cannot provide unsafe operational bypasses or legal/compliance certification. Review the authorized SOP and have a qualified supervisor approve any critical action.",
            "confidence": 0.0,
            "citations": [],
            "relatedAssets": extract_asset_tags(request.question),
            "missingInfo": [
                "Query requested unsafe, unsupported, or certification-like guidance."
            ],
            "fallback": True,
        }

    try:
        plant_uuid = optional_uuid(request.plantId)
        if not plant_uuid:
            return graceful_query_response(
                request.question, "Invalid or missing plantId"
            )

        # Generate 1536-dimensional query embedding
        query_embedding = await ai_clients.get_gemini_embedding_1536(request.question)

        # Parse filters
        filters = request.filters or {}
        asset_filter = filters.get("assetTag") if isinstance(filters, dict) else None
        document_type_filters = (
            normalize_filter_values(
                filters.get("documentTypes") or filters.get("documentType")
            )
            if isinstance(filters, dict)
            else []
        )
        date_from = filters.get("dateFrom") if isinstance(filters, dict) else None
        date_to = filters.get("dateTo") if isinstance(filters, dict) else None

        # 1. Retrieve Candidate Chunks (pgvector Cosine Distance <->)
        async with database.db_pool.acquire() as conn:
            org_row = await conn.fetchrow(
                "SELECT organization_id FROM identity.plants WHERE id = $1", plant_uuid
            )
            if not org_row:
                return graceful_query_response(request.question, "Plant not found")

            org_id = optional_uuid(request.organizationId) or org_row["organization_id"]
            if (
                optional_uuid(request.organizationId)
                and org_row["organization_id"] != org_id
            ):
                return graceful_query_response(
                    request.question, "Plant is outside the requested organization"
                )

            current_user_role = await resolve_request_role(
                conn, request, plant_uuid, org_id
            )
            access_columns = await get_document_access_columns(conn)
            access_select = document_access_select(access_columns)

            chunks_rows = await conn.fetch(
                f"""
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
            """,
                query_embedding,
                plant_uuid,
                org_id,
                document_type_filters,
                date_from,
                date_to,
            )

            # 2. Keyword/Exact Tag search fallback (if tag is queried or contained)
            keyword_chunks = []
            extracted_tags = re.findall(
                r"\b[A-Z]+[-\s]*\d+[A-Z]*\b", request.question.upper()
            )
            if asset_filter:
                extracted_tags.append(asset_filter.upper())

            if extracted_tags:
                tag_queries = [f"%{tag}%" for tag in extracted_tags]
                for t_q in tag_queries:
                    k_rows = await conn.fetch(
                        f"""
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
                          AND (c.chunk_text ILIKE $2 OR d.title ILIKE $2)
                        LIMIT 3
                    """,
                        plant_uuid,
                        t_q,
                        org_id,
                        document_type_filters,
                        date_from,
                        date_to,
                    )
                    keyword_chunks.extend(k_rows)

            # Combine searches
            seen = set()
            combined_results = []
            restricted_matches = 0
            for r in keyword_chunks + list(chunks_rows):
                if r["id"] not in seen:
                    seen.add(r["id"])
                    if not document_row_access_allowed(r, current_user_role):
                        restricted_matches += 1
                        continue
                    combined_results.append(r)

            # Sort combined results
            combined_results = combined_results[:6]

            if not combined_results:
                missing_info = []
                if restricted_matches:
                    missing_info.append(
                        "Some matching sources are restricted for the current user role."
                    )
                return {
                    "answer": "No accessible documentation or evidence was found for the query in this plant's database.",
                    "confidence": 0.0,
                    "citations": [],
                    "relatedAssets": [],
                    "missingInfo": missing_info,
                }

            # 3. Construct prompt
            context_blocks = []
            citations = []
            for idx, r in enumerate(combined_results):
                context_blocks.append(
                    f"[{idx + 1}] Doc: {r['title']} (Page {r['page_no']})\n{r['chunk_text']}"
                )
                citations.append(
                    {
                        "documentId": str(r["document_id"]),
                        "documentTitle": r["title"],
                        "page": r["page_no"],
                        "snippet": r["chunk_text"][:200] + "...",
                    }
                )

            context_str = "\n\n".join(context_blocks)
            prompt = f"""
You are an expert industrial engineering AI. Answer the following technical question based ONLY on the provided document excerpts.
Every fact in your answer must cite the source excerpt index like [1] or [2] matching the provided texts.
If the information is not present, clearly state what information is missing.

Question: {request.question}

Excerpts:
{context_str}

Format your output exactly as:
ANSWER: <detailed answer citing [1], [2] etc>
CONFIDENCE: <estimated floating point score between 0.0 and 1.0>
MISSING_INFO: <comma separated details of any missing info or data gaps, or None>
"""
            llm_output = await ai_clients.generate_text_llm(prompt)

            # Parse output
            answer = "Unable to process query."
            confidence = 0.5
            missing_info = []

            try:
                ans_match = re.search(
                    r"ANSWER:\s*(.*?)(?=CONFIDENCE:|$)", llm_output, re.DOTALL
                )
                conf_match = re.search(r"CONFIDENCE:\s*([\d\.]+)", llm_output)
                miss_match = re.search(r"MISSING_INFO:\s*(.*)", llm_output)

                if ans_match:
                    answer = ans_match.group(1).strip()
                if conf_match:
                    confidence = float(conf_match.group(1).strip())
                if miss_match:
                    info_text = miss_match.group(1).strip()
                    if info_text.lower() != "none" and info_text:
                        missing_info = [
                            x.strip() for x in info_text.split(",") if x.strip()
                        ]
            except Exception as parse_e:
                answer = llm_output

            if not answer_has_citation(answer, len(citations)):
                missing_info.append(
                    "The generated answer did not include valid source citations, so it was not returned as a factual answer."
                )
                answer = "Not enough cited evidence is available to answer safely. Please review the source documents or refine the question."
                confidence = min(confidence, 0.2)

            # Save query log to DB
            query_uuid = uuid.uuid4()
            await conn.execute(
                """
                INSERT INTO rag.queries (id, organization_id, plant_id, user_id, query_text, answer_text, confidence)
                VALUES ($1, $2, $3, $4, $5, $6, $7)
            """,
                query_uuid,
                org_id,
                plant_uuid,
                optional_uuid(request.userId),
                request.question,
                answer,
                confidence,
            )

            for citation in citations:
                await conn.execute(
                    """
                    INSERT INTO rag.citations (query_id, document_id, page_no, quoted_text)
                    VALUES ($1, $2, $3, $4)
                """,
                    query_uuid,
                    uuid.UUID(citation["documentId"]),
                    citation["page"],
                    citation["snippet"],
                )

            return {
                "answer": answer,
                "confidence": confidence,
                "citations": citations,
                "relatedAssets": extracted_tags,
                "missingInfo": missing_info,
            }

    except Exception as e:
        return graceful_query_response(request.question, str(e))
