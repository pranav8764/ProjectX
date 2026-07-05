import asyncio
import hashlib
import numpy as np
from typing import List
from google.genai import types
from app.config import GOOGLE_API_KEY, gemini_client, groq_client, logger

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

async def get_gemini_embedding_1536(text: str) -> List[float]:
    """
    Generates 1536-dimensional embeddings using Gemini text-embedding-004.
    Since text-embedding-004 produces 768 dimensions by default, we concatenate the vector
    with itself to yield exactly 1536 dimensions. This preserves cosine similarity mathematically.
    """
    if not GOOGLE_API_KEY or not gemini_client:
        return deterministic_embedding_1536(text)

    try:
        response = await gemini_client.aio.models.embed_content(
            model="text-embedding-004",
            contents=text,
            config=types.EmbedContentConfig(
                task_type="RETRIEVAL_DOCUMENT"
            )
        )
        emb_768 = response.embeddings[0].values
        # Concatenate vector with itself to reach 1536 dimensions
        emb_1536 = emb_768 + emb_768
        return emb_1536
    except Exception as e:
        logger.error(f"Error generating Gemini embedding: {e}")
        return deterministic_embedding_1536(text)

async def generate_text_llm(prompt: str) -> str:
    """Helper to generate text using Gemini-1.5-pro or Groq Llama3"""
    if GOOGLE_API_KEY and gemini_client:
        try:
            response = await gemini_client.aio.models.generate_content(
                model='gemini-1.5-pro',
                contents=prompt
            )
            return response.text.strip()
        except Exception as e:
            logger.warning(f"Gemini generation failed: {e}. Falling back to Groq...")

    if groq_client:
        try:
            loop = asyncio.get_event_loop()
            response = await loop.run_in_executor(
                None,
                lambda: groq_client.chat.completions.create(
                    messages=[{"role": "user", "content": prompt}],
                    model="llama3-70b-8192",
                    temperature=0.2
                )
            )
            return response.choices[0].message.content.strip()
        except Exception as e:
            logger.error(f"Groq generation failed: {e}")

    return "AI generation unavailable. Please check API keys configuration."
