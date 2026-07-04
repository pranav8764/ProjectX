# ADR 0001: Domain Boundaries and Consolidated Runtime

## Status

Accepted, amended for the MVP runtime.

## Context

PlantBrainAI has multiple high-complexity domains: document management, OCR/ingestion, asset intelligence, retrieval, graph construction, RCA, compliance, reports, audit, and notifications. The original scaffold represented those domains as separate service folders.

That created empty service folders before the MVP had real deployable code. The current project has two runnable backend services:

- `services/api`: Go API/BFF
- `services/ai`: Python FastAPI AI and ingestion service

## Decision

Keep explicit domain boundaries through docs, contracts, PostgreSQL schemas, and ownership rules, but consolidate the MVP runtime into `services/api` and `services/ai`.

Do not recreate empty per-domain service folders. A future split must introduce a real implementation, Dockerfile, health endpoint, contract, ownership plan, and deployment path in the same change.

## Consequences

- The MVP stays easier to run and demo.
- Teams still have clear ownership through schemas and logical modules.
- Contract changes must be documented in the consolidated OpenAPI files first.
- Split-service OpenAPI files should not be reintroduced unless the split is implemented as a real service in the same change.
- A future service split is possible without pretending the split exists today.
