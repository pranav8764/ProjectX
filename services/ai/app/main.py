import os
import time
import threading
from collections import defaultdict
from contextlib import asynccontextmanager
from fastapi import FastAPI, Request
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse
from starlette.middleware.base import BaseHTTPMiddleware
import uvicorn
import sys

from app.database import init_db_pool, close_db_pool
import app.database as database
from app.routes import ingestion, query, rca, compliance

# Import schemas, pipelines and utility functions to expose in main namespace for backward compatibility
from app.schemas import CopilotQueryRequest, RCAGenerateRequest, DocumentProcessRequest
from app.routes.ingestion import run_ingestion_pipeline
from app.routes.query import rag_query
from app.routes.rca import generate_rca
from app.routes.compliance import audit_compliance
from app.utils.ai_clients import get_gemini_embedding_1536

class TokenBucketLimiter:
    def __init__(self, rate: float, capacity: float):
        self.rate = rate
        self.capacity = capacity
        self.buckets = defaultdict(lambda: (capacity, time.time()))
        self.lock = threading.Lock()

    def allow(self, key: str) -> tuple[bool, float]:
        with self.lock:
            now = time.time()
            tokens, last_update = self.buckets[key]
            
            elapsed = now - last_update
            replenished = elapsed * self.rate
            tokens = min(self.capacity, tokens + replenished)
            
            if tokens >= 1.0:
                self.buckets[key] = (tokens - 1.0, now)
                return True, 0.0
            else:
                self.buckets[key] = (tokens, now)
                needed = 1.0 - tokens
                retry_after = needed / self.rate
                return False, retry_after

class RateLimitMiddleware(BaseHTTPMiddleware):
    def __init__(self, app, rate: float = 5.0, capacity: float = 20.0):
        super().__init__(app)
        self.limiter = TokenBucketLimiter(rate, capacity)

    async def dispatch(self, request: Request, call_next):
        if request.url.path == "/health":
            return await call_next(request)
            
        key = request.headers.get("x-user-id") or request.client.host or "unknown"
        allowed, retry_after = self.limiter.allow(key)
        if not allowed:
            return JSONResponse(
                status_code=429,
                content={
                    "detail": "Too many requests. Please try again later.",
                    "retry_after": f"{retry_after:.2f}s"
                },
                headers={"Retry-After": f"{int(retry_after) or 1}"}
            )
        return await call_next(request)

@asynccontextmanager
async def lifespan(app: FastAPI):
    # Startup: Initialize the database pool
    await init_db_pool()
    yield
    # Shutdown: Close the database pool
    await close_db_pool()

app = FastAPI(title="PlantBrainAI Consolidated AI Service", version="1.0.0", lifespan=lifespan)

# Setup CORS Policy
allowed_origins_str = os.getenv("CORS_ALLOWED_ORIGINS", "")
allowed_origins = [origin.strip() for origin in allowed_origins_str.split(",") if origin.strip()]
if not allowed_origins:
    allowed_origins = ["http://localhost:3000"]

app.add_middleware(
    CORSMiddleware,
    allow_origins=allowed_origins,
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# Setup Rate-Limiting Policy
AI_RATE_LIMIT_RPS = float(os.getenv("AI_RATE_LIMIT_RPS", "5.0"))
AI_RATE_LIMIT_BURST = float(os.getenv("AI_RATE_LIMIT_BURST", "20.0"))

app.add_middleware(
    RateLimitMiddleware,
    rate=AI_RATE_LIMIT_RPS,
    capacity=AI_RATE_LIMIT_BURST
)

@app.get("/health")
async def health_check():
    return {"status": "healthy", "service": "ai-service"}

# Include modular routes
app.include_router(ingestion.router)
app.include_router(query.router)
app.include_router(rca.router)
app.include_router(compliance.router)

# Class wrapper to proxy db_pool access dynamically to app.database.db_pool (used in unit tests)
class MainModuleWrapper(object):
    def __init__(self, wrapped):
        self.__dict__['wrapped'] = wrapped

    def __getattr__(self, name):
        if name == 'db_pool':
            return database.db_pool
        return getattr(self.wrapped, name)

    def __setattr__(self, name, value):
        if name == 'db_pool':
            database.db_pool = value
        else:
            setattr(self.wrapped, name, value)

sys.modules[__name__] = MainModuleWrapper(sys.modules[__name__])

if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=8000)
