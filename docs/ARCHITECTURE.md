# Architecture

## Consolidated MVP Runtime

PlantBrainAI is domain-oriented, but the current MVP has only two runnable backend services:

```text
User
  -> apps/web
  -> services/api
  -> services/ai
  -> PostgreSQL + pgvector
  -> MinIO
```

`services/api` is the public Go API/BFF. It owns auth enforcement, upload orchestration, document metadata, asset and compliance reads, dashboard metrics, report jobs, and frontend-shaped responses.

`services/ai` is the internal Python FastAPI service. It owns OCR/text extraction, markdown normalization, table extraction, chunking, entity extraction, embeddings, RAG answers, RCA generation, compliance scans, and graph writes.

PostgreSQL schemas keep domain ownership explicit while the runtime is consolidated:

```text
services/api -> identity, document, asset, compliance, report, rag, rca
services/ai  -> ingestion, graph, rag, rca, compliance, ai
future       -> audit, notification
```

## Logical Modules

The original product plan used names such as Document Service, RAG Service, Graph Service, RCA Service, and Compliance Service. In this repository those are logical modules, not folders under `services/`.

| Logical module | Runtime owner | Responsibilities |
| --- | --- | --- |
| Identity | `services/api` | Users, roles, memberships, auth provider mapping, permission context |
| Documents | `services/api` | Uploads, versions, status, file pointers, duplicate detection |
| Ingestion | `services/ai` | Parsing, OCR fallback, markdown conversion, tables, chunks, embeddings |
| RAG | `services/api`, `services/ai` | Query routing, retrieval, cited answer generation, confidence and feedback |
| Assets | `services/api`, `services/ai` | Asset records, aliases, profile aggregation, timelines, risk fields |
| Graph | `services/ai` | Extracted entities, relationships, deduplication, graph expansion |
| RCA | `services/api`, `services/ai` | Failure context, probable causes, recommendations, cited RCA records |
| Compliance | `services/api`, `services/ai` | Requirements, evidence matching, gap detection, dashboard reads |
| Reports | `services/api` | CSV export jobs for the MVP; PDF/DOCX can be added later |
| Audit and notifications | `services/api` now, future split if needed | Audit events and processing/compliance alerts |

## Storage

Object storage stores original uploaded files. The local Docker stack uses MinIO plus a shared `uploads-data` volume mounted at `/app/uploads` so the API can save files and the AI service can process them.

PostgreSQL stores metadata, chunks, embeddings through pgvector, graph entities, query logs, RCA reports, compliance gaps, report jobs, audit events, and development auth data.

## Async Direction

The current MVP calls `services/ai` over HTTP for document processing, RAG, RCA, and compliance scans. `contracts/events/asyncapi.yaml` defines the event vocabulary for a queue-backed version using Redis Streams first, then Kafka/NATS/SQS if volume requires it.

Important planned events:

- `document.uploaded`
- `document.processing_started`
- `document.text_extracted`
- `document.entities_extracted`
- `document.indexed`
- `document.processing_failed`
- `asset.upserted`
- `compliance.gap_detected`
- `rca.report_generated`
- `report.generated`
- `audit.event_recorded`

## Synchronous Calls

```text
apps/web -> services/api
services/api -> services/ai
services/api -> PostgreSQL
services/ai -> PostgreSQL + pgvector
services/api -> MinIO
services/ai -> shared uploads volume
```

External clients should call only `services/api`. The AI service is internal to the compose network and should not become the browser-facing API.

## Processing Pipeline

```text
Upload request
  -> services/api validates auth, file type, plant, and document type
  -> services/api stores metadata in document schema
  -> services/api writes original file to MinIO and /app/uploads
  -> services/api calls services/ai /process-document
  -> services/ai extracts text or runs OCR fallback
  -> services/ai creates markdown, tables, chunks, entities, and embeddings
  -> services/ai writes ingestion and graph data
  -> services/ai upserts detected assets
  -> services/ai marks document status COMPLETED or FAILED
```

## Split Criteria

Create a new runtime service only when the MVP has a concrete need such as independent scaling, separate ownership with active code, distinct deployment lifecycle, or stronger isolation. Until then, keep domain code inside `services/api` or `services/ai` and update contracts first when request, response, or event shapes change.
