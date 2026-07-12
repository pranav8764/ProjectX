from pydantic import BaseModel
from typing import Dict, Any, Optional

class DocumentProcessRequest(BaseModel):
    document_id: str
    file_path: str
    file_type: str
    metadata: Dict[str, Any]

class CopilotQueryRequest(BaseModel):
    question: str
    plantId: str
    userId: Optional[str] = None
    organizationId: Optional[str] = None
    userRole: Optional[str] = None
    filters: Optional[Dict[str, Any]] = None

class RCAGenerateRequest(BaseModel):
    assetTag: str
    failureDescription: str
    # plantId is mandatory: with it omitted the evidence query used to match
    # documents across every tenant in the database.
    plantId: str
    userId: Optional[str] = None
    organizationId: Optional[str] = None
    userRole: Optional[str] = None
