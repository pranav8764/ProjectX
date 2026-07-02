# Microservice Architecture

This project is structured as a domain-oriented microservice system. Each service owns a clear business capability, a data schema, API contracts, and event contracts.

## Service Topology

```text
apps/web
  |
  v
services/api-gateway
  |
  |-- services/identity-service
  |-- services/document-service
  |-- services/rag-service
  |-- services/asset-service
  |-- services/graph-service
  |-- services/rca-service
  |-- services/compliance-service
  |-- services/report-service
  |-- services/audit-service
  |-- services/notification-service
  |
  v
PostgreSQL schemas + pgvector, Redis Streams, MinIO

Async workers:
  services/ingestion-worker
  services/ai-orchestrator-service
```

## Services

| Service | Port | Owns | Data schema |
| --- | ---: | --- | --- |
| `api-gateway` | 8080 | Public APIs, BFF aggregation, auth enforcement | none |
| `identity-service` | 8081 | Users, orgs, roles, memberships | `identity` |
| `document-service` | 8082 | Documents, versions, upload sessions, status | `document` |
| `rag-service` | 8083 | Queries, citations, retrieval feedback | `rag` |
| `asset-service` | 8084 | Assets, aliases, profiles, risk summaries | `asset` |
| `graph-service` | 8085 | Entities, relationships, graph expansion | `graph` |
| `rca-service` | 8086 | RCA reports and workflows | `rca` |
| `compliance-service` | 8087 | Requirements, evidence, gaps | `compliance` |
| `report-service` | 8088 | Export jobs and generated report metadata | `report` |
| `audit-service` | 8089 | Audit logs and access events | `audit` |
| `notification-service` | 8090 | Alerts, notifications, delivery state | `notification` |
| `ai-orchestrator-service` | 8091 | LLM provider calls, prompts, safety helpers | `ai` |
| `ingestion-worker` | 8092 | OCR, parsing, chunking, embeddings | `ingestion` |

## Communication Rules

- External clients only call `api-gateway`.
- Services communicate internally through REST for reads/commands that need immediate responses.
- Long-running document processing happens through events.
- Services must not directly write to another service's schema.
- Cross-service read models should be built through events or gateway aggregation.

## MVP Deployment Style

For the hackathon/MVP, use one monorepo and one local PostgreSQL instance with separate schemas. This keeps development fast while preserving the ability to split services later.

## Production Direction

When the MVP is stable:

- Move high-traffic services to separate deploy units.
- Move from Redis Streams to Kafka/NATS/SQS if event volume grows.
- Split PostgreSQL schemas into separate databases where isolation is required.
- Add OpenTelemetry tracing across gateway, service calls, and workers.
- Add service-to-service auth with signed internal tokens or mTLS.

