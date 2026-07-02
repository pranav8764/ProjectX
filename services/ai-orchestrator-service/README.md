# AI Orchestrator Service

Internal service for shared AI provider access and model safety behavior.

## Responsibilities

- LLM provider routing
- Embedding provider routing
- Prompt templates
- Safety and refusal policies
- Citation validation helpers
- Common model-call telemetry

## Owns Schema

`ai`

## Used By

- `rag-service`
- `rca-service`
- `compliance-service`
- `ingestion-worker`

