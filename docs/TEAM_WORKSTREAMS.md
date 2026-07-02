# Team Workstreams

## Frontend

Folder: `apps/web`

Owns:

- Dashboard
- Document upload UI
- Processing status UI
- Copilot chat
- Asset profile
- RCA assistant
- Compliance page
- Reports and graph view

## Backend API

Folders:

- `services/api-gateway`
- `services/identity-service`
- `services/document-service`

Owns:

- Public API routing
- Authentication and authorization checks
- Organization, user, and role data
- Document metadata APIs
- Upload orchestration
- Document status lifecycle

## Retrieval and AI

Folders:

- `services/ingestion-worker`
- `services/ai-orchestrator-service`
- `services/rag-service`

Owns:

- OCR and text extraction
- Document classification
- Chunking
- Embeddings
- Hybrid retrieval
- Citation validation
- LLM provider integration
- Prompt and safety templates

## Asset and Graph Intelligence

Folders:

- `services/asset-service`
- `services/graph-service`

Owns:

- Asset APIs
- Asset profiles
- Asset timelines
- Entity deduplication
- Graph entities and relationships
- Graph expansion for retrieval

## RCA and Compliance

Folders:

- `services/rca-service`
- `services/compliance-service`

Owns:

- RCA and compliance agents
- Compliance requirements
- Evidence matching
- Gap detection
- RCA report records

## Reports, Audit, and Notifications

Folders:

- `services/report-service`
- `services/audit-service`
- `services/notification-service`

Owns:

- Export jobs
- Generated report metadata
- Document access logs
- Admin audit events
- Processing and compliance alerts

## Database and Infra

Folder: `infra`

Owns:

- PostgreSQL schemas
- pgvector setup
- Local Docker Compose
- Seed data
- Storage configuration
- Queue configuration
- Kubernetes placeholders
- Observability config

## Demo and QA

Folders:

- `data`
- `tests`
- `docs`
- `contracts`

Owns:

- Sample documents
- Demo script
- Evaluation questions
- Test fixtures
- Acceptance checks
- Contract examples
