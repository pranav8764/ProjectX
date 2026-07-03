# Deployment

## Local MVP

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

## Kubernetes

Kubernetes and Helm folders are future deployment scaffolds. Do not add manifests for logical modules unless there is a real container and health endpoint behind them.
