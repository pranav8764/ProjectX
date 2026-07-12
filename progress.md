# PlantBrainAI — Progress & Required Changes

> Generated 2026-07-06 from a multi-agent audit of the codebase against `instructions.md`
> (6 subsystem auditors + 1 completeness critic; every claim below is grounded in a `file:line` citation).

---

## 1. Executive Summary

**The system does not currently work end-to-end, but it is much closer than the finding count suggests.**

The codebase is a well-architected shell whose two ends are far more real than its middle:

- **The Go API gateway is genuinely solid** — real multipart upload with sha256 dedup and versioning, MinIO storage with presigned URLs, plant/org-scoped RBAC with document-level access policies, audit logging, rate limiting, and real proxying to the AI service with graceful fallbacks. Almost nothing in it is mocked.
- **The Postgres schema is the strongest layer** — all 12 tables from spec §1.7 exist (some sensibly normalized further), with pgvector 1536-dim + ivfflat index, retry bookkeeping, page-level text storage, and asset-alias support.
- **The AI core is broken at one load-bearing joint**: the asyncpg pool never registers a pgvector codec, so **every embedding INSERT and every vector query raises at runtime**. No uploaded document has ever reached `COMPLETED`; every real RAG query silently degrades to the "not enough evidence" fallback.
- **The frontend actively conceals this**: a client-side `setInterval` fakes the ingestion pipeline (a document that FAILED in the backend displays as COMPLETED), Copilot/RCA silently substitute canned answers with fabricated citations on any API error, the knowledge-graph page and dashboard never call their real backend endpoints, and a hardcoded "CRAG Faithfulness Verified" badge claims an evaluation that was never built.
- **Auth is theater**: login fabricates sessions client-side with hardcoded bearer tokens (`dev-token`, `meera-token`, …) shipped in the client bundle; better-auth is installed but is dead code; role/permissions live in user-editable localStorage.
- **Queue/retry/observability tiers are dead infrastructure**: Redis runs in compose but zero code connects to it; `next_retry_at` is written but nothing ever polls it; the OTel collector config has no producer and is never deployed.
- **CI is red**: `make compose-config` fails (required env vars never provided) and `tests/integration_test.py` mock mode fails at import (the harness predates the LangGraph rewrite).

**Distance to a truthful demo:** roughly one one-line-class fix (pgvector codec) plus about a week of de-mocking and wiring makes upload → ingest → cited-answer real at demo scale. Distance to the full MVP bar of `instructions.md` (real compliance agent, RCA depth, real auth, retries, evaluation metrics): several additional weeks.

---

## 2. FR / Requirement Scorecard

| Requirement | Status | One-line reality |
|---|---|---|
| FR-1 Authentication | 🟡 Partial / 🔴 Mocked | Go verifies sessions/JWTs correctly, but no signup/login endpoints exist anywhere; the web login is entirely simulated with hardcoded tokens |
| FR-2 RBAC | 🟢 Implemented | Real middleware + document-level access policies + unit tests in Go; frontend matrix matches spec — but its inputs come from fake auth |
| FR-3 Document Upload | 🟢 Implemented (with leaks) | Real multipart, dedup, versioning, MinIO; but user-supplied title/department/version/assetTag form fields are silently dropped, and AI dispatch is synchronous (violates <2s ack) |
| FR-4 OCR / Text Extraction | 🟡 Partial | Native PDF/DOCX/XLSX/CSV/image extraction + per-page Tesseract OCR fallback are real; all confidence scores are hardcoded constants (0.9/0.95) |
| FR-5 Document Classification | 🔴 Mocked | `CLASSIFYING` status is emitted with no classifier behind it; document_type is whatever the uploader typed |
| FR-6 Entity Extraction | 🟡 Partial | spaCy + regex extraction exists; the DATE regex is confirmed broken (unclosed character class matches giant text spans), EQUIPMENT_TYPE/PARAMETER never extracted, confidences hardcoded |
| FR-7 Knowledge Graph | 🟡 Partial | Typed edges (HAS_MANUAL, HAS_FAILURE, …) are built with evidence links and idempotent rebuild; polluted by the DATE-regex noise and O(n²) unbatched CO_OCCURS_WITH edges; never persists in practice due to the pgvector break upstream |
| FR-8 RAG Copilot | 🔴 Broken | Real LangGraph pipeline (guardrail → intent → hybrid retrieval → RBAC → generation → citation validation) with the correct §1.8 response shape — but vector retrieval throws at runtime; no reranking, no graph expansion, no query rewrite, no CRAG grading, no conflict detection |
| FR-9 Asset Profile | 🟡 Partial | DB-backed asset API + polished profile page; missing maintenance/inspection history, timeline data, asset-scoped graph; frontend hardcodes severity/status on real failures |
| FR-10 RCA Assistant | 🟡 Partial / 🔴 insecure | Single-prompt RCA over 10 entity-matched chunks; no timeline/patterns/missing-data output; **cross-tenant data leak** (plantId/orgId optional, no RBAC) |
| FR-11 Compliance Gaps | 🔴 Mocked | Heuristic string matching (`title ILIKE '%expired%'`), hardcoded requirements seeded inside a GET handler, no org/plant scoping, no auth |
| FR-12 Lessons Learned | ⚫ Missing | No route, node, or function exists at all |
| FR-13 Dashboard | 🟡 Partial | Real metrics SQL in Go (`/api/dashboard/metrics`) — but the dashboard page never calls it; renders mock/localStorage state instead |
| FR-14 Report Generation | 🟡 Partial | Real server-side CSV + hand-rolled PDF + minimal DOCX for 5 report types; missing audit-evidence package, citations absent from query-answers report, no restricted-doc filtering |
| Graph Visualization | 🔴 Mocked | Static hand-positioned SVG from title-substring matching; never calls `GET /api/graph` |
| Data model (§1.7) | 🟢 Implemented | All spec tables + richer (versions, pages, aliases, jobs, audit, model_calls) |
| Queue / async workers | 🔴 Mocked | Redis provisioned, zero consumers; FastAPI BackgroundTasks lost on restart; retry columns write-only |
| Security NFRs | 🟡 Partial | Signed URLs/audit/rate-limit real; dev-token auto-seeded whenever `APP_ENV != production`; no TLS; no encryption at rest; JWT trusts claims without DB check |
| Observability | 🔴 Mocked | `ai.model_calls` + `audit.events` + request logs are real; OTel config is orphaned; no metrics endpoint; correlation IDs never propagate |
| Testing (§1.11) | 🔴 Broken | Only Go RBAC pure-function tests pass; AI integration test fails at import; zero AI-evaluation, security, or performance tests |
| CI | 🔴 Broken | `docker compose config` fails on unset `${VAR:?}` interpolations; Go 1.26 in go.mod vs 1.22 in CI |
| Risk score (should-have) | ⚫ Missing | No computation anywhere; values exist only in `demo_seed.sql` |

---

## 3. Critical Blockers (fix these first, in order)

### P0.1 — pgvector codec never registered (breaks the entire product)
`services/ai/app/database.py:14` creates the asyncpg pool bare, while `services/ai/app/routes/ingestion.py:532-535` and `services/ai/app/graph.py:112-127` bind raw Python lists to `vector(1536)` params. asyncpg falls back to the text codec for unknown extension types and raises `DataError` on list input. Confirmed by grep: no `register_vector`, no `::vector` cast anywhere; `pgvector==0.4.2` is pinned but unused.

**Consequences:** every real ingestion fails at `STORING_RESULTS` → document `FAILED`; every RAG query throws in `retrieval_node` → both vector AND keyword paths die (keyword runs in the same failing block) → graceful "not enough evidence" fallback for everything.

**Fix (small):** `asyncpg.create_pool(..., init=pgvector.asyncpg.register_vector)` — or serialize embeddings as `'[x,y,…]'` strings with `::vector` casts in both ingestion and retrieval SQL. While in there:
- Fix seeded all-zero embeddings (`infra/db/seeds/demo_seed.sql:336-360`) — cosine distance to a zero vector is NaN, so demo chunks are unreachable by vector search even after the codec fix.
- Bind `dateFrom`/`dateTo` as `datetime` objects (`graph.py:123-124,149-150`) — the frontend sends strings, asyncpg raises on any dated query.
- Wrap vector and keyword retrieval in independent try/except so one path failing doesn't kill both.

### P0.2 — De-mock the frontend so the demo shows reality
- Delete the fake ingestion simulator (`apps/web/src/context/DataContext.tsx:353-414`) that overwrites real backend status with `COMPLETED` + fabricated confidences; poll `GET /api/documents/{id}/status` instead.
- Stop silently substituting canned Copilot/RCA answers with fabricated citations on API errors (`copilot/page.tsx:179-196`, `rca/page.tsx:114-121` → `mockData.ts`); show an explicit error or clearly-labeled demo-mode banner.
- Remove fake trust signals: "CRAG Faithfulness Verified" (`copilot/page.tsx:507`), "100% linked" badge (`app/page.tsx:147-149`), static "Online" gateway status, hardcoded "Available" report-jobs status.
- Wire dashboard to `GET /api/dashboard/metrics` and graph page to `GET /api/graph` — both endpoints exist in Go and are fully orphaned (`main.go:2806`, `main.go:3336`).
- Fetch certificates from a backend source or remove the tab (`mockData.ts:338-367` is permanent mock); persist `resolveGap`/`addFailureEvent` to the backend instead of localStorage.

### P0.3 — Make auth real
- Wire the existing better-auth client (`apps/web/src/lib/auth-client.ts`, currently dead code) to a real auth backend, or add login/signup endpoints to the Go API (identity.* tables already model BetterAuth).
- Remove hardcoded bearer tokens from client source (`login/page.tsx:60-66,113-118`) and the `DEFAULT_DEV_TOKEN` fallback (`api.ts:84-95`).
- Gate dev seeding behind an explicit `SEED_DEV_DATA=true` instead of `APP_ENV != production` (`main.go:196-198, 4155-4159`) — currently any misconfigured deployment ships a known 30-day credential.
- Drop `OR s.id = $1` from session lookup (`main.go:507`) — seeded session ids are predictable strings.
- Verify JWT subjects still exist in `identity.users` instead of trusting claims (deleted users keep access until expiry).

### P0.4 — Close the cross-tenant holes in the AI service
- **RCA leak:** `POST /rca` with plantId/organizationId omitted matches documents across ALL organizations (`rca.py:40-41`, both Optional in `schemas.py:18-23`) with no role check. Make them mandatory and apply the same RBAC chunk filter the copilot graph uses.
- **Compliance leaks:** requirements fetched globally (`compliance.py:20`), inspection evidence counted across all orgs (`compliance.py:48-52`), and a random `uuid4()` used as org_id when plant lookup fails (`compliance.py:31`). Scope everything by org/plant; add authorization; stop seeding hardcoded requirements inside a GET handler.
- Add service-to-service auth (the AI service trusts `userRole` from the request body and is directly reachable).

### P0.5 — Fix the confirmed data-corruption bug in entity extraction
The DATE regex (`ingestion.py:397`) has an escaped `]` so the character class never closes — verified at runtime to match arbitrary giant spans as ONE entity, flooding `graph.entities` and generating spurious edges through the O(n²) CO_OCCURS_WITH builder. Fix the character class; also constrain the SEVERITY regex (bare `low|medium|high` anywhere in prose) and cap/batch CO_OCCURS_WITH inserts.

---

## 4. Subsystem Detail

### 4.1 Go API (`services/api/cmd/server/main.go`, 4,178 lines)

**Working and real:** multipart upload + validation + 50MB cap + PDF magic check + sha256 dedup (`:1878-2252`); versioning with per-doc dedup and derived-data cleanup (`:2255-2599`); MinIO with presigned 10-min GETs; RBAC middleware on every route with per-document access policies and passing unit tests (`rbac_test.go`); audit events on all sensitive actions; rate limiting; all four §1.8 endpoints with matching shapes; graceful AI fallbacks; DB-backed dashboard/assets/graph/reports handlers.

**Required changes (beyond P0):**
| Priority | Change |
|---|---|
| High | Move AI dispatch out of the upload request path (currently synchronous with 12s timeout inside the request, `:133, :2171`) so upload acks in <2s |
| High | Add a background dispatcher polling `ingestion.processing_jobs.next_retry_at` — the retry machinery is currently write-only dead code (`:1186-1197`) |
| High | Persist the title/department/version/assetTag form fields the frontend already sends — currently dropped, title = filename (`:1887-1905, :2064`) |
| High | Raise `aiServiceTimeout` for /query and /rca — the LangGraph flow (intent LLM + generation + up to 3 validation retries + per-request checkpointer setup) routinely exceeds 12s, so real answers get dropped to fallback while still being logged, skewing metrics |
| High | Presign MinIO URLs with a browser-reachable endpoint — `S3_ENDPOINT=minio:9000` produces download URLs that only resolve inside the Docker network (broken for every real user) |
| High | Include citations in the query_answers report (FR-14 requires them); filter restricted docs from reports per caller's access policy |
| Medium | Add asset maintenance/inspection history + timeline to the asset profile response (FR-9); populate LinkedDocument access fields (contract drift); re-validate citation permissions API-side; align admin `documentAccessContextForRole` with `documentSourcePolicyForRole`; add top-searched-assets metric; sync api.yaml drifts (DocumentSummary/DashboardMetrics fields, DELETE x-rbac); surface audit-insert failures; namespace local upload files per version (retry-file clobbering); SSE on MinIO puts |
| Low | Password-protected PDF detection; CSV formula-injection escaping; audit-evidence-package report type; trusted-proxy config; report-job reaper for stuck RUNNING |

### 4.2 AI Service — Ingestion (`services/ai/app/routes/ingestion.py`, `graph.py`, `utils/`)

**Working and real:** native extraction for PDF/DOCX/XLSX/CSV/images/TXT with per-page OCR fallback (Tesseract + poppler installed in the image); page-level raw+markdown storage; char-approximated chunking within spec range; spaCy+regex entity extraction; typed graph relationship building with evidence links and idempotent per-document rebuild; full status lifecycle persisted to `processing_jobs` with PARTIAL_SUCCESS reasons; batch Gemini embeddings with Groq LLM fallback and per-call logging to `ai.model_calls`.

**Broken / mocked:**
- Embedding storage (P0.1 above) — the pipeline has likely never completed for a real upload.
- FR-5 classification is pure status theater: `CLASSIFYING` emitted at `:107`, no classifier exists; `classification_confidence` hardcoded 0.9.
- All OCR confidences hardcoded (0.9 / 0.95 per page, `:506, :523`) — the low-confidence-review flow can never trigger. Use `pytesseract.image_to_data` for real confidence.
- Silent fake success: with no `GOOGLE_API_KEY`, SHA256-derived pseudo-embeddings are stored and the document is marked COMPLETED — random retrieval masquerading as cited evidence. Must at minimum flag PARTIAL_SUCCESS.
- Document summary generation (§1.9) entirely absent.
- Asset tag normalization is `upper().strip()` only → "P 101" and "P-101" create duplicate assets; the correct normalizer already exists in `utils/helpers.py:38-46` but is unused in ingestion.
- Tables stored only as entity blobs with `page_no=1` — never chunked, embedded, or entity-scanned (invisible to retrieval).
- Markdown conversion is a naive ALL-CAPS heuristic, not the layout-aware parsing the spec calls for (Marker/MinerU/LlamaParse).
- Reliability: BackgroundTasks die with the process (documents stuck in non-terminal statuses forever, no reaper, no startup recovery); CPU-bound extraction blocks the event loop, starving health checks and query routes; `update_status` swallows every exception.
- Model IDs are stale for 2026: `gemini-1.5-pro` retired, `llama3-70b-8192` deprecated on Groq — both providers can 404, leaving the literal string "AI generation unavailable…" flowing into parsers as if it were an answer.

### 4.3 AI Service — Query / RCA / Compliance

**Working and real:** LangGraph copilot pipeline with guardrail → intent router → vector+keyword+metadata retrieval → RBAC chunk filter → generation → citation-validation loop (max 3 attempts) → correct §1.8 response shape with citations/confidence/missingInfo; version-aware retrieval (`current_version_id`, non-ARCHIVED); rag.queries/citations persistence.

**Gaps vs spec §1.6 retrieval stack:** no reranking, no knowledge-graph expansion, no query rewrite, no CRAG retrieval grading, no conflicting-source detection, no faithfulness validation (only checks a `[n]` token exists). `relatedAssets` just regexes the user's question. Confidence is LLM self-reported. GENERAL-intent branch returns uncited answers with hardcoded confidence 1.0 — a misrouted technical question violates "no answer without citations". Query embeddings use `RETRIEVAL_DOCUMENT` task type instead of `RETRIEVAL_QUERY`. Per-request checkpointer `setup()` + graph recompilation add latency against the 3–8s target; thread_id handling either disables memory or cross-contaminates all of a user's conversations.

**RCA (FR-10):** single prompt over ≤10 chunks; prompt asks for TIMELINE but the output format omits it so it's never parsed; no failure-pattern analysis, fishbone, similar incidents, or missingData in success responses; citations lack page/snippet; comma-split parsing is fragile; bypasses the guardrail/validation graph entirely. Plus the P0.4 tenancy hole.

**Compliance (FR-11):** two heuristics only — inspection-evidence title matching and `title ILIKE '%expired%'` for certificates (false positives at CRITICAL severity; real expired certs never flagged). No date extraction, no missing-SOP/audit-evidence/maintenance-proof/conflict checks, no severity model. Needs a real agent: extract expiry/inspection dates during ingestion, compare against requirement frequencies.

**FR-12 Lessons Learned: does not exist.** Prompt-injection protection: none (chunk text interpolated raw into prompts; keyword blocklist trivially bypassed) — the spec's own test case ("Ignore previous instructions" inside a PDF) would fail.

### 4.4 Frontend (`apps/web/`)

**Working and real:** complete §1.4 IA (all 9 nav sections), thorough client-side RBAC matrix matching FR-2, real API calls (via Next rewrites) for upload/versions/retry/status/archive/download, copilot/RCA POSTs with spec-shaped payloads, compliance gaps fetch + scan trigger, reports POST + server-artifact download, genuinely mobile-friendly layouts with technician-specific copilot UX.

**Mocked (see P0.2):** auth, ingestion status simulation, copilot/RCA fallback answers, graph page, dashboard metrics, certificates, gap resolution, failure logging, fake trust badges. Additional fixes: map real severity/status/workOrder from the asset API instead of hardcoding High/Open/RCA (`assets/[tag]/page.tsx:50-58`); the frontend status normalizer doesn't know the AI's intermediate statuses (`CONVERTING_TO_MARKDOWN`, `DETECTING_TABLES`, `STORING_RESULTS`) and maps them back to UPLOADED, making processing look stalled (`normalizers.ts:38-54`); stale localStorage state merges with and can mask fresh API data; label client-side export fallbacks instead of pretending PDF/DOCX.

### 4.5 Infra / Data / CI

**Working and real:** complete §1.7 schema with pgvector + ivfflat + GIN metadata index; coherent compose topology (ports/env/URLs line up, `${VAR:?}` guards); rich demo seed matching the §3 demo story; accurate Makefile + smoke script; OpenAPI contracts covering every endpoint.

**Required changes:**
| Priority | Change |
|---|---|
| Critical | Make CI green: provide `POSTGRES_*`/`MINIO_*`/`STORAGE_BUCKET` env in the workflow (compose config step currently errors), align setup-go with `go.mod` (1.26 vs 1.22), repair `tests/integration_test.py` mock harness (fails at import — predates the LangGraph rewrite; CI never pip-installs deps so the broken path always runs) |
| High | Implement a real queue consumer or remove Redis/`EVENT_BUS`/asyncapi.yaml theater; add startup recovery for jobs stuck in non-terminal statuses |
| High | Fix seed data: non-zero embeddings, real sha256 (currently md5 in `file_sha256`), file_urls/bucket that actually exist (seeded docs are undownloadable); seed all four demo sessions (3 of 4 hardcoded login tokens currently 401 out of the box) |
| High | Add compose healthchecks + `depends_on: service_healthy` + restart policies (cold initdb can outlast the API's 10s retry budget → crash loop) |
| Medium | Adopt a migration runner (schema changes currently only apply on fresh volumes); align `.env.example` with reality (missing `GOOGLE_API_KEY`/`GROQ_API_KEY`, declares unused `LLM_PROVIDER`/`EMBEDDING_*`/`JWT_SECRET`/`EVENT_BUS`; `STORAGE_PROVIDER=local` contradicts compose); either deploy the OTel collector + instrument services or delete the config; forward `X-Request-ID`/user id from Go to AI (correlation ids are always NULL; AI per-user rate limiting never engages) |
| Low | FK constraints for cross-schema refs; fix `services/catalog.yaml` double schema ownership; parameterize `run_seeds.sh`; add e2e compose test (upload → poll to COMPLETED → query) |

---

## 5. Design Decision: Incremental Re-embedding on Version Change (Notion-style)

**Requirement (owner decision, 2026-07-06):** when a new version of a document is uploaded, do **not** re-embed the whole document. Only changed content should be re-embedded, to save compute.

**Design** (to be implemented as part of the chunking/versioning work — decide this *before* building out the embedding pipeline further):

1. **Structure-first chunking.** Chunk the markdown on structural boundaries (headings/sections/paragraphs/tables) as stable "blocks", sub-splitting only blocks that exceed the token limit. This replaces pure token-window splitting, which defeats diffing: one inserted paragraph shifts every downstream window boundary, changing all hashes and forcing full re-embedding anyway. (This also supersedes the naive `600 tokens × 4 chars` approximation in `ingestion.py:360-391`.)
2. **Content hashing.** Add `chunk_hash` and a stable `block_id` to `ingestion.document_chunks`; chunks reference a `document_version` rather than being hard-deleted on re-upload.
3. **On version upload:** parse → split into blocks → hash → set-diff against the previous version's hashes (no LLM/embedding calls).
   - **Unchanged** → carry existing chunk rows + embeddings forward (repoint to the new version).
   - **New/modified** → embed only these.
   - **Deleted** → remove rows and deactivate their knowledge-graph edges — which FR-7's "new version should update old graph links" edge case already requires, so this design satisfies both at once.
4. **Payoff:** a revision touching 5% of a manual costs ~5% of the embedding spend; also gets FR-8's "prefer latest version / old-vs-new conflict" edge cases nearly for free.

Note: the current version-upload flow calls `cleanupDocumentDerivedData` (`main.go:1836-1866`) which wipes derived data — that becomes the diff-and-carry-forward step in this design.

---

## 6. What the Critic Flagged as Uncovered by Any Component

- **Risk score** (should-have, FR-9, §1.11 unit test): no computation exists anywhere; values come only from `demo_seed.sql`.
- **TLS / encryption in transit:** nothing anywhere; all services speak plain HTTP. Fine for local compose, unmet NFR otherwise.
- **AI-safety NFRs** "separate facts from recommendations", "no legal certification claims", "human approval for critical actions": only decorative frontend copy; no prompts or workflow implement them.
- **AI evaluation / security / performance testing (§1.11):** zero harness for faithfulness, top-k accuracy, citation correctness, hallucination rate, prompt injection, role bypass, or load — none of the §2.4 metric targets is measurable.
- **Demo dataset:** `data/sample-documents/` holds ten ~300-byte markdown stubs — no PDFs, no scanned docs, insufficient to demo OCR/classification/the P-101 story by real upload. The demo currently depends entirely on hand-inserted seed rows (whose embeddings are broken zero-vectors).
- **Hindi/vernacular query edge case (FR-8):** unhandled; `packages/shared` is empty scaffolding; numeric performance targets (<2s dashboard, <1s vector search @50k chunks) are unverified and unverifiable without metrics.

---

## 7. Recommended Execution Order

1. **Unbreak the core (days):** pgvector codec + seed embeddings + date-filter binding + independent retrieval try/except; fix DATE regex; verify upload → COMPLETED → cited answer end-to-end with a real PDF.
2. **De-mock the demo surface (days):** kill the ingestion simulator, mock answer fallbacks, and fake badges; wire dashboard/graph/certificates to real endpoints; fix presigned-URL host; persist upload metadata; seed all demo sessions.
3. **Security & tenancy (days):** RCA/compliance org scoping + auth; dev-token gating; JWT-vs-DB check; real login flow.
4. **Reliability & CI (days):** retry poller + startup recovery + healthchecks; green CI (env vars, Go version, fixed integration harness); raise AI timeouts / move dispatch off the request path.
5. **Intelligence depth (weeks):** real classification + OCR confidence + summaries; reranking + graph expansion + CRAG grading + conflict detection; RCA timeline/patterns; compliance date-extraction agent; lessons-learned engine; incremental re-embedding design (§5) alongside the chunking rework.
6. **Evaluation & polish (week):** faithfulness/citation-correctness harness against §2.4 targets; prompt-injection tests; asset timeline + asset-scoped graph; report citations + audit-evidence package.

---

*Full per-finding evidence (7 agents, 302k tokens of audit, ~70 tool calls) is preserved in the workflow journal; every `file:line` above was cited by an auditor and spot-checked by the completeness critic, which also corrected three auditor claims (noted inline).*
