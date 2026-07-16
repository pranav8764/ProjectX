<p align="center">
  <h1 align="center">🧠 PlantBrainAI</h1>
  <p align="center">
    <strong>AI-Powered Industrial Knowledge Platform</strong>
  </p>
  <p align="center">
    Turn scattered plant documents into searchable, cited asset intelligence.
  </p>
  <p align="center">
    <a href="#-quick-start">Quick Start</a> •
    <a href="#-architecture-overview">Architecture</a> •
    <a href="#-high-level-design">HLD</a> •
    <a href="#-low-level-design">LLD</a> •
    <a href="#-api-reference">API</a> •
    <a href="#-deployment">Deployment</a> •
    <a href="#-contributing">Contributing</a>
  </p>
</p>

---

## 📋 Table of Contents

- [Overview](#-overview)
- [Key Features](#-key-features)
- [Architecture Overview](#-architecture-overview)
- [High-Level Design (HLD)](#-high-level-design)
- [Low-Level Design (LLD)](#-low-level-design)
- [Technology Stack](#-technology-stack)
- [Repository Layout](#-repository-layout)
- [Quick Start](#-quick-start)
- [Environment Variables](#-environment-variables)
- [API Reference](#-api-reference)
- [Data Model](#-data-model)
- [Security & Access Control](#-security--access-control)
- [Testing](#-testing)
- [Deployment](#-deployment)
- [Roadmap](#-roadmap)
- [Architecture Decision Records](#-architecture-decision-records)
- [Contributing](#-contributing)
- [License](#-license)

---

## 🔍 Overview

**PlantBrainAI** is an AI-powered platform that transforms unstructured industrial plant documents — maintenance logs, work orders, inspection reports, OEM manuals, compliance checklists — into a connected knowledge layer. Engineers can upload documents, ask natural-language questions with cited answers, inspect asset profiles, generate root-cause analyses, and detect compliance gaps — all backed by evidence from their own plant data.

### The Problem

Industrial plants generate thousands of documents across maintenance, safety, compliance, and operations. This critical knowledge is trapped in PDFs, scans, and spreadsheets — making it nearly impossible for engineers to find answers quickly, trace failure patterns, or verify compliance.

### The Solution

PlantBrainAI ingests plant documents through OCR and text extraction, chunks and embeds them for semantic search, extracts entities and relationships into a knowledge graph, and exposes the intelligence through a RAG-powered copilot, asset profiles, RCA generator, and compliance gap detector.

---

## ✨ Key Features

| Feature | Description |
| --- | --- |
| **Document Ingestion** | Upload PDFs, DOCX, Excel, CSVs, images, and text files. Automatic OCR fallback for scanned documents. |
| **RAG Copilot** | Ask natural-language questions and receive cited answers with confidence scores, powered by a LangGraph state machine. |
| **Asset Intelligence** | Auto-extracted asset profiles with tags, types, risk scores, failure timelines, and linked documents. |
| **Root Cause Analysis** | AI-generated RCA reports with probable causes, recommendations, and document-backed citations. |
| **Compliance Gap Detection** | Heuristic compliance scanning that identifies missing evidence, expired inspections, and uncovered requirements. |
| **Knowledge Graph** | Extracted entities and relationships between assets, failure modes, parts, and procedures. |
| **Report Generation** | Export asset summaries, compliance gaps, document inventories, RCA reports, and query answers as CSV, PDF, or DOCX. |
| **RBAC & Document Access Control** | Role-based access with document-level sensitivity policies separating metadata visibility from source file access. |
| **Stateful Conversations** | LangGraph checkpointer-backed memory with session persistence and conversation continuity. |

---

## 🏗 Architecture Overview

PlantBrainAI follows a **consolidated microservice** pattern — domain boundaries are explicit through PostgreSQL schemas and contracts, while the MVP runtime is streamlined into two backend services for simplicity and deployability.

```
┌─────────────────────────────────────────────────────────────────────┐
│                          Client Layer                               │
│                                                                     │
│                    Next.js Web App (:3000)                          │
│         Dashboard │ Copilot │ Assets │ RCA │ Compliance            │
└──────────────────────────┬──────────────────────────────────────────┘
                           │ HTTPS / REST
┌──────────────────────────▼──────────────────────────────────────────┐
│                       API / BFF Layer                               │
│                                                                     │
│                  Go API Service (:8080)                             │
│     Auth │ Upload │ Documents │ Assets │ Reports │ Dashboard       │
│               RBAC Enforcement │ Rate Limiting                     │
└──────┬────────────┬───────────────┬────────────┬───────────────────┘
       │            │               │            │
       ▼            ▼               ▼            ▼
┌─────────┐  ┌─────────────┐  ┌─────────┐  ┌─────────────────┐
│  MinIO  │  │ Python AI   │  │  Redis  │  │  PostgreSQL     │
│  Object │  │ Service     │  │  Cache  │  │  + pgvector     │
│  Store  │  │ (:8000)     │  │ & Queue │  │  (12 schemas)   │
│         │  │             │  │         │  │                 │
│ uploads │  │ OCR│RAG│RCA │  │ events  │  │ identity│asset  │
│ reports │  │ NLP│Graph   │  │ streams │  │ document│graph  │
└─────────┘  └─────────────┘  └─────────┘  │ ingestion│rag  │
                    │                       │ rca│compliance │
                    ▼                       │ report│audit   │
             ┌────────────┐                 │ notification│ai│
             │ LLM / Embed│                 └─────────────────┘
             │ Providers   │
             │ Gemini│Groq │
             └────────────┘
```

### Service Topology

| Service | Language | Port | Role |
| --- | --- | ---: | --- |
| `apps/web` | TypeScript / Next.js | 3000 | Frontend SPA with SSR |
| `services/api` | Go | 8080 | Public API / BFF — auth, uploads, assets, compliance, reports |
| `services/ai` | Python / FastAPI | 8000 | Internal AI service — OCR, embeddings, RAG, RCA, graph |
| PostgreSQL + pgvector | — | 5432 | Relational data + vector embeddings |
| Redis | — | 6379 | Cache, future event streaming (Redis Streams) |
| MinIO | — | 9000/9001 | S3-compatible object storage for uploads |

> **Design Principle:** External clients only call `services/api`. The AI service is internal to the Docker network and is never browser-facing.

---

## 📐 High-Level Design

### System Context

```
┌───────────┐        ┌──────────────┐        ┌────────────────┐
│ Engineers │───────▶│  PlantBrain  │───────▶│  LLM Providers │
│ Operators │◀───────│    AI        │◀───────│  (Gemini/Groq) │
│ Managers  │        └──────┬───────┘        └────────────────┘
└───────────┘               │
                   ┌────────▼────────┐
                   │ Plant Documents │
                   │ PDFs, Scans,    │
                   │ Spreadsheets    │
                   └─────────────────┘
```

### Core Workflows

#### 1. Document Processing Pipeline

```
Upload Request
  → API validates auth, file type, plant, document type
  → API stores metadata in document schema
  → API writes original file to MinIO + shared volume
  → API calls AI service /process-document
  → AI extracts text (or OCR fallback for scanned documents)
  → AI creates markdown, extracts tables, chunks content
  → AI generates 1536-dimensional vector embeddings
  → AI extracts entities (assets, parts, failure modes)
  → AI upserts detected assets into asset schema
  → AI writes graph entities and relationships
  → AI marks document status COMPLETED or FAILED
```

#### 2. RAG Copilot Pipeline (LangGraph State Machine)

```
User Question
  → [guardrail_node]     Input validation & PII check
  → [intent_router_node] LLM classifies intent (RAG_QUERY vs GENERAL)
  → [retrieval_node]     Hybrid search: pgvector + keyword fallback
  → [rbac_node]          Document-level role access filtering
  → [generation_node]    LLM synthesizes cited response
  → [validation_node]    Self-correcting citation verification (up to 3 retries)
  → Final cited answer with confidence score
```

#### 3. RCA Generation

```
RCA Request (asset tag + failure description)
  → Retrieve relevant document chunks for the asset
  → LLM analyzes failure patterns across maintenance history
  → Generate: summary, probable causes, recommendations
  → Return cited RCA report with confidence score
```

#### 4. Compliance Gap Detection

```
Compliance Scan Request
  → Load compliance requirements for the plant
  → Check each asset for: missing evidence, expired inspections, uncovered requirements
  → Generate gap records with severity and remediation hints
  → Persist gaps for dashboard and report export
```

### Logical Module Map

| Logical Module | Runtime Owner | Responsibilities |
| --- | --- | --- |
| Identity | `services/api` | Users, roles, memberships, auth, permissions |
| Documents | `services/api` | Uploads, versions, status, file pointers, duplicates |
| Ingestion | `services/ai` | Parsing, OCR, markdown, tables, chunks, embeddings |
| RAG | `api` + `ai` | Query routing, retrieval, cited answer generation |
| Assets | `api` + `ai` | Asset records, aliases, profiles, timelines, risk scoring |
| Graph | `services/ai` | Entity extraction, relationships, deduplication |
| RCA | `api` + `ai` | Failure context, probable causes, recommendations |
| Compliance | `api` + `ai` | Requirements, evidence matching, gap detection |
| Reports | `services/api` | Synchronous report jobs — CSV, PDF, DOCX exports |
| Audit | `services/api` | Audit events, document access logs |

---

## 🔬 Low-Level Design

### API Service (`services/api`) — Go

```
services/api/
├── cmd/server/
│   ├── main.go              # HTTP server, router, all public API routes
│   └── seed.go              # Development data seeder
├── internal/
│   ├── auth/                # Session/token validation, Better Auth integration
│   ├── handlers/            # Domain-specific HTTP handlers
│   │   ├── assets.go        # Asset CRUD, profile aggregation
│   │   ├── compliance.go    # Compliance gaps, scan triggers
│   │   ├── copilot.go       # RAG query proxy to AI service
│   │   ├── dashboard.go     # Metrics aggregation for dashboard
│   │   ├── documents.go     # Upload, versioning, download, archive
│   │   ├── graph.go         # Knowledge graph reads
│   │   ├── me.go            # Current user context
│   │   ├── rca.go           # RCA generation proxy
│   │   └── reports.go       # Report job creation, CSV/PDF/DOCX generation
│   ├── middleware/           # Auth middleware, rate limiting, CORS
│   └── models/              # Go struct definitions
├── go.mod / go.sum
└── Dockerfile
```

**Key Design Decisions:**
- Single `main.go` router with handler functions extracted into `internal/handlers/`
- Session-based auth via Better Auth with `dev-token` for local development
- Configurable sliding-window rate limiters (`API_RATE_LIMIT`, `API_AUTH_RATE_LIMIT`)
- Presigned MinIO URLs for secure document downloads (600s expiry)

### AI Service (`services/ai`) — Python FastAPI

```
services/ai/
├── app/
│   ├── main.py              # FastAPI app, lifespan, router registration
│   ├── config.py            # Environment config, provider settings
│   ├── database.py          # Async DB pool (asyncpg) lifecycle
│   ├── schemas.py           # Pydantic request/response models
│   ├── state.py             # LangGraph state definitions
│   ├── graph.py             # LangGraph RAG state machine (compiled graph)
│   ├── routes/
│   │   ├── ingestion.py     # /process-document — OCR, chunking, embeddings
│   │   ├── query.py         # /query — RAG copilot
│   │   ├── rca.py           # /rca — Root cause analysis
│   │   └── compliance.py    # /compliance-scan — Gap detection
│   └── utils/
│       ├── ai_clients.py    # Gemini/Groq provider wrappers (google-genai)
│       └── helpers.py       # Shared utility functions
├── requirements.txt
└── Dockerfile
```

**Key Design Decisions:**
- LangGraph compiled state machine for multi-step RAG with memory checkpointing
- `AsyncPostgresSaver` for production session persistence, `MemorySaver` for testing
- Token-bucket rate limiter middleware (`AI_RATE_LIMIT_RPS`, `AI_RATE_LIMIT_BURST`)
- Graceful degradation: ingestion marked `PARTIAL_SUCCESS` when no API key is configured

### Web Application (`apps/web`) — Next.js

```
apps/web/src/
├── app/                     # Next.js App Router pages
│   ├── page.tsx             # Dashboard (home)
│   ├── login/               # Authentication page
│   ├── copilot/             # RAG chat interface
│   ├── assets/              # Asset explorer & profiles
│   ├── documents/           # Document upload & management
│   ├── compliance/          # Compliance gap dashboard
│   ├── rca/                 # RCA generator
│   ├── graph/               # Knowledge graph visualization
│   ├── reports/             # Report generation & downloads
│   ├── admin/               # Admin panel
│   └── api/auth/[...all]/   # Better Auth route handler
├── components/              # Reusable UI components
│   └── NavigationShell.tsx   # App shell with sidebar navigation
├── context/
│   └── DataContext.tsx       # Global data provider
├── lib/
│   ├── api.ts               # API client utilities
│   ├── auth.ts              # Better Auth client setup
│   └── normalizers.ts       # Data normalization helpers
├── features/                # Feature-specific modules
└── styles/                  # Global CSS
```

### Communication Patterns

```
┌──────────────────────────────────────────────────────┐
│                  Synchronous (HTTP)                    │
│                                                        │
│  Browser ──REST──▶ API ──REST──▶ AI Service            │
│  Browser ──REST──▶ API ──SQL───▶ PostgreSQL            │
│  Browser ──REST──▶ API ──S3────▶ MinIO                 │
│  AI Service ─────────────SQL───▶ PostgreSQL + pgvector │
│  AI Service ─────────────File──▶ Shared uploads volume │
└──────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────┐
│               Asynchronous (Planned)                   │
│                                                        │
│  API ──publish──▶ Redis Streams ──consume──▶ AI        │
│  Events: document.uploaded, document.indexed,          │
│          asset.upserted, compliance.gap_detected,      │
│          rca.report_generated                          │
└──────────────────────────────────────────────────────┘
```

---

## 🛠 Technology Stack

| Layer | Technology |
| --- | --- |
| **Frontend** | Next.js 14+, TypeScript, React |
| **API / BFF** | Go, `net/http`, `pgx/v5`, `chi` router |
| **AI Service** | Python 3.11+, FastAPI, LangGraph, asyncpg |
| **Database** | PostgreSQL 16 + pgvector extension |
| **Vector Search** | pgvector (1536-dimensional embeddings) |
| **Object Storage** | MinIO (S3-compatible) / AWS S3 / Cloudflare R2 |
| **Cache / Queue** | Redis 7 (future: Redis Streams for event bus) |
| **LLM Providers** | Google Gemini, Groq (Llama 3.3) |
| **Embeddings** | Google `text-embedding-004` (1536-dim) |
| **Auth** | Better Auth (session-based, email/password + OAuth) |
| **Containerization** | Docker, Docker Compose |
| **CI/CD** | GitHub Actions |
| **Future Infra** | Kubernetes, Helm, Terraform |
| **Observability** | OpenTelemetry (planned) |

---

## 📁 Repository Layout

```
ProjectX/
├── apps/
│   └── web/                          # Next.js frontend (TypeScript)
├── contracts/
│   ├── openapi/                      # REST API contracts (api.yaml, ai.yaml)
│   ├── events/                       # AsyncAPI event contracts
│   ├── schemas/                      # Shared JSON schemas
│   └── proto/                        # Future gRPC/protobuf contracts
├── services/
│   ├── api/                          # Go API/BFF service
│   └── ai/                           # Python FastAPI AI service
├── packages/
│   └── shared/                       # Shared types, schemas, constants
├── infra/
│   ├── db/                           # Migrations and seed data
│   │   ├── migrations/               # PostgreSQL schema DDL
│   │   └── seeds/                    # Demo data inserts
│   ├── docker/                       # Docker helper files
│   ├── k8s/                          # Kubernetes manifests (future)
│   ├── helm/                         # Helm charts (future)
│   ├── observability/                # Logs, metrics, tracing config
│   └── terraform/                    # Cloud infrastructure (future)
├── docs/                             # Architecture, API, data model docs
│   ├── ARCHITECTURE.md
│   ├── API_CONTRACTS.md
│   ├── DATA_MODEL.md
│   ├── MICROSERVICES.md
│   ├── DEPLOYMENT.md
│   ├── MVP_SCOPE.md
│   └── adr/                          # Architecture Decision Records
├── data/
│   ├── sample-documents/             # Demo PDFs, scans, CSVs
│   └── processed/                    # Local processing outputs
├── scripts/                          # Developer and automation scripts
├── tests/                            # Integration tests and fixtures
├── docker-compose.yml                # Full local development stack
├── Makefile                          # Developer workflow commands
├── .env.example                      # Environment variable template
└── .github/workflows/ci.yml          # CI pipeline
```

---

## 🚀 Quick Start

### Prerequisites

- Docker & Docker Compose
- Node.js 20+ and npm (for local web dev)
- Git

### 1. Clone and Configure

```bash
git clone https://github.com/pranav8764/ProjectX.git
cd ProjectX
cp .env.example .env
```

Edit `.env` and fill in required values. For a fully local setup:

```bash
# Use the bundled local PostgreSQL
DATABASE_URL=postgresql://plantbrain:plantbrain@postgres:5432/plantbrain

# Generate auth secret
BETTER_AUTH_SECRET=$(openssl rand -base64 32)

# Optional: Add AI provider keys for real LLM responses
GOOGLE_API_KEY=your-gemini-api-key
GROQ_API_KEY=your-groq-api-key
```

> **Note:** Without `GOOGLE_API_KEY` or `GROQ_API_KEY`, the AI service operates in fallback mode — ingestion is marked `PARTIAL_SUCCESS` and the copilot returns a no-evidence response.

### 2. Start the Stack

**Option A: Full Docker Compose (Recommended)**

```bash
# Start everything including the web app
docker compose --profile local-db up --build

# In a separate terminal, apply migrations and seed data
make db-migrate
make db-seed
make seed-demo-auth
```

**Option B: Services + Local Web Dev**

```bash
# Start backend services
make services-up
make db-seed

# Start the web app locally
npm --prefix apps/web ci
npm --prefix apps/web run dev
```

### 3. Verify

```bash
# Run smoke tests
make smoke

# Check service health
curl http://localhost:8080/health   # API
curl http://localhost:8000/health   # AI
```

### 4. Access

| Service | URL |
| --- | --- |
| Web App | http://localhost:3000 |
| API / BFF | http://localhost:8080 |
| AI Service | http://localhost:8000 |
| MinIO Console | http://localhost:9001 |

> **Dev Token:** The API automatically seeds a `dev-token` session when the database is empty and `APP_ENV` is not `production`. Use `Authorization: Bearer dev-token` for API requests.

### Demo Personas

After running `make seed-demo-auth`, the following demo accounts are available (password: `PlantBrain#2026`):

Use these to test different RBAC roles through the login page at http://localhost:3000/login.

---

## 🔐 Environment Variables

### Required

| Variable | Description | Example |
| --- | --- | --- |
| `DATABASE_URL` | PostgreSQL connection string (use session pooler for Supabase) | `postgresql://plantbrain:plantbrain@postgres:5432/plantbrain` |
| `BETTER_AUTH_SECRET` | Cryptographic signing key for Better Auth | Generate with `openssl rand -base64 32` |
| `MINIO_ROOT_USER` | MinIO admin username | `plantbrain` |
| `MINIO_ROOT_PASSWORD` | MinIO admin password (≥ 8 chars) | `plantbrain-dev-secret` |

### Optional — AI Providers

| Variable | Description | How to Get |
| --- | --- | --- |
| `GOOGLE_API_KEY` | Gemini API key for LLM + embeddings | [Google AI Studio](https://aistudio.google.com/) |
| `GROQ_API_KEY` | Groq API key for fast inference | [Groq Console](https://console.groq.com/) |
| `GEMINI_MODEL` | Override Gemini model (default: `gemini-2.0-flash`) | — |
| `GEMINI_EMBED_MODEL` | Override embedding model (default: `text-embedding-004`) | — |
| `GROQ_MODEL` | Override Groq model (default: `llama-3.3-70b-versatile`) | — |

### Optional — Tuning

| Variable | Default | Description |
| --- | --- | --- |
| `APP_ENV` | `development` | Environment (`development`, `production`, `test`) |
| `CORS_ALLOWED_ORIGINS` | `http://localhost:3000` | Comma-separated allowed origins |
| `API_RATE_LIMIT` | `300` | API router rate limit (requests per window) |
| `API_RATE_LIMIT_WINDOW` | `1m` | API rate limit window |
| `AI_RATE_LIMIT_RPS` | `5.0` | AI service requests/second per client |
| `AI_RATE_LIMIT_BURST` | `20.0` | AI service token bucket burst capacity |
| `LOG_LEVEL` | `info` | Log verbosity (`debug`, `info`, `warn`, `error`) |

See [`.env.example`](.env.example) for the complete variable reference.

---

## 📡 API Reference

All authenticated endpoints use `Authorization: Bearer <token>`. Full contract details are in [`docs/API_CONTRACTS.md`](docs/API_CONTRACTS.md) and the machine-readable OpenAPI specs at [`contracts/openapi/`](contracts/openapi/).

### Public API (`services/api` — :8080)

| Method | Endpoint | Description |
| --- | --- | --- |
| `GET` | `/health` | Health check |
| `GET` | `/api/me` | Current user context |
| `POST` | `/api/documents/upload` | Upload a document (multipart) |
| `GET` | `/api/documents?plantId=` | List documents for a plant |
| `GET` | `/api/documents/:id` | Document detail with chunks & entities |
| `GET` | `/api/documents/:id/status` | Processing status |
| `GET` | `/api/documents/:id/download` | Presigned download URL |
| `POST` | `/api/documents/:id/versions` | Upload new document version |
| `POST` | `/api/documents/:id/retry-processing` | Retry failed processing |
| `DELETE` | `/api/documents/:id` | Archive document |
| `POST` | `/api/copilot/query` | Ask the RAG copilot |
| `GET` | `/api/assets?plantId=` | List assets |
| `GET` | `/api/assets/:id` | Asset profile |
| `POST` | `/api/rca/generate` | Generate root cause analysis |
| `GET` | `/api/compliance/gaps?plantId=` | List compliance gaps |
| `POST` | `/api/compliance/scan?plantId=` | Run compliance scan |
| `GET` | `/api/graph?plantId=` | Knowledge graph data |
| `GET` | `/api/dashboard/metrics?plantId=` | Dashboard metrics |
| `POST` | `/api/reports` | Create report job |
| `GET` | `/api/reports?plantId=` | List reports |
| `GET` | `/api/reports/:id/download` | Download generated report |

### Internal AI API (`services/ai` — :8000)

| Method | Endpoint | Description |
| --- | --- | --- |
| `GET` | `/health` | Health check |
| `POST` | `/process-document` | Document ingestion pipeline |
| `POST` | `/query` | RAG retrieval and answer generation |
| `POST` | `/rca` | Root cause analysis generation |
| `POST` | `/compliance-scan` | Compliance gap detection |

### Supported Upload Formats

`.pdf`, `.docx`, `.doc`, `.xlsx`, `.xls`, `.csv`, `.png`, `.jpg`, `.jpeg`, `.txt`, `.md`

### Report Types & Formats

| Report Type | Formats |
| --- | --- |
| `asset_summary` | CSV, PDF, DOCX |
| `compliance_gap` | CSV, PDF, DOCX |
| `document_inventory` | CSV, PDF, DOCX |
| `rca_report` | CSV, PDF, DOCX |
| `query_answers` | CSV, PDF, DOCX |

---

## 🗄 Data Model

PlantBrainAI uses a **single PostgreSQL instance with 12 domain schemas**. Schema boundaries are explicit to enable a future microservice split without data migration.

```
┌────────────────────────────────────────────────────────────┐
│                     PostgreSQL Instance                      │
│                                                              │
│  ┌─────────────┐  ┌──────────┐  ┌───────────┐              │
│  │  identity    │  │ document │  │ ingestion │              │
│  │ orgs,users, │  │ docs,    │  │ jobs,     │              │
│  │ sessions,   │  │ versions,│  │ pages,    │              │
│  │ roles,      │  │ uploads  │  │ chunks    │              │
│  │ memberships │  │          │  │           │              │
│  └─────────────┘  └──────────┘  └───────────┘              │
│                                                              │
│  ┌─────────┐  ┌─────────┐  ┌────────┐  ┌──────────┐       │
│  │  asset  │  │  graph  │  │  rag   │  │   rca    │       │
│  │ assets, │  │entities,│  │queries,│  │ reports  │       │
│  │ aliases │  │relations│  │citations│ │          │       │
│  └─────────┘  └─────────┘  └────────┘  └──────────┘       │
│                                                              │
│  ┌────────────┐  ┌────────┐  ┌──────────────┐  ┌────┐     │
│  │ compliance │  │ report │  │ notification │  │ ai │     │
│  │ requiremts,│  │ jobs   │  │ alerts       │  │model│    │
│  │ gaps       │  │        │  │              │  │calls│    │
│  └────────────┘  └────────┘  └──────────────┘  └────┘     │
│                                                              │
│  ┌────────┐                                                 │
│  │ audit  │  (future: event log)                            │
│  │ events │                                                 │
│  └────────┘                                                 │
└────────────────────────────────────────────────────────────┘
```

### Schema Ownership

| Schema | Owner | Key Tables |
| --- | --- | --- |
| `identity` | `services/api` | `organizations`, `plants`, `users`, `sessions`, `accounts`, `roles`, `memberships` |
| `document` | `services/api` | `documents`, `document_versions`, `upload_sessions` |
| `ingestion` | `services/ai` | `processing_jobs`, `document_pages`, `document_chunks` |
| `asset` | `services/api` | `assets`, `asset_aliases` |
| `graph` | `services/ai` | `entities`, `relationships` |
| `rag` | `api` + `ai` | `queries`, `citations` |
| `rca` | `api` + `ai` | `reports` |
| `compliance` | `api` + `ai` | `requirements`, `gaps` |
| `report` | `services/api` | `jobs` |
| `ai` | `services/ai` | `model_calls` (latency, token usage auditing) |
| `audit` | future | `events` |
| `notification` | future | `notifications` |

### Authentication Schema (Better Auth)

- **`identity.users`** — Extends Better Auth user with tenant fields (`organization_id`)
- **`identity.sessions`** — Active login sessions, tokens, expiry, IP/user-agent
- **`identity.accounts`** — Linked auth providers (credentials, GitHub, Google)
- **`identity.verifications`** — Temporary tokens for email verification and password resets

Migrations are in [`infra/db/migrations/`](infra/db/migrations/). Demo seed data is in [`infra/db/seeds/`](infra/db/seeds/).

---

## 🔒 Security & Access Control

### RBAC Roles

| Role | Document Upload | Source Download | RCA | Compliance Scan | Reports | Dashboard |
| --- | :---: | :---: | :---: | :---: | :---: | :---: |
| Admin | ✅ | ✅ All | ✅ | ✅ | ✅ | ✅ |
| Plant Manager | ✅ | ✅ Restricted | ✅ | ✅ | ✅ | ✅ |
| Engineer | ✅ | ✅ Restricted | ✅ | ❌ | ✅ | ✅ |
| Compliance Officer | ✅ | ✅ Confidential | ❌ | ✅ | ✅ | ✅ |
| Technician | ✅ | ✅ Public/Internal | ❌ | ❌ | ❌ | ❌ |
| Viewer | ❌ | ✅ Public only | ❌ | ❌ | ❌ | ❌ |

### Document Access Levels

| Level | Visibility |
| --- | --- |
| `public` | All authenticated users in the plant |
| `internal` | All authenticated users in the plant |
| `restricted` | Admin, Plant Manager, Engineer, Compliance Officer |
| `confidential` | Admin, Compliance Officer only |

### Security Features

- **Rate Limiting** — Configurable sliding-window (Go API) and token-bucket (Python AI) limiters
- **CORS** — Origin allowlisting via `CORS_ALLOWED_ORIGINS`
- **Presigned URLs** — Document downloads via 600-second expiring MinIO signed URLs
- **Plant Scoping** — All endpoints verify plant membership within the user's organization
- **Metadata vs. Source Separation** — Document metadata is visible separately from source file access

---

## 🧪 Testing

### Available Test Commands

```bash
# Run all tests (API + AI + Web build)
make test

# Go API tests
make test-api

# Python AI tests (mock mode by default)
make test-ai

# Next.js build validation
make test-web

# Live smoke test (requires running stack)
make smoke
```

### CI Pipeline

The GitHub Actions CI pipeline (`.github/workflows/ci.yml`) runs on every push to `main` and on all PRs:

1. **Repository layout validation** — Ensures required files exist
2. **Docker Compose validation** — Runs `make compose-config`
3. **Go API tests** — `make test-api`
4. **Python AI tests** — `make test-ai` (mock mode)
5. **Web app build** — `make test-web`

### Git Hooks

Pre-commit and pre-push hooks run `make test` automatically. Bypass with `--no-verify`:

```bash
git commit -m "message" --no-verify
git push origin branch --no-verify
```

---

## 🚢 Deployment

### Local Development (Docker Compose)

```bash
# Full stack (with local PostgreSQL)
docker compose --profile local-db up --build

# Without local DB (using Supabase or external PostgreSQL)
docker compose up --build

# Verify
make smoke
```

### Production Strategy

| Component | Recommended Deployment |
| --- | --- |
| `apps/web` | Vercel or Node.js container |
| `services/api` | Go container, public HTTP entry point |
| `services/ai` | Python container, internal HTTP (not browser-facing) |
| PostgreSQL | Managed Postgres (Supabase, RDS, Cloud SQL) |
| Object Storage | AWS S3, Cloudflare R2, or self-hosted MinIO |
| Redis | Managed Redis (for event streaming when needed) |

### Makefile Reference

| Command | Description |
| --- | --- |
| `make help` | Show all available commands |
| `make infra-up` | Start Redis and MinIO only |
| `make local-db-up` | Start bundled local PostgreSQL |
| `make services-up` | Start entire backend stack |
| `make services-down` | Stop the stack |
| `make db-migrate` | Apply schema migrations |
| `make db-seed` | Seed demo data |
| `make seed-demo-auth` | Create demo login accounts |
| `make compose-config` | Validate docker-compose.yml |
| `make smoke` | Run live health and auth checks |
| `make test` | Run all test suites |
| `make ps` | Show running services |
| `make logs` | Follow compose logs |

For the complete deployment guide, see [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md).

---

## 🗺 Roadmap

### ✅ Implemented (MVP)

- Document upload with multipart validation and duplicate detection
- Text extraction with OCR fallback for scanned documents
- Markdown normalization, table extraction, and content chunking
- 1536-dimensional vector embeddings via Google `text-embedding-004`
- RAG copilot with LangGraph state machine, citations, and confidence scores
- Stateful conversations with PostgreSQL-backed memory checkpointing
- Asset entity extraction and profile aggregation
- Root cause analysis with probable causes and recommendations
- Compliance gap detection (missing evidence, expired inspections)
- Dashboard with core metrics
- Report generation (CSV, PDF, DOCX) for 5 report types
- RBAC with 6 role levels and document-level access control
- Document versioning with evidence re-processing
- Better Auth integration (email/password + OAuth)
- Containerized full-stack deployment (Docker Compose)
- CI pipeline with GitHub Actions

### 🔜 Planned

- Production-hardened JWT/provider auth flow
- Per-section/per-page document ACLs
- Advanced query filters (document type, date range)
- Deep knowledge graph: relationship extraction, alias handling, graph expansion
- Full RCA: timeline, missing-data analysis, page-level evidence, conflict handling
- Regulatory rule ingestion for compliance
- Redis Streams event bus for async document processing
- OpenTelemetry tracing across all services
- Service-to-service auth (signed tokens / mTLS)
- Kubernetes + Helm deployment manifests

### 🚫 Out of Scope (Not in MVP)

- Full ERP/SAP/Maximo integration
- Real-time IoT/SCADA integration
- Full CAD/P&ID symbol parsing
- Production-grade regulatory certification
- Offline mobile app
- Custom LLM fine-tuning

---

## 📝 Architecture Decision Records

| ADR | Title | Status |
| --- | --- | --- |
| [ADR-0001](docs/adr/0001-microservice-boundaries.md) | Domain Boundaries and Consolidated Runtime | Accepted |
| [ADR-0002](docs/adr/0002-ai-service-modularization-and-security.md) | AI Service Modularization, Web Containerization, and Security Controls | Accepted |

**Key Architectural Decisions:**

1. **Consolidated Runtime** — Domain boundaries are maintained through PostgreSQL schemas and contracts, not separate deployable services. A new service is only created when there's a concrete need (independent scaling, separate ownership, distinct deployment lifecycle).

2. **AI Service Modularization** — The AI service was refactored from a monolithic `main.py` into modular components (`config`, `schemas`, `database`, `routes`, `utils`) for maintainability and testability.

3. **LangGraph for RAG** — The RAG pipeline uses a compiled LangGraph state machine instead of procedural lookups, enabling stateful reasoning, intent classification, memory persistence, and self-correcting citation validation.

---


### Team Workstreams

| Workstream | Folder(s) | Focus |
| --- | --- | --- |
| Frontend | `apps/web` | Dashboard, Upload UI, Copilot, Asset Profile, Reports |
| Backend API | `services/api` | Auth, uploads, document lifecycle, RBAC, reports |
| Retrieval & AI | `services/ai` | OCR, chunking, embeddings, RAG, citation validation |
| Asset & Graph | `services/api` + `services/ai` | Asset profiles, entity dedup, graph relationships |
| RCA & Compliance | `services/api` + `services/ai` | RCA agents, compliance rules, evidence matching |
| Database & Infra | `infra/` | Schemas, migrations, Compose, Helm, observability |
| Demo & QA | `data/`, `tests/`, `docs/` | Sample docs, test fixtures, evaluation checks |

---

## 📚 Further Documentation

| Document | Description |
| --- | --- |
| [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) | Detailed architecture and processing pipelines |
| [`docs/API_CONTRACTS.md`](docs/API_CONTRACTS.md) | Full API endpoint examples with request/response |
| [`docs/DATA_MODEL.md`](docs/DATA_MODEL.md) | Complete database schema and table reference |
| [`docs/MICROSERVICES.md`](docs/MICROSERVICES.md) | Service topology and communication rules |
| [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md) | Local and production deployment guide |
| [`docs/MVP_SCOPE.md`](docs/MVP_SCOPE.md) | MVP scope, status, and remaining gaps |
| [`docs/EVENTS.md`](docs/EVENTS.md) | Event contracts and async messaging |
| [`docs/DEMO_FLOW.md`](docs/DEMO_FLOW.md) | Demo script and sample dataset |
| [`docs/SERVICE_CATALOG.md`](docs/SERVICE_CATALOG.md) | Service registry with ports and ownership |
| [`docs/TEAM_WORKSTREAMS.md`](docs/TEAM_WORKSTREAMS.md) | Team ownership and workstream breakdown |

---

## 📄 License

This project is proprietary. All rights reserved.

---

<p align="center">
  Built with ❤️ by the PlantBrainAI team
</p>
