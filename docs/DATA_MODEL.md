# Data Model

The MVP uses one PostgreSQL instance with service-owned schemas. This gives us real microservice boundaries while staying easy to run locally.

## Schemas

| Schema | Owner |
| --- | --- |
| `identity` | `identity-service` |
| `document` | `document-service` |
| `ingestion` | `ingestion-worker` |
| `asset` | `asset-service` |
| `graph` | `graph-service` |
| `rag` | `rag-service` |
| `rca` | `rca-service` |
| `compliance` | `compliance-service` |
| `report` | `report-service` |
| `audit` | `audit-service` |
| `notification` | `notification-service` |
| `ai` | `ai-orchestrator-service` |

## Important Tables

- `identity.organizations`
- `identity.plants`
- `identity.users`
- `identity.memberships`
- `document.documents`
- `document.document_versions`
- `document.upload_sessions`
- `ingestion.processing_jobs`
- `ingestion.document_pages`
- `ingestion.document_chunks`
- `asset.assets`
- `asset.asset_aliases`
- `graph.entities`
- `graph.relationships`
- `rag.queries`
- `rag.citations`
- `rca.reports`
- `compliance.requirements`
- `compliance.gaps`
- `report.jobs`
- `audit.events`
- `notification.notifications`
- `ai.model_calls`

Database migrations live in `infra/db/migrations`.
