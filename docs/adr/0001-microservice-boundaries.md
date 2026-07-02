# ADR 0001: Microservice Boundaries

## Status

Accepted for MVP scaffold.

## Context

PlantBrainAI has multiple high-complexity domains: document management, OCR/ingestion, asset intelligence, retrieval, graph construction, RCA, compliance, reports, audit, and notifications. A single backend would be quicker for a tiny prototype but would make team ownership unclear.

## Decision

Use a monorepo with explicit microservice boundaries. Each service owns a domain, folder, data schema, README, API contract, and events where relevant.

For local development, services share one PostgreSQL instance with separate schemas. For production, services can move to separate databases as needed.

## Consequences

- Teams can work independently with clearer boundaries.
- Contract changes must be documented.
- More boilerplate exists than in a monolith.
- Async document processing is easier to reason about from the beginning.
