# OpenAPI Contracts

## Canonical Runtime Specs

- `api.yaml`: public Go API/BFF implemented by `services/api`
- `ai.yaml`: internal Python AI service implemented by `services/ai`

These are the specs CI should treat as the current MVP API surface.

## Legacy Planning Sketches

The `*-service.yaml` files describe the original domain-service split. They are useful for future planning, but they do not correspond to folders under `services/`, Compose services, or deployable containers in the MVP.

Update a legacy sketch only when planning a real service split. For normal endpoint changes, update `api.yaml` or `ai.yaml`.
