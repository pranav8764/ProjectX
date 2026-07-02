# API Gateway

Public entry point for PlantBrainAI APIs.

## Responsibilities

- Validate external requests
- Enforce authentication and tenant context
- Route to internal services
- Aggregate frontend-friendly responses
- Apply rate limits and request logging
- Hide internal service topology from clients

## Should Not Own

- Business data tables
- OCR/RAG implementation
- Long-running jobs

## Contracts

- Public API: `contracts/openapi/api-gateway.yaml`
- Internal service calls: service-specific OpenAPI files

