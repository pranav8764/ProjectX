# Deployment

## Local MVP

Compose defaults are self-contained for local infrastructure. Export `GOOGLE_API_KEY` or `GROQ_API_KEY` before `make services-up` if you want live AI calls; otherwise the AI service returns deterministic fallback text where provider calls are unavailable.

Start only shared dependencies:

```bash
make infra-up
```

Start the runnable backend stack:

```bash
make services-up
```

This starts PostgreSQL with pgvector, Redis, MinIO, `services/api`, and `services/ai` from the root `docker-compose.yml`.

Seed demo data after PostgreSQL is running:

```bash
make db-seed
```

Run the web app from the host:

```bash
npm --prefix apps/web ci
npm --prefix apps/web run dev
```

Useful local URLs:

- API health: `http://localhost:8080/health`
- AI health: `http://localhost:8000/health`
- MinIO console: `http://localhost:9001`
- Web app: `http://localhost:3000`

After the stack is up, run a live smoke check:

```bash
make smoke
```

The smoke check calls API and AI `/health`, then verifies `/api/me` with `PLANTBRAIN_API_TOKEN` or the local `dev-token`. Set `PLANTBRAIN_SKIP_AUTH_SMOKE=1` to skip the auth probe for non-dev environments, or `PLANTBRAIN_SMOKE_LIVE_AI=1` to run the live AI endpoint checks from `tests/integration_test.py`.

## Runtime Strategy

The MVP deploys as:

- `apps/web`: Vercel or a Node container
- `services/api`: Go container, public HTTP entry point
- `services/ai`: Python FastAPI container, internal HTTP service
- PostgreSQL + pgvector: managed Postgres or the compose database for demos
- Object storage: S3, Cloudflare R2, or MinIO
- Redis: available for queues/events when async processing moves off direct HTTP

The deleted per-domain service folders should not be recreated as empty deploy units. Domain boundaries are represented by PostgreSQL schemas, docs, and contracts until a real split is justified.

## Compose Validation

Validate local compose changes with:

```bash
make compose-config
```

CI also runs `docker compose config` to catch invalid YAML or service references.

Generated CSV, PDF, and DOCX reports are written under the API upload volume at `/app/uploads/reports` and are downloaded through `GET /api/reports/{id}/download`.

## Kubernetes

Kubernetes and Helm folders are future deployment scaffolds. Do not add manifests for logical modules unless there is a real container and health endpoint behind them.
