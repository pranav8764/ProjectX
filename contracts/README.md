# Contracts

Shared contracts live here so teams can work independently.

## Folders

- `openapi`: REST API contracts
- `events`: AsyncAPI and event contracts
- `schemas`: Shared JSON schemas
- `proto`: Future protobuf/gRPC contracts

## Current Runtime Contracts

- `openapi/api.yaml`: public Go API/BFF exposed by `services/api`
- `openapi/ai.yaml`: internal Python AI service exposed by `services/ai`
- `events/asyncapi.yaml`: planned async event vocabulary for queue-backed processing

The older `openapi/*-service.yaml` files are legacy planning sketches for possible future domain service splits. They are intentionally not referenced by local Compose or CI as runnable services.

## Rule

If a request, response, or event changes, update the relevant consolidated contract in the same pull request. Update legacy sketches only when the future split plan itself changes.
