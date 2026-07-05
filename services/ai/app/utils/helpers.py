import uuid
import re
import json
from typing import Optional, List, Dict, Any
from app.config import logger

ROLE_ALIASES = {
    "administrator": "admin",
    "plant_manager": "plant_manager",
    "manager": "plant_manager",
    "maintenance_engineer": "engineer",
    "reliability_engineer": "engineer",
    "plant_engineer": "engineer",
    "field_technician": "technician",
    "plant_operator": "technician",
    "operator": "technician",
    "compliance": "compliance_officer",
    "readonly": "viewer",
    "read_only": "viewer",
}
RESTRICTED_DOCUMENT_ROLES = {"admin", "plant_manager", "engineer", "compliance_officer"}
PUBLIC_ACCESS_LEVELS = {"", "public", "open", "internal", "organization", "org", "plant", "general", "unrestricted"}
RESTRICTED_ACCESS_LEVELS = {"restricted", "confidential", "sensitive", "private", "controlled", "compliance", "audit", "regulated"}

UNSAFE_QUERY_PATTERN = re.compile(
    r"\b(bypass|disable|override|ignore safety|hotwire|remove guard|defeat interlock|without permit|legal guarantee|certify compliance)\b",
    re.IGNORECASE,
)

def optional_uuid(value: Optional[str]) -> Optional[uuid.UUID]:
    if not value:
        return None
    try:
        return uuid.UUID(str(value))
    except (TypeError, ValueError):
        return None

def extract_asset_tags(text: str) -> List[str]:
    seen = set()
    tags = []
    for match in re.finditer(r"\b(?!(?:VERSION|PAGE|REV|TABLE|FIG|FIGURE)\b)[A-Z]+[-\s]*\d+[A-Z]*\b", text.upper()):
        tag = re.sub(r"\s+", "-", match.group()).strip("-")
        if tag and tag not in seen:
            seen.add(tag)
            tags.append(tag)
    return tags

def row_get(row: Any, key: str, default: Any = None) -> Any:
    if row is None:
        return default
    try:
        return row[key]
    except (KeyError, IndexError, TypeError):
        return default

def normalize_role_name(role: Any) -> str:
    normalized = re.sub(r"[^a-z0-9]+", "_", str(role or "").strip().lower()).strip("_")
    if not normalized:
        return "viewer"
    return ROLE_ALIASES.get(normalized, normalized)

def normalize_access_level(access_level: Any) -> str:
    return re.sub(r"[^a-z0-9]+", "_", str(access_level or "").strip().lower()).strip("_")

def parse_allowed_roles(value: Any) -> List[str]:
    if value is None:
        return []

    parsed_value = value
    if isinstance(value, str):
        stripped = value.strip()
        if not stripped:
            return []
        try:
            parsed_value = json.loads(stripped)
        except json.JSONDecodeError:
            parsed_value = re.split(r"[,;]", stripped)

    if isinstance(parsed_value, dict):
        parsed_value = (
            parsed_value.get("roles")
            or parsed_value.get("allowedRoles")
            or parsed_value.get("allowed_roles")
            or list(parsed_value.values())
        )

    if isinstance(parsed_value, (list, tuple, set)):
        return [normalize_role_name(role) for role in parsed_value if str(role or "").strip()]

    return [normalize_role_name(parsed_value)] if str(parsed_value or "").strip() else []

def request_role_hint(request: Any) -> Optional[str]:
    for attr in ("userRole", "role"):
        value = getattr(request, attr, None)
        if value:
            return value

    filters = getattr(request, "filters", None) or {}
    if isinstance(filters, dict):
        for key in ("userRole", "role", "currentUserRole"):
            if filters.get(key):
                return filters[key]
    return None

async def resolve_request_role(conn: Any, request: Any, plant_uuid: uuid.UUID, org_id: uuid.UUID) -> str:
    role_hint = request_role_hint(request)
    user_uuid = optional_uuid(getattr(request, "userId", None))
    if user_uuid:
        try:
            role_row = await conn.fetchrow("""
                SELECT r.name AS role
                FROM identity.memberships m
                JOIN identity.roles r ON m.role_id = r.id
                WHERE m.user_id = $1
                  AND m.organization_id = $2
                  AND (m.plant_id IS NULL OR m.plant_id = $3)
                ORDER BY CASE WHEN m.plant_id = $3 THEN 0 ELSE 1 END
                LIMIT 1
            """, user_uuid, org_id, plant_uuid)
            db_role = row_get(role_row, "role")
            if db_role:
                return normalize_role_name(db_role)
        except Exception as e:
            logger.warning(f"Unable to resolve user role for RAG access filtering: {e}")
    return normalize_role_name(role_hint)

def document_row_access_allowed(row: Any, user_role: str) -> bool:
    role = normalize_role_name(user_role)
    if role == "admin":
        return True

    allowed_roles = parse_allowed_roles(row_get(row, "allowed_roles"))
    if allowed_roles:
        return role in allowed_roles

    access_level = normalize_access_level(row_get(row, "access_level"))
    if access_level in PUBLIC_ACCESS_LEVELS:
        return True
    if access_level in RESTRICTED_ACCESS_LEVELS:
        return role in RESTRICTED_DOCUMENT_ROLES

    return True

def query_looks_unsafe(question: str) -> bool:
    return bool(UNSAFE_QUERY_PATTERN.search(question or ""))

def answer_has_citation(answer: str, citation_count: int) -> bool:
    if citation_count <= 0:
        return False
    cited_indexes = {int(match) for match in re.findall(r"\[(\d+)\]", answer or "")}
    return any(1 <= index <= citation_count for index in cited_indexes)

def graceful_query_response(question: str, reason: str) -> Dict[str, Any]:
    missing_info = ["Not enough evidence is available because the AI retrieval path could not complete."]
    if reason:
        missing_info.append(reason)
    return {
        "answer": "Not enough evidence is available to answer safely right now.",
        "confidence": 0.0,
        "citations": [],
        "relatedAssets": extract_asset_tags(question),
        "missingInfo": missing_info,
        "fallback": True
    }

def graceful_rca_response(asset_tag: str, reason: str) -> Dict[str, Any]:
    missing_data = ["RCA could not be generated from evidence because the AI path could not complete."]
    if reason:
        missing_data.append(reason)
    normalized_tag = asset_tag.upper().strip() if asset_tag else "the requested asset"
    return {
        "summary": f"RCA for {normalized_tag} could not be generated safely right now.",
        "probableCauses": ["Data unavailable"],
        "recommendations": ["Retry after AI processing is healthy", "Review available work orders and inspection records manually before taking action"],
        "confidence": 0.0,
        "citations": [],
        "missingData": missing_data,
        "fallback": True
    }

def normalize_filter_values(value: Any) -> List[str]:
    if value is None:
        return []
    if isinstance(value, str):
        return [item.strip() for item in re.split(r"[,;]", value) if item.strip()]
    if isinstance(value, (list, tuple, set)):
        return [str(item).strip() for item in value if str(item).strip()]
    return [str(value).strip()] if str(value).strip() else []

