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

## Rule

If a request, response, or event changes, update the relevant consolidated contract in the same pull request. Do not add a split-service contract unless that pull request also adds a real deployable service and updates Compose/CI/deployment ownership.
