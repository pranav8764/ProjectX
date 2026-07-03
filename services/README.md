# Services

PlantBrainAI currently uses a consolidated MVP backend. External clients call the Go API/BFF, and long-running AI/document intelligence work is handled by the Python AI service.

## Service List

| Service | Purpose |
| --- | --- |
| `api` | Public API/BFF, dev auth, document upload, asset profiles, compliance gaps, report jobs |
| `ai` | OCR/text extraction, table extraction, chunking, entity extraction, embeddings, RAG answers, RCA, compliance scan |

Do not add empty folders for individual domains. Document, asset, graph, RAG, RCA, compliance, report, audit, and notification are logical modules inside the consolidated runtime until the MVP needs a real service split.

See `docs/SERVICE_CATALOG.md` for ownership details and `contracts/openapi/README.md` for current versus legacy contracts.
