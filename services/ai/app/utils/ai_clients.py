import asyncio
import hashlib
import time
import numpy as np
from typing import List, Optional
from google.genai import types
from app.config import GOOGLE_API_KEY, gemini_client, groq_client, logger, GEMINI_MODEL, GEMINI_EMBED_MODEL, GEMINI_EMBED_DIM, GROQ_MODEL
import app.database as database
from app.utils.helpers import optional_uuid

class LLMUnavailableError(RuntimeError):
    """Raised when every configured LLM provider fails or none is configured.
    Callers must treat this as an error path — never parse it as model output."""

def _l2_normalize(values: List[float]) -> List[float]:
    """gemini-embedding-001 only pre-normalizes at 3072 dims; normalize truncated
    outputs so cosine distance behaves correctly."""
    norm = float(np.linalg.norm(values)) or 1.0
    return [v / norm for v in values]

def _embed_config(task_type: str):
    return types.EmbedContentConfig(task_type=task_type, output_dimensionality=GEMINI_EMBED_DIM)

async def log_model_call(
    service_name: str,
    provider: str,
    model: str,
    status: str,
    latency_ms: int,
    prompt_tokens: Optional[int] = None,
    completion_tokens: Optional[int] = None,
    org_id: Optional[str] = None,
    correlation_id: Optional[str] = None
):
    """Asynchronously logs AI model calls to the database auditing schema"""
    if not database.db_pool:
        return
    try:
        async with database.db_pool.acquire() as conn:
            await conn.execute("""
                INSERT INTO ai.model_calls (
                    organization_id, service_name, provider, model, 
                    prompt_tokens, completion_tokens, latency_ms, status, correlation_id
                )
                VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
            """, optional_uuid(org_id), service_name, provider, model, prompt_tokens, completion_tokens, latency_ms, status, correlation_id)
    except Exception as e:
        logger.warning(f"Failed to log model call to database: {e}")

def deterministic_embedding_1536(text: str) -> List[float]:
    """Stable local fallback embedding so retrieval never degrades to identical zero vectors."""
    seed = hashlib.sha256((text or "").encode("utf-8")).digest()
    values = []
    counter = 0
    while len(values) < 1536:
        digest = hashlib.sha256(seed + counter.to_bytes(4, "big")).digest()
        for byte in digest:
            values.append((byte / 127.5) - 1.0)
            if len(values) == 1536:
                break
        counter += 1
    norm = float(np.linalg.norm(values)) or 1.0
    return [v / norm for v in values]

async def get_gemini_embedding_1536(text: str, org_id: Optional[str] = None, correlation_id: Optional[str] = None,
                                     task_type: str = "RETRIEVAL_QUERY") -> Optional[List[float]]:
    """
    Generates a query-side embedding at GEMINI_EMBED_DIM (1536) via gemini-embedding-001.

    Returns None when no real embedding can be produced — callers must skip vector
    search rather than compare a fake vector against real document embeddings.
    """
    if not GOOGLE_API_KEY or not gemini_client:
        return None

    try:
        start_time = time.time()
        response = await gemini_client.aio.models.embed_content(
            model=GEMINI_EMBED_MODEL,
            contents=text,
            config=_embed_config(task_type)
        )
        latency = int((time.time() - start_time) * 1000)

        # Log embedding call
        asyncio.create_task(log_model_call(
            service_name="copilot",
            provider="google",
            model=GEMINI_EMBED_MODEL,
            status="SUCCESS",
            latency_ms=latency,
            org_id=org_id,
            correlation_id=correlation_id
        ))

        return _l2_normalize(response.embeddings[0].values)
    except Exception as e:
        logger.error(f"Error generating Gemini embedding: {e}")
        asyncio.create_task(log_model_call(
            service_name="copilot",
            provider="google",
            model=GEMINI_EMBED_MODEL,
            status="FAILED",
            latency_ms=0,
            org_id=org_id,
            correlation_id=correlation_id
        ))
        return None

async def get_gemini_embeddings_1536_batch(texts: List[str], org_id: Optional[str] = None, correlation_id: Optional[str] = None) -> tuple:
    """
    Generates 1536-dimensional embeddings for a list of texts in a single batch call.

    Returns (embeddings, used_fallback). used_fallback=True means the vectors are
    deterministic hash pseudo-embeddings (semantic search will not work for them);
    ingestion must surface this as PARTIAL_SUCCESS instead of silently completing.
    """
    if not texts:
        return [], False

    if not GOOGLE_API_KEY or not gemini_client:
        return [deterministic_embedding_1536(t) for t in texts], True

    try:
        start_time = time.time()
        response = await gemini_client.aio.models.embed_content(
            model=GEMINI_EMBED_MODEL,
            contents=texts,
            config=_embed_config("RETRIEVAL_DOCUMENT")
        )
        latency = int((time.time() - start_time) * 1000)

        # Log batch embedding call
        asyncio.create_task(log_model_call(
            service_name="ingestion",
            provider="google",
            model=GEMINI_EMBED_MODEL,
            status="SUCCESS",
            latency_ms=latency,
            org_id=org_id,
            correlation_id=correlation_id
        ))

        return [_l2_normalize(emb.values) for emb in response.embeddings], False
    except Exception as e:
        logger.error(f"Error generating Gemini batch embeddings: {e}")
        asyncio.create_task(log_model_call(
            service_name="ingestion",
            provider="google",
            model=GEMINI_EMBED_MODEL,
            status="FAILED",
            latency_ms=0,
            org_id=org_id,
            correlation_id=correlation_id
        ))
        return [deterministic_embedding_1536(t) for t in texts], True

async def generate_text_llm(prompt: str, org_id: Optional[str] = None, correlation_id: Optional[str] = None) -> str:
    """Generate text via the configured Gemini model, falling back to Groq. Raises LLMUnavailableError when no provider succeeds."""
    if GOOGLE_API_KEY and gemini_client:
        try:
            start_time = time.time()
            response = await gemini_client.aio.models.generate_content(
                model=GEMINI_MODEL,
                contents=prompt
            )
            latency = int((time.time() - start_time) * 1000)
            
            # Safe token extraction
            usage = getattr(response, "usage_metadata", None)
            prompt_tokens = getattr(usage, "prompt_token_count", None) if usage else None
            completion_tokens = getattr(usage, "candidates_token_count", None) if usage else None
            
            # Log successful model call
            asyncio.create_task(log_model_call(
                service_name="copilot",
                provider="google",
                model=GEMINI_MODEL,
                status="SUCCESS",
                latency_ms=latency,
                prompt_tokens=prompt_tokens,
                completion_tokens=completion_tokens,
                org_id=org_id,
                correlation_id=correlation_id
            ))
            return response.text.strip()
        except Exception as e:
            logger.warning(f"Gemini generation failed: {e}. Falling back to Groq...")
            asyncio.create_task(log_model_call(
                service_name="copilot",
                provider="google",
                model=GEMINI_MODEL,
                status="FAILED",
                latency_ms=0,
                org_id=org_id,
                correlation_id=correlation_id
            ))

    if groq_client:
        try:
            start_time = time.time()
            loop = asyncio.get_event_loop()
            response = await loop.run_in_executor(
                None,
                lambda: groq_client.chat.completions.create(
                    messages=[{"role": "user", "content": prompt}],
                    model=GROQ_MODEL,
                    temperature=0.2
                )
            )
            latency = int((time.time() - start_time) * 1000)
            
            usage = getattr(response, "usage", None)
            prompt_tokens = getattr(usage, "prompt_tokens", None) if usage else None
            completion_tokens = getattr(usage, "completion_tokens", None) if usage else None
            
            # Log Groq call
            asyncio.create_task(log_model_call(
                service_name="copilot",
                provider="groq",
                model=GROQ_MODEL,
                status="SUCCESS",
                latency_ms=latency,
                prompt_tokens=prompt_tokens,
                completion_tokens=completion_tokens,
                org_id=org_id,
                correlation_id=correlation_id
            ))
            return getattr(response.choices[0].message, "content", "").strip() if getattr(response, "choices", None) else ""
        except Exception as e:
            logger.error(f"Groq generation failed: {e}")
            asyncio.create_task(log_model_call(
                service_name="copilot",
                provider="groq",
                model=GROQ_MODEL,
                status="FAILED",
                latency_ms=0,
                org_id=org_id,
                correlation_id=correlation_id
            ))

    raise LLMUnavailableError("All LLM providers failed or none is configured (set GOOGLE_API_KEY / GROQ_API_KEY).")
