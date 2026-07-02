# Services

PlantBrainAI backend is split into bounded microservices. External clients should call only `api-gateway`; domain services are internal.

## Service List

| Service | Purpose |
| --- | --- |
| `api-gateway` | Public API gateway and frontend BFF |
| `identity-service` | Organizations, users, roles, memberships |
| `document-service` | Document metadata, uploads, versions, status |
| `ingestion-worker` | OCR, parsing, chunking, embeddings |
| `ai-orchestrator-service` | LLM calls, prompts, model routing, safety helpers |
| `rag-service` | Hybrid retrieval, cited answers, query history |
| `asset-service` | Asset profiles, aliases, risk summaries |
| `graph-service` | Entities, relationships, knowledge graph expansion |
| `rca-service` | Root cause analysis workflows and reports |
| `compliance-service` | Requirements, evidence, compliance gaps |
| `report-service` | PDF, DOCX, CSV export jobs |
| `audit-service` | Immutable audit events and access logs |
| `notification-service` | Alerts and async notifications |

See `docs/SERVICE_CATALOG.md` for ownership details.
