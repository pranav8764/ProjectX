# Consolidated MVP Service Architecture

The original product plan is domain-oriented, but this MVP uses two runnable backend services to avoid dead service directories and keep the demo deployable. Data is still separated by PostgreSQL schemas so the system can be split into microservices later if needed.

## Service Topology

```text
apps/web
  |
  v
services/api
  |
  |-- PostgreSQL schemas: identity, document, ingestion, asset, graph, rag, rca, compliance, report
  |-- Redis
  |-- MinIO
  |
  v
services/ai
  |
  v
PostgreSQL + pgvector
```

## Services

| Service | Port | Owns | Data schema |
| --- | ---: | --- | --- |
| `api` | 8080 | Public API/BFF, auth enforcement, document upload, asset/compliance/report reads | `identity`, `document`, `asset`, `compliance`, `report`, `rag`, `rca` |
| `ai` | 8000 | OCR, parsing, table extraction, chunking, entity extraction, embeddings, RAG, RCA, compliance scans | `ingestion`, `graph`, `rag`, `rca`, `compliance`, `ai` |

The old per-domain names are logical modules in this MVP, not service folders. Keep new code in `services/api` or `services/ai` unless the team deliberately creates a real deployable service.

## Communication Rules

- External clients only call `services/api`.
- `services/api` calls `services/ai` for document processing, RAG, and RCA.
- Uploads are saved to a shared Docker volume and MinIO so the API and AI service can process the same file.
- PostgreSQL schemas preserve domain boundaries even though the runtime is consolidated.

## MVP Deployment Style

Use one monorepo, one local PostgreSQL instance with separate schemas, Redis, MinIO, the Go API, and the Python AI service.

Canonical runtime contracts live in `contracts/openapi/api.yaml` and `contracts/openapi/ai.yaml`. Do not add split-service OpenAPI files unless the same change introduces a real deployable service.

## Production Direction

When the MVP is stable:

- Move high-traffic services to separate deploy units.
- Move from Redis Streams to Kafka/NATS/SQS if event volume grows.
- Split PostgreSQL schemas into separate databases where isolation is required.
- Add OpenTelemetry tracing across gateway, service calls, and workers.
- Add service-to-service auth with signed internal tokens or mTLS.
