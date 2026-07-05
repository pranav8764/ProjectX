import asyncio
import hashlib
import time
import numpy as np
from typing import List, Optional
from google.genai import types
from app.config import GOOGLE_API_KEY, gemini_client, groq_client, logger
import app.database as database
from app.utils.helpers import optional_uuid

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

async def get_gemini_embedding_1536(text: str, org_id: Optional[str] = None, correlation_id: Optional[str] = None) -> List[float]:
    """
    Generates 1536-dimensional embeddings using Gemini text-embedding-004.
    Since text-embedding-004 produces 768 dimensions by default, we concatenate the vector
    with itself to yield exactly 1536 dimensions. This preserves cosine similarity mathematically.
    """
    if not GOOGLE_API_KEY or not gemini_client:
        return deterministic_embedding_1536(text)

    try:
        start_time = time.time()
        response = await gemini_client.aio.models.embed_content(
            model="text-embedding-004",
            contents=text,
            config=types.EmbedContentConfig(
                task_type="RETRIEVAL_DOCUMENT"
            )
        )
        latency = int((time.time() - start_time) * 1000)
        
        # Log embedding call
        asyncio.create_task(log_model_call(
            service_name="copilot",
            provider="google",
            model="text-embedding-004",
            status="SUCCESS",
            latency_ms=latency,
            org_id=org_id,
            correlation_id=correlation_id
        ))

        emb_768 = response.embeddings[0].values
        emb_1536 = emb_768 + emb_768
        return emb_1536
    except Exception as e:
        logger.error(f"Error generating Gemini embedding: {e}")
        asyncio.create_task(log_model_call(
            service_name="copilot",
            provider="google",
            model="text-embedding-004",
            status="FAILED",
            latency_ms=0,
            org_id=org_id,
            correlation_id=correlation_id
        ))
        return deterministic_embedding_1536(text)

async def get_gemini_embeddings_1536_batch(texts: List[str], org_id: Optional[str] = None, correlation_id: Optional[str] = None) -> List[List[float]]:
    """
    Generates 1536-dimensional embeddings for a list of texts in a single batch call to reduce roundtrips.
    """
    if not texts:
        return []

    if not GOOGLE_API_KEY or not gemini_client:
        return [deterministic_embedding_1536(t) for t in texts]

    try:
        start_time = time.time()
        response = await gemini_client.aio.models.embed_content(
            model="text-embedding-004",
            contents=texts,
            config=types.EmbedContentConfig(
                task_type="RETRIEVAL_DOCUMENT"
            )
        )
        latency = int((time.time() - start_time) * 1000)
        
        # Log batch embedding call
        asyncio.create_task(log_model_call(
            service_name="ingestion",
            provider="google",
            model="text-embedding-004",
            status="SUCCESS",
            latency_ms=latency,
            org_id=org_id,
            correlation_id=correlation_id
        ))

        results = []
        for emb in response.embeddings:
            emb_768 = emb.values
            emb_1536 = emb_768 + emb_768
            results.append(emb_1536)
        return results
    except Exception as e:
        logger.error(f"Error generating Gemini batch embeddings: {e}")
        asyncio.create_task(log_model_call(
            service_name="ingestion",
            provider="google",
            model="text-embedding-004",
            status="FAILED",
            latency_ms=0,
            org_id=org_id,
            correlation_id=correlation_id
        ))
        return [deterministic_embedding_1536(t) for t in texts]

async def generate_text_llm(prompt: str, org_id: Optional[str] = None, correlation_id: Optional[str] = None) -> str:
    """Helper to generate text using Gemini-1.5-pro or Groq Llama3 with token usage logging"""
    if GOOGLE_API_KEY and gemini_client:
        try:
            start_time = time.time()
            response = await gemini_client.aio.models.generate_content(
                model='gemini-1.5-pro',
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
                model="gemini-1.5-pro",
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
                model="gemini-1.5-pro",
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
                    model="llama3-70b-8192",
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
                model="llama3-70b-8192",
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
                model="llama3-70b-8192",
                status="FAILED",
                latency_ms=0,
                org_id=org_id,
                correlation_id=correlation_id
            ))

    return "AI generation unavailable. Please check API keys configuration."
