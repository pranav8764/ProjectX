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

Folder: `services/api`

Owns:

- Public API routing
- Authentication and authorization checks
- Organization, user, and role data
- Document metadata APIs
- Upload orchestration
- Document status lifecycle

## Retrieval and AI

Folder: `services/ai`

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

- `services/api`
- `services/ai`

Owns:

- Asset APIs
- Asset profiles
- Asset timelines
- Entity deduplication
- Graph entities and relationships
- Graph expansion for retrieval

## RCA and Compliance

Folders:

- `services/api`
- `services/ai`

Owns:

- RCA and compliance agents
- Compliance requirements
- Evidence matching
- Gap detection
- RCA report records

## Reports, Audit, and Notifications

Folder: `services/api`

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
- Future Kubernetes and Helm manifests
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
