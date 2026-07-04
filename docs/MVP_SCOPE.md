# MVP Scope

## Must Have

- Document upload
- Text extraction and OCR fallback
- Chunking and embeddings
- Hybrid retrieval using vector search plus exact asset-tag search
- RAG copilot with citations
- Asset/entity extraction
- Asset profile page
- Basic RCA assistant
- Basic compliance gap checker
- Dashboard with core metrics

## Current MVP Status

- Implemented runtime services: `services/api` and `services/ai`.
- Implemented public API routes: documents, signed document downloads, Copilot, dashboard metrics, assets, RCA, compliance gaps/scans, graph, and report generation.
- Implemented report formats: `csv`, `pdf`, and `docx` for asset summary, compliance gap, document inventory, RCA, and query answer reports.
- Implemented RBAC: coarse route-level role gates plus plant membership checks. Local development uses the seeded `dev-token` session.
- Implemented document-source access control across API, AI retrieval, web UX, and contract fields for `accessLevel`, `sensitivity`, `allowedRoles`, `sourceRestricted`, and `sourceDownloadAllowed`, separating metadata visibility from extracted/original evidence access.
- Implemented ingestion path: upload to MinIO and shared volume, OCR/text extraction, markdown conversion, table extraction, chunking, 1536-dimensional embeddings, entity extraction, asset upsert, and status updates.

## Remaining PRD Gaps

- Production authentication is not complete: current API accepts session tokens/cookies from `identity.sessions`, but the PRD's JWT/provider-backed sign-up and login flow still needs hardening.
- Source-level authorization is document-level, not paragraph-level: restricted source files and RAG chunks are filtered by document policy, but per-page/per-section ACLs are not implemented.
- Document versioning is functional for source replacement: users can upload a newer version, the API makes it current, clears stale derived evidence, and requeues AI processing. Version comparison/diff views are not implemented.
- Query filters are partial: `assetTag` affects retrieval, while document type/date filters are not fully applied in the AI service.
- Knowledge graph depth is partial: entities and asset upserts exist, but relationship extraction, deduplication, alias handling, and graph expansion are still limited.
- RCA output is basic: it returns summary, probable causes, recommendations, confidence, and document-title citations, but not full timeline, missing-data, page-level evidence, or conflict handling.
- Compliance detection is heuristic: default requirements, missing inspection evidence, and expired-title checks exist, but regulatory rule ingestion and strong expiry/evidence reasoning are not production-grade.
- Reports cover the main MVP tabular exports, but audit evidence packages and cited query-answer bundles remain incomplete.
- Non-functional gaps remain around observability, retryable background queues, encryption hardening, and formal AI safety/citation validation workflows.

## Should Have

- Document versioning
- Confidence scores
- Query filters by asset, document type, and date
- Exportable reports
- Knowledge graph visualization

## Not In MVP

- Full ERP/SAP/Maximo integration
- Real-time IoT/SCADA integration
- Full CAD/P&ID symbol parsing
- Production-grade regulatory certification
- Offline mobile app
- Custom LLM fine-tuning
