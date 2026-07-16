import uuid
from fastapi import APIRouter, HTTPException
import app.database as database
from app.config import logger

router = APIRouter()

@router.get("/compliance")
async def audit_compliance(plantId: str):
    """Scans all assets for this plant and flags compliance gaps (missing checklists, overdue inspections).

    Every query is scoped to the plant's organization — evidence from other tenants
    must never satisfy (or trigger) a gap here. Requirements are seeded via
    infra/db/seeds, never invented inside this handler.
    """
    if not database.db_pool:
        raise HTTPException(status_code=500, detail="Database connection unavailable")

    try:
        plant_uuid = uuid.UUID(plantId)
    except ValueError:
        raise HTTPException(status_code=400, detail="plantId must be a valid UUID")

    try:
        async with database.db_pool.acquire() as conn:
            org_row = await conn.fetchrow("SELECT organization_id FROM identity.plants WHERE id = $1", plant_uuid)
            if not org_row or not org_row["organization_id"]:
                raise HTTPException(status_code=404, detail="Plant not found; cannot run a scoped compliance scan")
            org_id = org_row["organization_id"]

            # 1. Fetch all assets for this plant
            assets = await conn.fetch(
                "SELECT id, asset_tag, asset_name FROM asset.assets WHERE plant_id = $1 AND organization_id = $2",
                plant_uuid, org_id,
            )

            # Requirements applying to this plant: plant-specific plus org-wide ones
            requirements = await conn.fetch("""
                SELECT id, title, requirement_type, frequency
                FROM compliance.requirements
                WHERE organization_id = $1
                  AND (plant_id IS NULL OR plant_id = $2)
            """, org_id, plant_uuid)

            if not requirements:
                logger.warning(
                    "Compliance scan for plant %s ran without any configured requirements; "
                    "missing-evidence checks were skipped (seed compliance.requirements to enable them)",
                    plantId,
                )

            # 2. Check each asset against the requirements
            gaps = []
            for asset in assets:
                asset_id = asset["id"]
                tag = asset["asset_tag"]

                # Check for "Inspection" or "Report" documents mentioning this asset tag, in this tenant only
                has_inspection = await conn.fetchval("""
                    SELECT COUNT(*) FROM graph.entities e
                    JOIN document.documents d ON e.document_id = d.id
                    WHERE e.normalized_value = $1
                      AND d.plant_id = $2
                      AND d.organization_id = $3
                      AND d.status <> 'ARCHIVED'
                      AND (d.document_type ILIKE '%inspection%' OR d.title ILIKE '%inspection%' OR d.title ILIKE '%report%')
                """, tag, plant_uuid, org_id)

                if has_inspection == 0:
                    # Missing all inspections
                    for req in requirements:
                        description = f"No inspection evidence found for asset {tag} satisfying standard requirement '{req['title']}'."
                        await conn.execute("""
                            INSERT INTO compliance.gaps (organization_id, plant_id, asset_id, requirement_id, gap_type, description, severity, status)
                            VALUES ($1, $2, $3, $4, 'MISSING_EVIDENCE', $5, 'HIGH', 'OPEN')
                            ON CONFLICT DO NOTHING
                        """, org_id, plant_uuid, asset_id, req["id"], description)

                        gaps.append({
                            "assetTag": tag,
                            "gapType": "MISSING_EVIDENCE",
                            "description": description,
                            "severity": "HIGH",
                            "status": "OPEN",
                            "standard": req["title"]
                        })
                else:
                    # Check for expiration of certificate if any document has an expiry tag
                    expired_docs = await conn.fetch("""
                        SELECT d.id, d.title FROM graph.entities e
                        JOIN document.documents d ON e.document_id = d.id
                        WHERE e.normalized_value = $1
                          AND d.plant_id = $2
                          AND d.organization_id = $3
                          AND d.status <> 'ARCHIVED'
                          AND d.title ILIKE '%expired%'
                    """, tag, plant_uuid, org_id)
                    for ed in expired_docs:
                        description = f"Overdue certificate/expired document: '{ed['title']}' found associated with {tag}."
                        await conn.execute("""
                            INSERT INTO compliance.gaps (organization_id, plant_id, asset_id, gap_type, description, severity, status, evidence_document_id)
                            VALUES ($1, $2, $3, 'EXPIRED_CERTIFICATE', $4, 'CRITICAL', 'OPEN', $5)
                            ON CONFLICT DO NOTHING
                        """, org_id, plant_uuid, asset_id, description, ed["id"])
                        gaps.append({
                            "assetTag": tag,
                            "gapType": "EXPIRED_CERTIFICATE",
                            "description": description,
                            "severity": "CRITICAL",
                            "status": "OPEN",
                            "standard": "Regulatory Certificate"
                        })

            return gaps

    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))
