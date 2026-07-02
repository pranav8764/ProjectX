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
    api-gateway/
    identity-service/
    document-service/
    ingestion-worker/
    ai-orchestrator-service/
    rag-service/
    asset-service/
    graph-service/
    rca-service/
    compliance-service/
    report-service/
    audit-service/
    notification-service/
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

If a feature changes business behavior, it belongs in a service. If it changes a request, response, or event shape, it belongs in `contracts`. If it changes deployment/runtime behavior, it belongs in `infra`.
