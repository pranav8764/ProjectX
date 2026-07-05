from typing import List, Dict, Any, Optional, Annotated
from typing_extensions import TypedDict
from langgraph.graph.message import add_messages

class CopilotState(TypedDict):
    # Short-term chat memory (messages list using the add_messages reducer)
    messages: Annotated[list, add_messages]
    
    # Request metadata
    plant_id: str
    organization_id: str
    user_id: Optional[str]
    user_role: str
    filters: Optional[Dict[str, Any]]
    
    # Intermediate pipeline outputs
    query_intent: str  # "GENERAL", "RAG_QUERY", "RCA_GENERATE"
    retrieved_chunks: List[Dict[str, Any]]
    rbac_allowed_chunks: List[Dict[str, Any]]
    
    # Output properties
    answer: str
    confidence: float
    missing_info: List[str]
    citations: List[Dict[str, Any]]
    
    # Self-correction loop tracking
    validation_attempts: int
    validation_errors: List[str]
