# Contributing

## Branch Naming

Use short, scoped branch names:

```text
feature/document-upload
feature/rag-copilot
fix/upload-validation
docs/api-contracts
```

## Commit Style

Use clear commit messages:

```text
feat: add document upload endpoint
fix: handle empty PDF upload
docs: add MVP architecture notes
```

## Pull Request Checklist

- The change belongs to the correct folder.
- New environment variables are added to `.env.example`.
- API changes are reflected in `docs/API_CONTRACTS.md`.
- Machine-readable API changes are reflected in `contracts/openapi`.
- Event changes are reflected in `contracts/events/asyncapi.yaml`.
- Database changes include a migration in `infra/db/migrations`.
- Service-owned data stays inside that service's schema.
- User-facing behavior is covered by a short test or manual test note.
