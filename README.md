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

The Docker Compose stack has local defaults for Postgres, Redis, MinIO, the Go API, and the Python AI service. Copy `.env.example` to `.env` in the root directory and fill in the required variables before starting. Add `GOOGLE_API_KEY` and/or `GROQ_API_KEY` to your `.env` only when you want live LLM/embedding calls instead of fallback responses.

### Environment Variables Guide

Here is a list of environment variables defined in `.env.example` and instructions on how to acquire or generate them:

#### Core Application
- `APP_NAME` (default: `PlantBrainAI`): The name of your application.
- `APP_ENV` (default: `development`): Environment setting. Set to `production` or `test` when deploying.
- `APP_URL` (default: `http://localhost:3000`): The base URL of the frontend web application.

#### Authentication
- `AUTH_PROVIDER` (default: `betterauth`): Configures the auth provider.
- `BETTER_AUTH_SECRET`: Secret key used for cryptographic signing in Better Auth.
  * **How to acquire**: Run `openssl rand -hex 32` in your terminal to generate a secure secret key, or refer to [Better Auth Secret Docs](https://www.better-auth.com/docs/installation#getting-started).
- `BETTER_AUTH_URL` (default: `http://localhost:3000`): Base URL for the Better Auth backend.
- `JWT_SECRET`: Secret key for JWT signing.
  * **How to acquire**: Generate a secure string using `openssl rand -hex 32`.
- `CORS_ALLOWED_ORIGINS` (default: `http://localhost:3000`): A comma-separated list of domains allowed to request resources.

#### Database
- `DATABASE_URL`: Connection URL for the PostgreSQL instance.
  * **Format**: `postgresql://[user]:[password]@[host]:[port]/[database]`
  * **Default**: `postgresql://plantbrain:plantbrain@localhost:5432/plantbrain` (matches compose settings).
- `POSTGRES_DB` (default: `plantbrain`): The name of the database.
- `POSTGRES_USER` (default: `plantbrain`): The database user.
- `POSTGRES_PASSWORD`: The password for the database user. Secure random password generated for development.

#### Vector Search
- `VECTOR_DB` (default: `pgvector`): Set to `pgvector` to enable vector database queries for semantic embeddings in PostgreSQL.

#### Object Storage (S3 / MinIO)
- `STORAGE_PROVIDER` (default: `local`): Storage provider. Set to `minio` or `s3` to enable bucket uploads.
- `STORAGE_BUCKET` (default: `plantbrain-documents`): Bucket name.
- `S3_ENDPOINT`: S3-compatible service URL (e.g. `http://localhost:9000` for local MinIO or `http://minio:9000` inside Docker).
- `S3_ACCESS_KEY_ID`: S3/MinIO Access Key.
  * **How to acquire**: From the [MinIO Console](http://localhost:9001) / AWS IAM Access Key dashboard.
- `S3_SECRET_ACCESS_KEY`: S3/MinIO Secret Access Key.
  * **How to acquire**: Pair credential generated alongside the Access Key ID.
- `MINIO_ROOT_USER` (default: `plantbrain`): MinIO admin console username.
- `MINIO_ROOT_PASSWORD` (default: `plantbrain_minio_secret_pass`): MinIO admin console password.

#### AI Providers
- `LLM_PROVIDER` (options: `gemini`, `groq`): Selects the provider for the LLM assistant.
- `LLM_API_KEY`: API key for the selected LLM provider.
- `EMBEDDING_PROVIDER` (options: `gemini`): Selects the provider for vector embeddings.
- `EMBEDDING_API_KEY`: API key for the embeddings provider.
  * **How to acquire Gemini Key**: Obtain from the [Google AI Studio Console](https://aistudio.google.com/).
  * **How to acquire Groq Key**: Obtain from the [Groq Console](https://console.groq.com/).

#### Services & Event Streaming
- `WEB_URL` (default: `http://localhost:3000`): Next.js web application root.
- `REDIS_URL` (default: `redis://localhost:6379`): Connection URL for the Redis server.
- `EVENT_BUS` (default: `redis-streams`): Set to `redis-streams` to enable asynchronous messaging.
- `EVENT_STREAM_PREFIX` (default: `plantbrain`): Prefix namespace for Redis streams.

#### Observability
- `LOG_LEVEL` (default: `info`): Output log verbosity (`debug`, `info`, `warn`, `error`).
- `OTEL_EXPORTER_OTLP_ENDPOINT` (default: `http://localhost:4318`): Host endpoint for OpenTelemetry Collector to export traces and metrics.


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
- `services/ai`: Python FastAPI AI service
- `infra/db`: PostgreSQL + pgvector migrations and demo seeds

Local service URLs:

- Web app: `http://localhost:3000`
- API/BFF: `http://localhost:8080`
- AI service: `http://localhost:8000`
- MinIO console: `http://localhost:9001`

The Go API seeds a local development session token, `dev-token`, when the database is empty and `APP_ENV` is not `production`.

Useful checks:

```bash
make compose-config
make smoke
make test-api
make test-ai
```

For the complete service map, read `docs/MICROSERVICES.md`. For deployment commands, read `docs/DEPLOYMENT.md`.

## Git Hooks for CI Verification

Local git hooks have been configured to automatically run `make test` (verifying Go, Python AI, and Next.js builds) before you commit or push:
- **Pre-commit**: Checks that your code builds and tests pass before committing.
- **Pre-push**: Checks that your code builds and tests pass before pushing.

These hooks reside in `.git/hooks/pre-commit` and `.git/hooks/pre-push` respectively. If you ever need to bypass these hooks temporarily, you can append `--no-verify` to your git command:
```bash
git commit -m "commit message" --no-verify
git push origin branch-name --no-verify
```
