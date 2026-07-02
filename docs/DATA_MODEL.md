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
- `identity.users` (Mapped to Better Auth user entity)
- `identity.sessions` (Better Auth sessions)
- `identity.accounts` (Better Auth linked accounts)
- `identity.verifications` (Better Auth verification tokens)
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

### Authentication Schema (Better Auth)

Better Auth handles credentials and session storage using the following tables in the `identity` schema:
- **`identity.users`**: Extends the default Better Auth `user` schema with tenant fields (e.g. `organization_id`) and profile info.
- **`identity.sessions`**: Stores active login sessions, tokens, expirations, IP addresses, and user-agents.
- **`identity.accounts`**: Stores link info for authentication providers (e.g., passwords or OAuth credentials like GitHub or Google).
- **`identity.verifications`**: Holds temporary tokens and codes for email verification and password resets.
