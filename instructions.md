# Problem Statement 8: AI for Industrial Knowledge Intelligence — Unified Asset & Operations Brain

## Product Documentation
### 1.1 Product Requirements Document
**Product Name**: PlantBrainAI — Unified Asset & Operations Brain  
**One-line Description**: PlantBrainAI is an AI-powered industrial knowledge platform that ingests plant(building) documents, extracts asset-level intelligence, builds a connected knowledge graph, and enables engineers, technicians, and compliance teams to ask operational questions with citations, RCA support, maintenance insights, and compliance gap detection.

**Problem**: Industrial plants store critical knowledge across disconnected sources:
- Equipment manuals
- P&IDs
- Maintenance logs
- Work orders
- Inspection reports
- SOPs
- Audit reports
- Safety documents
- Regulatory documents
- Incident and near-miss reports

This creates these problems:
- Engineers waste time searching for information.
- Maintenance teams miss past failure patterns.
- Compliance teams manually verify documents.
- New engineers cannot access retired experts’ knowledge.
- RCA is slow and incomplete.
- Important risks remain hidden across documents.

**Target Users**:
| User | Main Need |
|------|-----------|
| Maintenance Engineer | Find asset history, failures, manuals, and recommendations |
| Plant Operator | Quickly access SOPs and safety instructions |
| Reliability Engineer | Detect repeated failures and generate an RCA |
| Compliance Officer | Identify missing inspections and generate audit evidence |
| Plant Manager | View operational risk, knowledge gaps, and downtime risks |
| Field Technician | Ask mobile-friendly questions near the equipment |

**Core Value Proposition**: PlantBrainAI reduces industrial knowledge search time, improves maintenance decisions, detects compliance gaps, and preserves operational knowledge by turning scattered documents into a searchable, cited, AI-powered plant brain.

**Product Goals**:
| Goal | Description |
|------|-------------|
| Fast knowledge discovery | Answer industrial questions from uploaded documents within seconds |
| Evidence-backed answers | Every AI answer must cite source documents |
| Asset intelligence | Link documents, failures, work orders, inspections, and manuals by equipment tag |
| Maintenance support | Detect repeated failures and generate RCA suggestions |
| Compliance intelligence | Flag missing, expired, or conflicting compliance evidence |
| Mobile-first access | Make field technician usage practical |
| Hackathon-ready demo | Build a polished prototype with measurable impact |

**Non-goals for MVP**:
These should not be built in the first version:
- Full ERP/SAP/Maximo integration
- Real-time IoT/SCADA integration
- Full P&ID symbol-level CAD parsing
- Production-grade regulatory certification
- Offline mobile app
- Multi-plant enterprise deployment
- Fine-tuning a custom LLM

**MVP Scope**:
**Must-have**:
- Document upload
- OCR/text extraction
- Chunking and embeddings
- Metadata extraction
- Equipment/entity extraction
- Vector search
- RAG chatbot with citations
- Basic knowledge graph
- Asset profile page
- RCA assistant
- Compliance gap checker
- Dashboard with metrics

**Should-have**:
- Document versioning
- Confidence score
- Query filters by asset/document type/date
- Maintenance risk score
- Exportable report
- Graph visualization

**Could-have**:
- P&ID image parsing
- Multilingual query support
- Voice input for field technicians
- Automated alerts
- Work-order recommendation generation

### 1.2 Functional Requirements
**FR-1: User Authentication**
- Description: Users can sign up, log in, and access their plant workspace
- Priority: High
- Users: All
- Acceptance Criteria: Only authenticated users can upload/query documents
- Edge Cases: User token expired, User belongs to multiple organizations, User tries to access another plant’s documents, Deleted user still has old sessions, Role permissions are missing

**FR-2: Role-Based Access Control(Upload Allow all)**
| Role | Permissions |
|------|-------------|
| Admin | Manage users, documents, settings, |
| Plant Manager | View dashboard, reports, risks |
| Engineer | Upload docs, query, view asset intelligence |
| Technician | Query documents, view SOPs, view assets |
| Compliance Officer | View compliance gaps, generate reports |
| Viewer | Read-only access |
- Edge Cases: User uploads a sensitive document without permission, Technician asks for restricted compliance document, Admin removes user while query is running, Shared document has mixed permission level

**FR-3: Document Upload**
- Supported files: PDF, Scanned PDF, DOCX, XLSX/CSV, PNG/JPG scanned reports, TXT/Markdown, Maintenance logs, SOPs, Inspection reports
- Metadata Required: Document title, Document type, Plant/site, Department, Upload date, Uploaded by, Version, Asset tag, if detected, Confidence score, Audit and edit log
- Edge Cases: Empty file, Corrupt PDF, Password-protected PDF, Very large PDF, Duplicate upload, Same document with a newer version, Low-quality scan, Mixed-language document, Tables without headers, Rotated pages, Handwritten notes, Image-only PDF, File with no asset tag, File with multiple asset tags

**FR-4: OCR and Text Extraction**
The system should extract text from:
- Normal PDFs
- Scanned PDFs
- Images
- Tables
- Forms
- Maintenance logs
- Inspection checklists
OCR Pipeline:
- Detect file type
- Extract native text if available
- If no text, run OCR
- Detect tables separately
- Store raw text
- Store page-level text
- Store extraction confidence
Edge Cases:
- Bad lighting in scanned image
- Stamp/signature overlaps text
- Tables split across pages
- Text in diagrams
- Multi-column PDF
- Header/footer repeated on every page
- OCR hallucinated characters
- Similar-looking asset tags: P-101 vs P-IOI
- Units misread: bar, psi, °C

**FR-5: Document Classification** (try to store every document in markdown)
The AI should classify uploaded documents:
- OEM Manual
- SOP
- Maintenance Work Order
- Inspection Report
- Incident Report
- Audit Report
- Safety Procedure
- P&ID
- Compliance Document
- Training Document
- Unknown
Edge Cases:
- One document contains multiple types
- Document type is wrongly named
- No title page
- Old template format
- Handwritten document
- Document classification confidence is low

**FR-6: Entity Extraction**
The system should extract industrial entities:
| Entity | Example |
|--------|---------|
| Equipment Tag | P-101, HX-204, V-301 |
| Equipment Type | Pump, valve, compressor, boiler |
| Failure Type | Leakage, overheating, vibration |
| Maintenance Action | Bearing replacement, seal inspection |
| Date | 12 Jan 2025 |
| Person/Team | Mechanical Maintenance Team |
| Location | Boiler Area, Unit-2 |
| Parameter | Temperature, pressure, vibration |
| Regulation | Factory Act, OISD, PESO |
| Severity | Low, Medium, High, Critical |
Edge Cases:
- Same asset has multiple names
- Asset tag appears inside unrelated text
- Different plants use same asset tag
- Entity appears in table only
- Abbreviations: PMP-101, P-101, Pump 101
- Wrong unit extraction
- Missing dates
- Conflicting values across documents

**FR-7: Knowledge Graph Creation**
The system should create relationships like:
- Asset → has_manual → Document
- Asset → had_failure → Failure
- Failure → found_in → Work Order
- Asset → inspected_on → Inspection
- Inspection → has_status → Pass/Fail
- Asset → governed_by → Regulation
- Failure → probable_cause → Cause
- SOP → applies_to → Asset Type
Example:
Pump P-101 → had_failure → Seal Leakage → mentioned_in → Work Order WO-223
Edge Cases:
- Entity confidence is low
- Duplicate nodes
- Conflicting relationships
- Same document produces repeated relations
- Asset renamed over time
- Deleted document should remove graph links
- New version should update old graph links

**FR-8: RAG Copilot**
Users can ask questions like:
- “What is the maintenance history of Pump P-101?”
- “Why did Compressor C-204 fail repeatedly?”
- “Show SOP for boiler startup.”
- “Which assets have overdue inspection?”
- “Generate RCA for leakage in P-101.”
- “What safety precautions apply before hot work?”
Answer Requirements:
Every answer must include:
- Direct answer
- Source citations
- Confidence score
- Related documents
- Related assets
- Missing information warning, if applicable
Edge Cases:
- No relevant document found
- Relevant document found but confidence low
- Question asks outside document scope
- User asks for unsafe operational instruction
- User asks for legal/regulatory final decision
- Conflicting documents found
- Old version conflicts with new version
- Query contains wrong asset tag
- Query is too vague
- User asks in Hindi/vernacular language

**FR-9: Asset Profile Page**
Each asset should have a page showing:
- Asset tag
- Asset type
- Location
- Linked documents
- Failure history
- Maintenance history
- Inspection status
- Open risks
- Compliance gaps
- Recommended actions
- Timeline view
- Knowledge graph view
Edge Cases:
- Asset has no documents
- Asset appears in many plants
- Asset has conflicting names
- Asset has no recent inspection
- Asset risk score cannot be calculated
- Asset has documents but no maintenance history

**FR-10: RCA Assistant**
The RCA assistant should generate:
- Problem summary
- Timeline of events
- Repeated failure patterns
- Probable causes
- Supporting evidence
- Recommended corrective actions
- Missing data
- Confidence level
RCA Methods:
- 5 Whys
- Fishbone-style categories
- Failure pattern comparison
- Similar historical incidents
Edge Cases:
- Not enough failure data
- Conflicting maintenance logs
- Root cause cannot be determined
- Same symptom has multiple possible causes
- User asks for definitive cause without evidence
- LLM generates unsupported claim
- RCA is based on outdated document

**FR-11: Compliance Gap Detection**
The system should detect:
- Missing inspection records
- Expired certificates
- Missing SOPs
- Incomplete audit evidence
- Non-conformance reports
- Missing maintenance proof
- Conflicts between procedure and regulation
Edge Cases:
- Regulation document not uploaded
- Compliance checklist is incomplete
- Different compliance standards conflict
- Expiry date not found
- Evidence exists but OCR missed it
- User asks for legal guarantee
- Compliance status depends on external law update

**FR-12: Lessons Learned Engine**
The system should analyze:
- Incident reports
- Near-miss reports
- Audit findings
- Repeated failures
- Quality non-conformances
Then surface:
- Similar past incidents
- Recurring root causes
- High-risk assets
- Preventive recommendations
Edge Cases:
- Incident report has vague language
- Same incident duplicated
- Near-miss has no asset tag
- Historical data is biased/incomplete
- Pattern is statistically weak

**FR-13: Dashboard**
Dashboard should show:
- Total documents processed
- Assets discovered
- Critical assets
- Repeated failures
- Compliance gaps
- Query success rate
- Top searched assets
- Document processing status
- Knowledge graph completeness
- Time saved estimate
Edge Cases:
- Empty dashboard for new user
- Metrics delayed due to async processing
- Data processing failed
- Risk score missing
- User role hides some metrics

**FR-14: Report Generation**
Users should export:
- Asset summary report
- RCA report
- Compliance gap report
- Audit evidence package
- Query answer with citations
Formats:
- PDF
- DOCX
- CSV for tabular data
Edge Cases:
- Report includes restricted document
- Citation source deleted
- Large report timeout
- Missing logo/company metadata
- PDF generation fails

### 1.3 Non-Functional Requirements
**Performance**
| Requirement | Target |
|-------------|--------|
| Document upload response | < 2 sec initial acknowledgement |
| OCR processing | Async background job |
| Chat response | 3–8 sec for normal query |
| Vector search | < 1 sec |
| Dashboard load | < 2 sec |
| Asset page load | < 2 sec |
| Report generation | < 20 sec |

**Scalability**
MVP should support:
- 1 organization
- 3–5 plants
- 100–500 documents
- 5,000–50,000 chunks
- 100–1,000 assets
- 10–50 concurrent users
Future version should support:
- Multi-tenant organizations
- Millions of chunks
- Multi-plant deployments
- Streaming ingestion from ERP/CMMS/QMS

**Security**
- JWT-based authentication
- Role-based authorization
- File access isolation
- Signed URLs for document access
- Encryption at rest
- Encryption in transit
- Audit logs for document access
- No AI answer without permission-checked sources

**Reliability**
- Background jobs should be retryable
- Failed OCR should not break entire document pipeline
- Partial processing should be visible
- Document processing status should be transparent
- AI failures should return graceful fallback

**Observability**
Track:
- API latency
- OCR failures
- Embedding failures
- LLM failures
- Query latency
- Retrieval quality
- Hallucination reports
- User feedback
- Processing queue length

**AI Safety**
- No unsupported answer
- No answer without citations for document-based queries
- Show “not enough evidence” when needed
- Mention conflicting sources
- Separate facts from recommendations
- Do not claim legal/compliance certification
- Human approval required for critical maintenance/compliance actions

### 1.4 UX Documentation
**Information Architecture**
Main navigation:
- Dashboard
- Documents
- Assets
- Copilot
- RCA Assistant
- Compliance
- Knowledge Graph
- Reports
- Admin Settings

**User Persona 1: Maintenance Engineer**
Name: Arjun
Goal: Quickly understand why an asset keeps failing.
Pain: Maintenance logs are scattered across PDFs and Excel sheets.
PlantBrainAI Usage: Searches asset tag, sees timeline, asks RCA assistant.
Scenario: Arjun searches P-101. The system shows all failures, linked work orders, manual sections, and probable root causes.
User Story: As a maintenance engineer, I want to view the complete asset maintenance history so that I can identify repeated failure patterns.

**User Persona 2: Field Technician**
Name: Ravi
Goal: Get correct SOP while standing near equipment.
Pain: SOPs are hard to find on mobile.
PlantBrainAI Usage: Opens mobile app, searches “startup procedure for boiler B-12”.
User Story: As a technician, I want mobile access to SOPs so that I can follow safe procedures during field work.

**User Persona 3: Compliance Officer**
Name: Meera
Goal: Prepare audit evidence.
Pain: Inspection records and compliance docs are stored in multiple folders.
PlantBrainAI Usage: Opens compliance dashboard and exports missing evidence report.
User Story: As a compliance officer, I want missing inspection records flagged automatically so that I can fix audit gaps before inspection.

**User Persona 4: Plant Manager**
Name: Suresh
Goal: See risk across plant assets.
Pain: No single view of operational knowledge gaps.
PlantBrainAI Usage: Views dashboard with high-risk assets and unresolved compliance issues.
User Story: As a plant manager, I want a risk dashboard so that I can prioritize maintenance and compliance actions.

### 1.5 User Scenarios
**Scenario 1: Upload Maintenance Records**
Engineer uploads maintenance log PDF.
System extracts text.
System detects asset tags.
System creates chunks.
System creates embeddings.
System updates knowledge graph.
Asset page shows new maintenance history.
Edge Cases:
- Upload fails midway
- OCR confidence low
- No asset tag found
- Duplicate document detected
- User cancels upload

**Scenario 2: Ask Asset Question**
User asks:
“Why does Pump P-101 keep failing?”
System response:
Finds documents mentioning P-101
Retrieves work orders and inspection reports
Detects repeated seal leakage
Shows timeline
Suggests probable cause
Cites documents
Shows confidence score
Edge Cases:
- Only one failure found
- Multiple possible causes
- Manual contradicts maintenance notes
- No inspection report exists
- User lacks permission for one source

**Scenario 3: Compliance Audit**
User opens Compliance page.
System shows:
- 7 overdue inspections
- 3 missing SOPs
- 2 expired certificates
- 5 assets without recent maintenance evidence
Edge Cases:
- Compliance rules not uploaded
- Expiry date unreadable
- Asset retired but still shown
- Document version outdated
- Evidence exists in image-only PDF but OCR failed

### 1.6 Architecture Design Document
**Recommended Tech Stack**
| Layer | Tech |
|-------|------|
| Frontend | Next.js + TypeScript |
| Backend API | Go |
| AI Service | Python FastAPI |
| Database | PostgreSQL |
| Vector DB | pgvector |
| Object Storage | S3-compatible storage / local MinIO / Cloudflare R2 |
| Queue | Redis Queue / Celery / BullMQ / NATS |
| OCR | Tesseract / PaddleOCR |
| LLM | Groq (LLAMA 3.3 70B)/ Gemini / OpenAI |
| Embeddings | Gemini embeddings / sentence-transformers |
| Graph | Neo4j or PostgreSQL graph-style schema/ D3 js |
| Auth | Clerk / BetterAuth |
| Deployment | Docker Compose for demo, Railway/Vercel for hosted MVP |

**High-Level Architecture**
User
 |
 v
Next.js Frontend
 |
 v
Backend API Gateway
 |
 |------ Auth Service
 |------ Document Service
 |------ Asset Service
 |------ Query Service
 |------ Compliance Service
 |------ Report Service
 |
 v
PostgreSQL + pgvector
 |
 v
Object Storage
 |
 v
Background Worker Queue
 |
 v
Python AI Service
 |------ OCR Pipeline
 |------ Entity Extraction
 |------ Embedding Generator
 |------ RAG Orchestrator
 |------ RCA Agent
 |------ Compliance Agent
 |------ Graph Builder
 |
 v
Knowledge Graph / Graph Tables


**Core Services**
1. **Document Service**
   Responsibilities:
   - Upload file
   - Validate file
   - Store original file
   - Track processing status
   - Manage versions
   - Delete/archive documents
   Important APIs:
   - POST /api/documents/upload
   - GET /api/documents
   - GET /api/documents/{id}
   - DELETE /api/documents/{id}
   - GET /api/documents/{id}/status


2. **AI Processing Service**
   Responsibilities:
   - OCR
   - Text extraction
   - Chunking
   - Entity extraction
   - Embedding generation
   - Document classification
   - Summary generation
   Processing statuses:
   - UPLOADED
   - EXTRACTING_TEXT
   - OCR_RUNNING
   - CLASSIFYING
   - CHUNKING
   - EXTRACTING_ENTITIES
   - GENERATING_EMBEDDINGS
   - BUILDING_GRAPH
   - COMPLETED
   - FAILED
   - PARTIAL_SUCCESS


3. **Query/RAG Service**
   Responsibilities:
   - Accept natural language question
   - Detect intent
   - Apply permissions
   - Retrieve relevant chunks
   - Re-rank results
   - Generate cited answer
   - Return confidence score
   RAG flow:
   User Query
    -> Query Rewrite
    -> Intent Detection
    -> Entity/Asset Detection
    -> Permission Filter
    -> Vector Search
    -> Keyword Search
    -> Graph Expansion
    -> Reranking
    -> Context Builder
    -> LLM Answer
    -> Citation Validator
    -> Final Response


4. **Knowledge Graph Service**
   Responsibilities:
   - Store entities
   - Store relationships
   - Resolve duplicate asset names
   - Link assets to documents
   - Link failures to work orders
   - Link regulations to evidence
   Graph entities:
   - Asset
   - Document
   - Failure
   - Inspection
   - WorkOrder
   - SOP
   - Regulation
   - Location
   - Person
   - Department
   - MaintenanceAction
   - ComplianceRequirement

   Graph relationships:
   - MENTIONED_IN
   - HAS_FAILURE
   - HAS_INSPECTION
   - HAS_WORK_ORDER
   - REQUIRES_SOP
   - GOVERNED_BY
   - LOCATED_IN
   - PERFORMED_BY
   - HAS_RECOMMENDATION
   - HAS_EVIDENCE


5. **RCA Agent**
   Inputs:
   - Asset ID
   - Failure description
   - Work order history
   - Inspection history
   - OEM manual sections
   - Similar incidents
   Outputs:
   - Problem statement
   - Timeline
   - Probable causes
   - Evidence
   - Recommended actions
   - Missing data
   - Confidence score

6. **Compliance Agent**
   Inputs:
   - Compliance checklist
   - SOPs
   - Inspection records
   - Asset list
   - Certificate dates
   - Audit documents
   Outputs:
   - Gap list
   - Severity
   - Required action
   - Evidence document
   - Missing evidence
   - Report export

### 1.7 Data Model
**Tables**
- **users**
  - id
  - name
  - Email
  - mobile no
  - role
  - organization_id
  - created_at

- **organizations**
  - id
  - name
  - industry
  - created_at

- **plants**
  - id
  - organization_id
  - name
  - location
  - created_at

- **documents**
  - id
  - organization_id
  - plant_id
  - title
  - file_url
  - file_type
  - document_type
  - version
  - status
  - uploaded_by
  - ocr_confidence
  - classification_confidence
  - created_at
  - updated_at

- **document_chunks**
  - id
  - document_id
  - page_no
  - chunk_text
  - embedding
  - metadata_json
  - created_at

- **assets**
  - id
  - plant_id
  - asset_tag
  - asset_name
  - asset_type
  - location
  - criticality
  - risk_score
  - created_at
  - updated_at

- **extracted_entities**
  - id
  - document_id
  - chunk_id
  - entity_type
  - entity_value
  - normalized_value
  - confidence
  - page_no
  - created_at

- **relationships**
  - id
  - source_entity_id
  - target_entity_id
  - relationship_type
  - confidence
  - document_id
  - created_at

- **queries**
  - id
  - user_id
  - query_text
  - answer_text
  - confidence
  - created_at

- **citations**
  - id
  - query_id
  - document_id
  - chunk_id
  - page_no
  - quoted_text
  - score
  - created_at

- **compliance_gaps**
  - id
  - asset_id
  - gap_type
  - description
  - severity
  - evidence_document_id
  - status
  - created_at

- **rca_reports**
  - id
  - asset_id
  - failure_summary
  - probable_causes
  - recommendations
  - confidence
  - created_by
  - created_at


### 1.8 API Design
**Document Upload(Multer)**
POST /api/documents/upload

Request:
{
  "plantId": "plant_123",
  "documentType": "maintenance_report",
  "file": "binary"
}

Response:
{
  "documentId": "doc_123",
  "status": "UPLOADED",
  "message": "Document uploaded and queued for processing"
}


**Ask Copilot**
POST /api/copilot/query

Request:
{
  "question": "Why did Pump P-101 fail repeatedly?",
  "plantId": "plant_123",
  "filters": {
    "assetTag": "P-101",
    "documentTypes": ["work_order", "inspection_report", "manual"]
  }
}

Response:
{
  "answer": "Pump P-101 shows repeated seal leakage across three work orders...",
  "confidence": 0.82,
  "citations": [
    {
      "documentTitle": "WO-223 Maintenance Log",
      "page": 2,
      "snippet": "Seal leakage observed near bearing housing..."
    }
  ],
  "relatedAssets": ["P-101"],
  "missingInfo": ["No vibration report found after March 2025"]
}


**Get Asset Profile**
GET /api/assets/{assetId}

Response:
{
  "assetTag": "P-101",
  "assetType": "Pump",
  "location": "Unit-2",
  "riskScore": 78,
  "failures": [],
  "documents": [],
  "complianceGaps": []
}


**Generate RCA**
POST /api/rca/generate

Request:
{
  "assetTag": "P-101",
  "failureDescription": "Repeated seal leakage"
}

Response:
{
  "summary": "P-101 has repeated seal leakage...",
  "probableCauses": [],
  "recommendations": [],
  "confidence": 0.76,
  "citations": []
}


### 1.9 AI Design
**Document Ingestion Pipeline**
Store in MD format optimised for LLM, create a layout aware parsing pipeline, try Marker/MinerU or Llama parse/StrucTexT
File Upload
 -> File Validation
 -> Store Original File
 -> Extract Text
 -> OCR if Needed
 -> Clean Text
 -> Detect Tables
 -> Classify Document
 -> Chunk Text
 -> Extract Entities
 -> Normalize Asset Tags
 -> Generate Embeddings
 -> Store in pgvector
 -> Build Knowledge Graph
 -> Generate Document Summary
 -> Mark Completed

**Chunking Strategy - RecursiveTextSplitter**
Use hybrid chunking:
- 500–800 tokens per chunk
- 100-token overlap
- Page-level metadata
- Section-title metadata
- Table chunks stored separately
- Preserve asset tags and dates in metadata

**Retrieval Strategy**
Use hybrid retrieval:
- Vector similarity search
- Keyword search for exact asset tags
- Metadata filters
- Knowledge graph expansion
- Reranking
- Citation validation

**Why Hybrid Retrieval Is Required**
Industrial queries often contain exact identifiers like P-101, HX-204, or WO-331. Pure vector search may miss exact tags, so keyword and metadata search are required.

### 1.10 Edge Case Handling
**Document Edge Cases**
| Edge Case | Handling |
|-----------|----------|
| Corrupt file | Reject with clear error |
| Password-protected PDF | Ask user to upload unlocked file |
| Duplicate file | Show duplicate warning |
| New version uploaded | Link as newer version |
| Low OCR confidence | Mark as low-confidence and allow manual review |
| No text extracted | Mark failed and show reason |
| Very large file | Process asynchronously |
| Unknown document type | Classify as Unknown and continue |
| Multiple asset tags | Link the document to all detected assets |
| No asset tag | Store document but do not link to asset |

**AI Answer Edge Cases - use CRAG**
| Edge Case | Handling |
|-----------|----------|
| No evidence found | Say no evidence found |
| Low confidence | Show warning |
| Conflicting sources | Show both sources and conflict |
| Outdated document | Prefer latest version |
| User asks beyond docs | Refuse to guess |
| Unsafe instruction | Recommend authorized procedure review |
| Missing citation | Do not show answer as factual |
| Hallucination risk | Validate answer against retrieved chunks |

**Knowledge Graph Edge Cases**
| Edge Case | Handling |
|-----------|----------|
| Duplicate asset nodes | Normalize by plant + asset tag |
| Asset renamed | Store alias relationship |
| Deleted document | Remove or deactivate linked edges |
| Conflicting relationships | Keep both with source evidence |
| Low confidence edge | Mark as unverified |
| Graph too dense | Limit visualization depth |

**Compliance Edge Cases**
| Edge Case | Handling |
|-----------|----------|
| Missing standard document | Mark compliance check incomplete |
| Expiry date missing | Flag for manual review |
| Evidence unreadable | Mark OCR issue |
| Different versions conflict | Use newest approved document |
| Legal decision requested | Provide evidence summary, not legal certification |


### 1.11 Testing Documentation
**Test Plan**
**Unit Testing**
Test:
- File validation
- Document classification
- Chunking
- Entity extraction
- Asset tag normalization
- Permission checks
- Citation formatter
- Risk score calculation
- Compliance gap detection

**Integration Testing**
Test:
- Upload → OCR → Chunk → Embed → Query
- Upload → Entity extraction → Graph creation
- Query → Retrieval → LLM → Citation validation
- Asset page → Linked documents → RCA
- Compliance page → Gap generation → Report export

**System Testing**
Test complete user flows:
- Upload maintenance report
- Upload manual
- Ask an asset question
- Generate RCA
- View graph
- Export report

**AI Evaluation Testing**
| Test | Metric |
|------|--------|
| Entity extraction | Precision/Recall |
| Retrieval | Top-k accuracy |
| RAG answer | Faithfulness |
| Citation quality | Citation correctness |
| RCA | Expert usefulness score |
| Compliance gap detection | Accuracy |
| Hallucination | Unsupported claim rate |

**Security Testing**
- Unauthorized file access
- Role bypass
- Query permission leakage
- Prompt injection in documents
- Malicious PDF upload
- Large file DoS
- API rate limit
- JWT expiry

**Performance Testing**
- 100 documents upload
- 10 concurrent queries
- 50,000 chunks vector search
- Dashboard load under 2 sec
- LLM timeout handling
- Queue retry behavior

### 1.12 Sample Test Cases
| Test Case | Input | Expected Output |
|-----------|-------|-----------------|
| Upload valid PDF | Maintenance report PDF | Status becomes COMPLETED |
| Upload corrupt PDF | Broken file | Error message |
| Ask known asset query | “History of P-101” | Answer with citations |
| Ask unknown asset query | “History of P-999” | No evidence found |
| Duplicate document | Same PDF twice | Duplicate warning |
| Low OCR scan | Blurry image | Low confidence flag |
| Compliance missing inspection | Asset without inspection | Gap created |
| Conflicting docs | Two different inspection dates | Conflict warning |
| Unauthorized query | Technician asks restricted doc | Access denied |
| Prompt injection doc | “Ignore previous instructions” inside the PDF | Ignored by the model |


## 2. Process Documentation
### 2.1 Strategy Roadmap
**Phase 1: Hackathon MVP**
Goal: Prove core value.
Build:
- Upload documents
- Extract text/OCR
- Create embeddings
- RAG chatbot with citations
- Basic asset extraction
- Asset profile
- Basic RCA
- Compliance gap demo
- Dashboard

**Phase 2: Strong Prototype**
Goal: Improve intelligence.
Build:
- Knowledge graph visualization
- Better entity extraction
- Better document classification
- Multi-document RCA
- Report generation
- Feedback loop
- Risk scoring

**Phase 3: Production-Ready**
Goal: Enterprise readiness.
Build:
- Multi-tenant support
- ERP/CMMS/QMS connectors
- Advanced role permissions
- Audit logs
- Monitoring
- Real-time alerts
- Advanced compliance workflows

### 2.2 Technology Roadmap
**Week 0.5**
- Finalize schema
- Build frontend shell
- Build backend APIs
- Implement file upload
- Store files
- Set up PostgreSQL + pgvector

**Week 1**
- OCR/text extraction
- Chunking
- Embeddings
- Vector search
- Basic RAG endpoint

**Week 1.5**
- Entity extraction
- Asset profile
- Knowledge graph tables
- Citation system

**Week 2**
- RCA agent
- Compliance gap checker
- Dashboard
- Report export
- Demo polish

### 2.3 Release Roadmap
**Release v0.1 — Internal Demo**
Features:
- Upload PDF
- Process text
- Ask basic questions
- Show citations

**Release v0.2 — Hackathon MVP**
Features:
- Asset extraction
- Asset page
- RCA assistant
- Compliance page
- Dashboard

**Release v0.3 — Advanced Prototype**
Features:
- Knowledge graph visualization
- Version control
- Better OCR confidence
- Report export

**Release v1.0 — Pilot Version**
Features:
- Multi-user org
- Role-based access
- Audit logs
- Production deployment
- Monitoring

### 2.4 Metrics
The PDF evaluation focus includes entity extraction accuracy, query answer quality, knowledge graph linkage completeness, time-to-answer versus traditional search, compliance gap detection accuracy, and improvement in knowledge discovery.

**Product Metrics**
| Metric | Target |
|--------|--------|
| Document processing success rate | > 90% |
| Query answer citation rate | 100% for document questions |
| Average query response time | < 8 sec |
| Asset extraction precision | > 85% in demo |
| Compliance gap detection accuracy | > 80% in demo |
| RCA usefulness rating | > 4/5 from reviewers |
| Time-to-answer improvement | 70% faster than manual search |
| User task completion rate | > 85% |
| Unsupported AI claim rate | < 5% |

**Technical Metrics**
| Metric | Target |
|--------|--------|
| API uptime | > 99% for demo |
| OCR failure rate | < 10% |
| Vector search latency | < 1 sec |
| Upload failure rate | < 3% |
| Background job retry success | > 95% |
| LLM timeout rate | < 5% |

### 2.5 Standards
**Engineering Standards**
- REST API naming consistency
- Strict DTO validation
- Centralized error handling
- Database migrations
- Environment-based configuration
- No hardcoded secrets
- API rate limiting
- Request/response logging
- Background job retry logic

**AI Standards**
- All factual answers require citations
- Confidence score required
- No hidden reasoning shown
- No unsupported operational claims
- Retrieval context should be logged
- Prompt injection protection
- Human approval for critical recommendations

**Security Standards**
- RBAC for every API
- Signed URLs for file access
- Encrypted file storage
- Audit logs for document access
- Input sanitization
- Malware scanning for uploaded files, if possible
- Least-privilege access

**UX Standards**
- Mobile-friendly layout
- Clear processing status
- Clear confidence indicators
- No “AI magic” black box
- Show source documents
- Show missing information
- Use plain industrial language
- Avoid overloaded dashboards

## 3. Recommended Demo Story
**Demo Dataset**
Use 8–12 sample documents:
- Pump P-101 OEM Manual
- Pump P-101 Maintenance Log
- Work Order WO-223
- Inspection Report IR-91
- Boiler SOP
- Safety Checklist
- Audit Report
- Incident Report
- Compressor Manual
- Compliance Checklist

**Demo Flow**
- Upload documents.
- Show processing pipeline.
- Show extracted assets.
- Open asset profile for P-101.
- Ask: “Why is P-101 repeatedly failing?”
- Show cited RCA.
- Open knowledge graph.
- Open the compliance dashboard.
- Export RCA/compliance report.
- End with measurable impact.

**Best Demo Claim**
“PlantBrainAI turns scattered plant documents into a connected asset intelligence layer that helps engineers find answers, detect repeated failures, generate RCA, and identify compliance gaps with evidence-backed AI.”

## 4. Final MVP Build Priority
Build in this order:
1. Document upload
2. Text extraction/OCR
3. Chunking + embeddings
4. RAG chatbot with citations
5. Entity extraction
6. Asset profile
7. RCA assistant
8. Compliance gap checker
9. Dashboard
10. Report export
11. Graph visualization
Do not start with graph visualization. First, make the RAG + citations + asset intelligence solid.