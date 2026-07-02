# Architecture

## High-Level Microservice System

```text
User
  -> Web App
  -> API Gateway
  -> Domain Microservices
  -> Service-owned data schemas
  -> Event Bus
  -> Background Workers
```

## Core Services

### Web App

Responsible for dashboard, document upload, copilot, asset pages, RCA assistant, compliance views, and reports.

### API Gateway

Responsible for public REST APIs, request validation, authentication handoff, route aggregation, frontend-specific response shaping, rate limiting, and permission-aware calls into internal services.

### Identity Service

Responsible for organizations, users, roles, memberships, auth provider mapping, and service-to-service permission checks.

### Document Service

Responsible for document metadata, upload sessions, versions, statuses, file pointers, duplicate detection, and document lifecycle state.

### Ingestion Worker

Responsible for file parsing, OCR fallback, markdown conversion, table extraction, chunking, entity extraction, embedding generation, and publishing processing events.

### AI Orchestrator Service

Responsible for LLM provider selection, prompt templates, safety policies, answer validation helpers, and common model calls used by RAG, RCA, and compliance services.

### RAG Service

Responsible for hybrid retrieval, reranking, context building, cited answer generation, confidence scoring, and query feedback.

### Asset Service

Responsible for asset records, aliases, asset profile aggregation, timelines, risk scores, and asset-centric APIs.

### Graph Service

Responsible for extracted entities, relationships, graph expansion, deduplication, and graph visualization APIs.

### RCA Service

Responsible for root cause analysis workflows, failure timelines, probable causes, recommendations, missing-data warnings, and cited RCA reports.

### Compliance Service

Responsible for compliance requirements, evidence checks, expired/missing document detection, gap severity, and compliance dashboards.

### Report Service

Responsible for rendering PDF, DOCX, and CSV exports.

### Audit Service

Responsible for append-only audit events, document access logs, permission-sensitive activity logs, and admin audit APIs.

### Notification Service

Responsible for asynchronous alerts, processing-failure notifications, compliance-gap alerts, and future email/Slack hooks.

## Data Ownership

For local development, services share one PostgreSQL instance with separate schemas. For production, each service can move to its own database without changing the service boundary.

```text
identity-service      -> identity schema
document-service      -> document schema
ingestion-worker      -> ingestion schema
rag-service           -> rag schema
asset-service         -> asset schema
graph-service         -> graph schema
rca-service           -> rca schema
compliance-service    -> compliance schema
report-service        -> report schema
audit-service         -> audit schema
notification-service  -> notification schema
```

## Storage

Object storage stores original uploaded files, normalized markdown, extracted tables, generated reports, and OCR artifacts.

## Event Bus

Redis Streams are used for the local MVP. The same contracts can later move to Kafka, NATS, SQS, or Pub/Sub.

Important events:

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

The gateway talks synchronously to user-facing services. Heavy processing flows are async.

```text
Web -> API Gateway -> Document Service
Web -> API Gateway -> RAG Service
Web -> API Gateway -> Asset Service
Web -> API Gateway -> RCA Service
Web -> API Gateway -> Compliance Service
Web -> API Gateway -> Report Service
```

## Processing Pipeline

```text
Upload request
  -> API Gateway
  -> Document Service creates upload session
  -> Object Storage stores original file
  -> Document Service emits document.uploaded
  -> Ingestion Worker extracts text/OCR/tables
  -> Ingestion Worker chunks text and generates embeddings
  -> Graph Service stores entities and relationships
  -> Asset Service upserts detected assets
  -> Document Service marks document indexed
  -> Audit Service records processing lifecycle
```
