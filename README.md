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
    api/                          Go API/BFF: auth, uploads, assets, compliance, reports
    ai/                           Python AI service: OCR, chunking, embeddings, RAG, RCA
  packages/
    shared/                       Shared types, schemas, constants
  infra/
    db/                           Database migrations and seed data
    docker/                       Docker helper files
    k8s/                          Kubernetes manifests
    observability/                Logs, metrics, tracing config
    terraform/                    Future cloud infrastructure
  docs/                           Product, architecture, API, and team docs
  data/
    sample-documents/             Demo PDFs, scans, CSVs, and reports
    processed/                    Generated local processing outputs
  scripts/                        Developer and automation scripts
  tests/                          End-to-end tests and fixtures
```

## Recommended Team Split

- Frontend: `apps/web`
- API, auth, uploads, assets, compliance, reports: `services/api`
- OCR, parsing, extraction, embeddings, RAG, RCA: `services/ai`
- Service contracts: `contracts`
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

Install the web dependencies once:

```bash
npm --prefix apps/web ci
```

Then start the consolidated local stack:

```bash
make services-up
make db-seed
npm --prefix apps/web run dev
```

Primary starting points:

- `apps/web`: Next.js + TypeScript application
- `services/api`: Go API/BFF
- `services/ai`: Python FastAPI AI and ingestion service
- `infra/db`: PostgreSQL + pgvector migrations and demo seeds

Local service URLs:

- Web app: `http://localhost:3000`
- API/BFF: `http://localhost:8080`
- AI service: `http://localhost:8000`
- MinIO console: `http://localhost:9001`

The Go API seeds a local development session token, `dev-token`, when the database is empty and `APP_ENV` is not `production`.

For the complete service map, read `docs/MICROSERVICES.md`. For deployment commands, read `docs/DEPLOYMENT.md`.
