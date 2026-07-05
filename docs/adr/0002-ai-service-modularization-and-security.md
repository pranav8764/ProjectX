# ADR 0002: AI Service Modularization, Web Containerization, and Security Controls

## Status

Accepted

## Context

As the MVP grew:
1. The `services/ai` Python FastAPI service codebase became a monolithic 64KB `main.py` file. This hindered readability, made unit testing difficult (requiring complex patching of global variables), and complicated code maintenance.
2. The web application (`apps/web`) was run locally on the host machine. To make the stack completely portable and production-representative, it needed to be containerized and managed directly inside Docker Compose.
3. To secure the public Go API and internal Python AI services against traffic surges and cross-origin security threats, we needed robust CORS (Cross-Origin Resource Sharing) policies and rate-limiting controls.

## Decision

1. **Modularize the AI Service**: We split `services/ai/app/main.py` into distinct components:
   - `config.py`: For global client configuration and environmental variables.
   - `schemas.py`: For request/response definitions.
   - `database.py`: For DB pools and lifecycle management.
   - `utils/`: For utility helpers (`helpers.py`) and third-party AI clients (`ai_clients.py` using `google-genai` package).
   - `routes/`: Segmented API routers (`ingestion.py`, `query.py`, `rca.py`, `compliance.py`).
2. **Web Containerization**: Created a multi-stage production Dockerfile for the Next.js application, passing API and AI URL endpoints as build arguments to bake dynamic client-side proxies. Integrated the `web` service directly inside the root `docker-compose.yml`.
3. **CORS and Rate-Limiting Controls**:
   - Implemented standard CORS policies on the Python AI and Go API services, matching allowed request origins via the `CORS_ALLOWED_ORIGINS` environment variable.
   - Created a custom in-memory `TokenBucketLimiter` class and registered `RateLimitMiddleware` in the Python service.
   - Made the Go API's sliding-window rate limiters configurable via environment variables (`API_RATE_LIMIT` and `API_AUTH_RATE_LIMIT`).

## Consequences

- The Python AI codebase is highly maintainable, modular, and easier to unit test.
- The entire application stack (Frontend + BFF API + AI Node + DB + Storage + Cache) can be launched using a single command: `docker compose up --build`.
- Services are secure against rapid requests (DDOS/stress) and unauthorized cross-origin resource requests.
- Rate-limiting rules are fully configurable across development, staging, and production environments using environment variables.
