# OpenAPI Contracts

## Runtime Specs

- `api.yaml`: public Go API/BFF implemented by `services/api`
- `ai.yaml`: internal Python AI service implemented by `services/ai`

These are the only OpenAPI specs for the current MVP runtime. CI should treat
new `*-service.yaml` files as stale unless they arrive with a real service
folder, Dockerfile, health endpoint, owner, and deployment path.

For normal endpoint changes, update `api.yaml` or `ai.yaml` in the same pull
request as the implementation.
