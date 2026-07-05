# Service Catalog

## API

- Folder: `services/api`
- Type: Go HTTP service
- Public: yes
- Port: 8080
- Owns: authentication enforcement, document upload, document registry, asset profiles, compliance gaps, report jobs, frontend BFF routes
- Uses schemas: `identity`, `document`, `asset`, `compliance`, `report`, `rag`, `rca`
- Calls: `services/ai`
- Contract: `contracts/openapi/api.yaml`
- Implementation reference: consolidated API routes are implemented in `services/api/cmd/server/main.go`.

## AI

- Folder: `services/ai`
- Type: Python FastAPI service
- Public: internal
- Port: 8000
- Owns: OCR/text extraction, table extraction, markdown normalization, chunking, entity extraction, embedding generation, vector search, cited RAG answers, RCA generation, compliance scans
- Uses schemas: `ingestion`, `graph`, `rag`, `rca`, `compliance`, `ai`
- Contract: `contracts/openapi/ai.yaml`
- Implementation reference: implemented FastAPI routers live under `services/ai/app/routes/` and are registered in `services/ai/app/main.py`.

## Supporting Runtime

- PostgreSQL + pgvector stores domain schemas and embeddings.
- Redis is available for future queues/events.
- MinIO stores original uploads.
- A shared Docker volume mounted at `/app/uploads` lets `services/api` save files and `services/ai` process them during the MVP.
- Runtime OpenAPI contracts are limited to `contracts/openapi/api.yaml` and `contracts/openapi/ai.yaml`.
