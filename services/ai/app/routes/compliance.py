import uuid
from fastapi import APIRouter, HTTPException
import app.database as database

router = APIRouter()

@router.get("/compliance")
async def audit_compliance(plantId: str):
    """Scans all assets for this plant and flags compliance gaps (missing checklists, overdue inspections)"""
    if not database.db_pool:
        raise HTTPException(status_code=500, detail="Database connection unavailable")

    try:
        plant_uuid = uuid.UUID(plantId)
        async with database.db_pool.acquire() as conn:
            # 1. Fetch all assets for this plant
            assets = await conn.fetch("SELECT id, asset_tag, asset_name FROM asset.assets WHERE plant_id = $1", plant_uuid)

            # Fetch all requirements
            requirements = await conn.fetch("SELECT id, title, requirement_type, frequency FROM compliance.requirements")

            if not requirements:
                # Insert default requirements if none exist
                default_reqs = [
                    ("Annual Pressure Vessel Test", "SAFETY", "yearly"),
                    ("Quarterly Fire Extinguisher Check", "SAFETY", "quarterly"),
                    ("Monthly Pump Mechanical Seal Leak Inspection", "MAINTENANCE", "monthly")
                ]
                # Fetch org ID
                org_row = await conn.fetchrow("SELECT organization_id FROM identity.plants WHERE id = $1", plant_uuid)
                org_id = org_row["organization_id"] if org_row else uuid.uuid4()

                for title, r_type, freq in default_reqs:
                    await conn.execute("""
                        INSERT INTO compliance.requirements (organization_id, plant_id, title, requirement_type, frequency)
                        VALUES ($1, $2, $3, $4, $5)
                    """, org_id, plant_uuid, title, r_type, freq)
                requirements = await conn.fetch("SELECT id, title, requirement_type, frequency FROM compliance.requirements WHERE plant_id = $1", plant_uuid)

            # 2. Check each asset against the requirements
            # We look in the graph.entities or document.documents for inspection reports
            gaps = []
            for asset in assets:
                asset_id = asset["id"]
                tag = asset["asset_tag"]

                # Check for "Inspection" or "Report" or "Certificate" documents mentioning this asset tag
                has_inspection = await conn.fetchval("""
                    SELECT COUNT(*) FROM graph.entities e
                    JOIN document.documents d ON e.document_id = d.id
                    WHERE e.normalized_value = $1 AND (d.document_type ILIKE '%inspection%' OR d.title ILIKE '%inspection%' OR d.title ILIKE '%report%')
                """, tag)

                if has_inspection == 0:
                    # Missing all inspections
                    for req in requirements:
                        # Log gap
                        description = f"No inspection evidence found for asset {tag} satisfying standard requirement '{req['title']}'."
                        await conn.execute("""
                            INSERT INTO compliance.gaps (organization_id, plant_id, asset_id, requirement_id, gap_type, description, severity, status)
                            VALUES ((SELECT organization_id FROM asset.assets WHERE id = $1), $2, $1, $3, 'MISSING_EVIDENCE', $4, 'HIGH', 'OPEN')
                            ON CONFLICT DO NOTHING
                        """, asset_id, plant_uuid, req["id"], description)

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
                        WHERE e.normalized_value = $1 AND d.title ILIKE '%expired%'
                    """, tag)
                    for ed in expired_docs:
                        description = f"Overdue certificate/expired document: '{ed['title']}' found associated with {tag}."
                        await conn.execute("""
                            INSERT INTO compliance.gaps (organization_id, plant_id, asset_id, gap_type, description, severity, status, evidence_document_id)
                            VALUES ((SELECT organization_id FROM asset.assets WHERE id = $1), $2, $1, 'EXPIRED_CERTIFICATE', $3, 'CRITICAL', 'OPEN', $4)
                            ON CONFLICT DO NOTHING
                        """, asset_id, plant_uuid, description, ed["id"])
                        gaps.append({
                            "assetTag": tag,
                            "gapType": "EXPIRED_CERTIFICATE",
                            "description": description,
                            "severity": "CRITICAL",
                            "status": "OPEN",
                            "standard": "Regulatory Certificate"
                        })

            return gaps

    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))
