import re
import uuid
import json
from typing import Dict, Any
from fastapi import APIRouter

from app.schemas import RCAGenerateRequest
import app.database as database
import app.utils.ai_clients as ai_clients
from app.utils.helpers import optional_uuid, graceful_rca_response

router = APIRouter()

@router.post("/rca")
async def generate_rca(request: RCAGenerateRequest):
    """Retrieves maintenance logs and work orders for an asset tag and drafts an LLM-powered RCA report"""
    if not database.db_pool:
        return graceful_rca_response(request.assetTag, "Database connection unavailable")

    asset_tag = request.assetTag.upper().strip()
    def row_value(row, key, default=None):
        try:
            return row[key]
        except (KeyError, IndexError):
            return default

    try:
        plant_uuid = optional_uuid(request.plantId)
        org_uuid = optional_uuid(request.organizationId)
        async with database.db_pool.acquire() as conn:
            # Query all entities matching this asset tag to pull corresponding document contents
            rows = await conn.fetch("""
                SELECT DISTINCT c.chunk_text, d.title, d.created_at, d.id as document_id
                FROM graph.entities e
                JOIN ingestion.document_chunks c ON e.chunk_id = c.id
                JOIN document.documents d ON c.document_id = d.id
                WHERE e.normalized_value = $1
                  AND d.status <> 'ARCHIVED'
                  AND c.document_version_id = d.current_version_id
                  AND ($2::uuid IS NULL OR d.plant_id = $2)
                  AND ($3::uuid IS NULL OR d.organization_id = $3)
                ORDER BY d.created_at DESC
                LIMIT 10
            """, asset_tag, plant_uuid, org_uuid)

            if not rows:
                return {
                    "summary": f"No historical maintenance records or manuals found for asset tag {asset_tag}.",
                    "probableCauses": ["Data deficiency"],
                    "recommendations": ["Upload asset manuals and past work orders to initiate RCA"],
                    "confidence": 0.0,
                    "citations": []
                }

            history_blocks = []
            for r in rows:
                history_blocks.append(f"Date: {r['created_at'].strftime('%Y-%m-%d')} | Doc: {r['title']}\n{r['chunk_text']}")

            history_str = "\n\n".join(history_blocks)
            prompt = f"""
You are an industrial reliability expert. Construct a Root Cause Analysis (RCA) report for the asset {asset_tag}.
Symptoms of failure: {request.failureDescription}

Here is the historical context of logs, inspection records, and manual entries for {asset_tag}:
{history_str}

Please generate an RCA with:
1. PROBLEM SUMMARY: A clear problem statement based on the logs.
2. TIMELINE: Trace the chronology of failure events.
3. PROBABLE CAUSES: Deduce likely causes using 5-Whys methodology.
4. RECOMMENDATIONS: Bullet points of actionable corrective actions.
5. CONFIDENCE SCORE: A decimal between 0.0 and 1.0.

Format your output exactly as:
SUMMARY: <detailed text>
PROBABLE_CAUSES: <comma separated causes>
RECOMMENDATIONS: <comma separated actions>
CONFIDENCE: <decimal value>
"""
            llm_output = await ai_clients.generate_text_llm(prompt)

            # Simple parses
            summary = "Failed to draft RCA summary."
            probable_causes = []
            recommendations = []
            confidence = 0.5

            try:
                sum_match = re.search(r"SUMMARY:\s*(.*?)(?=PROBABLE_CAUSES:|$)", llm_output, re.DOTALL)
                causes_match = re.search(r"PROBABLE_CAUSES:\s*(.*?)(?=RECOMMENDATIONS:|$)", llm_output, re.DOTALL)
                recs_match = re.search(r"RECOMMENDATIONS:\s*(.*?)(?=CONFIDENCE:|$)", llm_output, re.DOTALL)
                conf_match = re.search(r"CONFIDENCE:\s*([\d\.]+)", llm_output)

                if sum_match:
                    summary = sum_match.group(1).strip()
                if causes_match:
                    probable_causes = [x.strip() for x in causes_match.group(1).split(",") if x.strip()]
                if recs_match:
                    recommendations = [x.strip() for x in recs_match.group(1).split(",") if x.strip()]
                if conf_match:
                    confidence = float(conf_match.group(1).strip())
            except Exception as e:
                summary = llm_output

            # Log to rca.reports table
            asset_row = await conn.fetchrow("""
                SELECT id, organization_id, plant_id
                FROM asset.assets
                WHERE asset_tag = $1
                  AND ($2::uuid IS NULL OR plant_id = $2)
                  AND ($3::uuid IS NULL OR organization_id = $3)
                LIMIT 1
            """, asset_tag, plant_uuid, org_uuid)
            if asset_row:
                asset_plant_id = row_value(asset_row, "plant_id", plant_uuid)
                await conn.execute("""
                    INSERT INTO rca.reports (organization_id, plant_id, asset_id, failure_summary, probable_causes, recommendations, confidence, created_by)
                    VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
                """, asset_row["organization_id"], asset_plant_id, asset_row["id"], summary, json.dumps(probable_causes), json.dumps(recommendations), confidence, optional_uuid(request.userId))

            citations = []
            for r in rows:
                document_id = row_value(r, "document_id")
                citation = {"documentTitle": row_value(r, "title", "Untitled document")}
                if document_id is not None:
                    citation["documentId"] = str(document_id)
                citations.append(citation)

            return {
                "summary": summary,
                "probableCauses": probable_causes,
                "recommendations": recommendations,
                "confidence": confidence,
                "citations": citations
            }

    except Exception as e:
        return graceful_rca_response(request.assetTag, str(e))
