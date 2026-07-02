# PlantBrainAI

PlantBrainAI is an AI-powered industrial knowledge platform that turns scattered plant documents into searchable, cited asset intelligence.

## MVP Goal

Build a demo-ready system where users can upload industrial documents, extract knowledge, ask asset-specific questions, view cited answers, inspect asset profiles, generate RCA suggestions, and detect basic compliance gaps.

## Repository Layout

```text
ProjectX/
  apps/
    web/                          Next.js frontend
  contracts/
    openapi/                      REST API contracts
    events/                       Async/event contracts
    schemas/                      Shared JSON schemas
    proto/                        Future gRPC/protobuf contracts
  services/
    api-gateway/                  Public API gateway and BFF
    identity-service/             Auth, users, orgs, roles
    document-service/             Document metadata, upload lifecycle
    ingestion-worker/             Parsing, OCR, chunking, embeddings
    ai-orchestrator-service/      LLM provider, prompts, safety policies
    rag-service/                  Retrieval, citations, copilot answers
    asset-service/                Assets, tags, profiles, timelines
    graph-service/                Knowledge graph entities/relations
    rca-service/                  Root cause analysis workflows
    compliance-service/           Compliance checks and gaps
    report-service/               PDF/DOCX/CSV report generation
    audit-service/                Audit trail and access events
    notification-service/         Alerts and async user notifications
  packages/
    shared/                       Shared types, schemas, constants
  infra/
    db/                           Database migrations and seed data
    docker/                       Docker helper files
    k8s/                          Kubernetes manifests
    observability/                Logs, metrics, tracing config
    terraform/                    Cloud infrastructure placeholders
  docs/                           Product, architecture, API, and team docs
  data/
    sample-documents/             Demo PDFs, scans, CSVs, and reports
    processed/                    Generated local processing outputs
  scripts/                        Developer and automation scripts
  tests/                          End-to-end tests and fixtures
```

## Recommended Team Split

- Frontend: `apps/web`
- API gateway and service contracts: `services/api-gateway`, `contracts`
- Auth and workspace management: `services/identity-service`
- Document upload and metadata: `services/document-service`
- OCR, parsing, extraction, embeddings: `services/ingestion-worker`
- RAG and AI behavior: `services/rag-service`, `services/ai-orchestrator-service`
- Asset intelligence and graph: `services/asset-service`, `services/graph-service`
- RCA and compliance: `services/rca-service`, `services/compliance-service`
- Reports, audit, notifications: `services/report-service`, `services/audit-service`, `services/notification-service`
- Database, deployment, observability: `infra`
- Demo data and evaluation: `data`, `tests`, `docs`

## First MVP Build Order

1. Document upload
2. Text extraction/OCR
3. Chunking and embeddings
4. RAG chatbot with citations
5. Entity extraction
6. Asset profile
7. RCA assistant
8. Compliance gap checker
9. Dashboard
10. Report export and graph visualization

## Getting Started

Copy the environment template:

```bash
cp .env.example .env
```

Then each team can initialize their own stack inside the relevant folder.

Suggested starting points:

- `apps/web`: Next.js + TypeScript
- `services/api-gateway`: Go, Node.js, or FastAPI API gateway
- `services/ingestion-worker`: Python workers for OCR, extraction, embeddings
- `services/rag-service`: Python/FastAPI or Node service for hybrid retrieval and answer generation
- `infra/db`: PostgreSQL + pgvector migrations

For the complete service map, read `docs/MICROSERVICES.md`.
