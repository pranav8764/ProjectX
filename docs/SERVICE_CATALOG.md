# Service Catalog

## API Gateway

- Folder: `services/api-gateway`
- Type: HTTP service
- Public: yes
- Depends on: all user-facing services
- Contracts: `contracts/openapi/api-gateway.yaml`

## Identity Service

- Folder: `services/identity-service`
- Type: HTTP service
- Public: internal
- Owns: organizations, users, roles, memberships
- Contracts: `contracts/openapi/identity-service.yaml`

## Document Service

- Folder: `services/document-service`
- Type: HTTP service + event producer
- Owns: document metadata, upload sessions, versions, status
- Emits: `document.uploaded`, `document.deleted`, `document.version_created`

## Ingestion Worker

- Folder: `services/ingestion-worker`
- Type: worker + health endpoint
- Owns: extraction jobs, chunks, embeddings
- Consumes: `document.uploaded`
- Emits: `document.text_extracted`, `document.entities_extracted`, `document.indexed`, `document.processing_failed`

## RAG Service

- Folder: `services/rag-service`
- Type: HTTP service
- Owns: queries, citations, feedback
- Depends on: graph, asset, document, ai-orchestrator

## Asset Service

- Folder: `services/asset-service`
- Type: HTTP service + event consumer
- Owns: asset profiles, aliases, risk summaries
- Consumes: `document.entities_extracted`

## Graph Service

- Folder: `services/graph-service`
- Type: HTTP service + event consumer
- Owns: graph entities and relationships
- Consumes: `document.entities_extracted`

## RCA Service

- Folder: `services/rca-service`
- Type: HTTP service
- Owns: RCA workflows and reports
- Depends on: rag, asset, graph, ai-orchestrator

## Compliance Service

- Folder: `services/compliance-service`
- Type: HTTP service + event consumer
- Owns: requirements, evidence, gaps
- Consumes: `document.indexed`, `asset.upserted`

## Report Service

- Folder: `services/report-service`
- Type: HTTP service + worker
- Owns: report jobs and generated artifacts
- Depends on: object storage

## Audit Service

- Folder: `services/audit-service`
- Type: event consumer + HTTP service
- Owns: immutable audit events
- Consumes: all audit-worthy domain events

## Notification Service

- Folder: `services/notification-service`
- Type: event consumer
- Owns: alerts and delivery state
- Consumes: processing failures, compliance gaps, report completion

## AI Orchestrator Service

- Folder: `services/ai-orchestrator-service`
- Type: internal HTTP service
- Owns: LLM prompts, model provider selection, safety wrappers
- Used by: rag, rca, compliance, ingestion

