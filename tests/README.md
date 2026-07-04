# Tests

End-to-end tests, fixtures, and evaluation checks live here.

## Current Checks

```bash
make test-api
make test-ai
make smoke
```

`make test-api` runs the Go API unit tests, including RBAC role normalization checks.

`make test-ai` runs `tests/integration_test.py` in offline mock mode by default. The mock mode imports `services/ai/app/main.py`, replaces external AI/database libraries with test doubles, and verifies ingestion, embeddings, RAG citations, RCA output, and compliance scan behavior.

`make smoke` checks a running stack by calling API and AI `/health` plus `/api/me` with `PLANTBRAIN_API_TOKEN` or the local `dev-token`.

To target a running AI service instead:

```bash
PLANTBRAIN_TEST_URL=http://localhost:8000 make test-ai
```

The live mode checks the AI service endpoints directly: `/health`, `/process-document`, `/query`, `/rca`, and `/compliance`.
