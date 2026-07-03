# Project Structure

```text
ProjectX/
  apps/
    web/
  contracts/
    events/
    openapi/
    proto/
    schemas/
  services/
    api/
    ai/
  packages/
    shared/
  infra/
    db/
    docker/
    helm/
    k8s/
    observability/
    terraform/
  docs/
  data/
  scripts/
  tests/
```

## Rule of Thumb

If a feature changes business behavior, it belongs in `services/api` or `services/ai` depending on whether it is request/domain orchestration or AI/document processing. If it changes a request, response, or event shape, it belongs in `contracts`. If it changes deployment/runtime behavior, it belongs in `infra`.
