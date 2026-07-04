# API Contracts

Canonical machine-readable runtime contracts:

- `contracts/openapi/api.yaml` for the public Go API/BFF
- `contracts/openapi/ai.yaml` for the internal Python AI service

This document keeps the most important MVP examples readable for the team. Whenever an endpoint request or response changes, update this file and the relevant consolidated OpenAPI contract.

Authenticated `/api/*` examples use the development bearer token seeded by the local API when the database is empty:

```http
Authorization: Bearer dev-token
```

## Role Gates

The API enforces coarse RBAC after authentication. `Admin`, `Engineer`, `Technician`, `Compliance Officer`, `Plant Manager`, and `Viewer` are normalized with demo aliases such as `Maintenance Engineer`, `Reliability Engineer`, and `Field Technician`.

| Endpoint/action | Roles |
| --- | --- |
| Get `/api/me`; read documents/assets/graph; ask Copilot; upload documents | Any authenticated user in the plant workspace |
| Download unrestricted document sources | Any authenticated user in the plant workspace |
| Download restricted document sources | Admin, Plant Manager, Engineer, Compliance Officer |
| Download confidential document sources | Admin, Compliance Officer |
| Archive documents | Admin, Engineer |
| Dashboard metrics | Admin, Plant Manager, Engineer, Compliance Officer |
| Generate RCA | Admin, Plant Manager, Engineer |
| View compliance gaps | Admin, Plant Manager, Engineer, Compliance Officer |
| Run compliance scan | Admin, Plant Manager, Compliance Officer |
| List/create/download reports | Admin, Plant Manager, Engineer, Compliance Officer |

All plant-scoped endpoints also check that the requested `plantId` belongs to the authenticated user's organization and plant membership.

Document metadata visibility and source-file access are separate. Document list/detail responses may include:

```json
{
  "accessLevel": "restricted",
  "sensitivity": "sensitive",
  "allowedRoles": ["admin", "plant_manager", "engineer", "compliance_officer"],
  "sourceRestricted": true,
  "sourceDownloadAllowed": false
}
```

`accessLevel` values are `public`, `internal`, `restricted`, and `confidential`. `sensitivity` values are `standard`, `sensitive`, `confidential`, and `safety_critical`. Clients should show metadata for visible plant documents while disabling extracted evidence and original-source downloads when `sourceDownloadAllowed` is false.

## Upload Document

`POST /api/documents/upload`

Request is `multipart/form-data`:

```text
plantId=07eea2c3-...
documentType=maintenance_report
file=@WO-223.pdf
```

Response:

```json
{
  "documentId": "doc_123",
  "status": "UPLOADED",
  "message": "Document uploaded and queued for processing"
}
```

Current MVP response code: `200 OK`. Duplicate uploads return `200 OK` with `"duplicate": true`.

Supported upload extensions: `.pdf`, `.docx`, `.doc`, `.xlsx`, `.xls`, `.csv`, `.png`, `.jpg`, `.jpeg`, `.txt`, and `.md`.

## Document Lifecycle

`GET /api/documents?plantId={plantId}` returns non-archived documents for the current organization and optional plant.

`GET /api/documents/{id}` returns document metadata, current version, page text, chunks, extracted entities, and optional access metadata. Access-controlled deployments may omit or redact source-level extracted text that the caller cannot use as evidence.

`GET /api/documents/{id}/status` returns the latest processing status and processing-job progress.

`POST /api/documents/{id}/retry-processing` is available to `admin` and `engineer` roles. It requeues the current document version and calls the AI processing service again:

```json
{
  "documentId": "doc_123",
  "status": "QUEUED",
  "message": "Document processing retry queued"
}
```

If the AI service rejects the retry, the API returns a `FAILED` response with a `retryUrl` for the same endpoint. If a processing job is already active, the API returns `409 Conflict`.

`POST /api/documents/{id}/versions` uploads a newer source file for an existing document. The caller needs plant access and must satisfy the document source policy for restricted/confidential documents. The API makes the new version current, clears stale extracted evidence from the previous version, and queues AI processing:

```text
versionLabel=v2
file=@WO-223-revision.pdf
```

```json
{
  "documentId": "doc_123",
  "versionId": "version_456",
  "versionLabel": "v2",
  "status": "UPLOADED",
  "message": "Document version uploaded and queued for processing"
}
```

`GET /api/documents/{id}/download` returns a short-lived signed URL for the original uploaded file:

```json
{
  "documentId": "doc_123",
  "accessLevel": "restricted",
  "sensitivity": "sensitive",
  "downloadUrl": "http://localhost:9000/plantbrain-documents/uploads/...",
  "expiresAt": "2026-07-03T12:10:00Z",
  "expiresInSec": 600
}
```

Signed document URLs expire after 600 seconds. The endpoint returns `403 Forbidden` when plant access is valid but the caller is not in the document's `allowedRoles` source-download policy. Report downloads do not use signed object-storage URLs; they stream the generated file from `GET /api/reports/{id}/download`.

`DELETE /api/documents/{id}` archives the document, removes derived graph/search artifacts, and records an audit event.

## Ask Copilot

`POST /api/copilot/query`

Request:

```json
{
  "question": "Why did Pump P-101 fail repeatedly?",
  "plantId": "plant_123",
  "filters": {
    "assetTag": "P-101",
    "documentTypes": ["work_order", "inspection_report", "manual"]
  }
}
```

Response:

```json
{
  "answer": "Pump P-101 shows repeated seal leakage across three work orders.",
  "confidence": 0.82,
  "citations": [
    {
      "documentTitle": "WO-223 Maintenance Log",
      "accessLevel": "internal",
      "sensitivity": "standard",
      "sourceDownloadAllowed": true,
      "page": 2,
      "snippet": "Seal leakage observed near bearing housing."
    }
  ],
  "relatedAssets": ["P-101"],
  "missingInfo": ["No vibration report found after March 2025"]
}
```

## Get Asset Profile

`GET /api/assets/{id}`

`id` may be an asset UUID or an asset tag such as `P-101`.

Response:

```json
{
  "assetTag": "P-101",
  "assetType": "Pump",
  "location": "Unit-2",
  "riskScore": 78,
  "failures": [],
  "documents": [],
  "complianceGaps": []
}
```

## Generate RCA

`POST /api/rca/generate`

Request:

```json
{
  "assetTag": "P-101",
  "failureDescription": "Repeated seal leakage",
  "plantId": "plant_123"
}
```

Response:

```json
{
  "summary": "P-101 has repeated seal leakage.",
  "probableCauses": [],
  "recommendations": [],
  "confidence": 0.76,
  "citations": []
}
```

## Scan Compliance

`GET /api/compliance/gaps?plantId=plant_123` returns persisted gaps:

```json
[
  {
    "id": "gap_123",
    "assetTag": "P-101",
    "gapType": "MISSING_EVIDENCE",
    "severity": "HIGH",
    "status": "OPEN",
    "description": "No inspection evidence found for asset P-101."
  }
]
```

`POST /api/compliance/scan?plantId=plant_123`

Response:

```json
[
  {
    "assetTag": "P-101",
    "gapType": "MISSING_EVIDENCE",
    "severity": "HIGH",
    "status": "OPEN"
  }
]
```

## Create Report

`GET /api/reports?plantId=plant_123` returns the latest 25 report jobs visible to the user.

`POST /api/reports`

Request:

```json
{
  "plantId": "plant_123",
  "reportType": "asset_summary",
  "format": "pdf",
  "parameters": {}
}
```

Supported MVP report types:

- `asset_summary`
- `compliance_gap`
- `document_inventory`
- `rca_report`
- `query_answers`

Supported formats: `csv`, `pdf`, `docx`. CSV is the default when `format` is omitted.

Reports are generated synchronously in the current MVP and return `201 Created` on success.

Response:

```json
{
  "id": "report_123",
  "status": "COMPLETED",
  "createdAt": "2026-07-03T12:00:00Z",
  "format": "pdf",
  "downloadUrl": "/api/reports/report_123/download",
  "message": "Report generated successfully"
}
```

`GET /api/reports/{id}/download` streams the generated file with a format-specific content type:

- `text/csv; charset=utf-8`
- `application/pdf`
- `application/vnd.openxmlformats-officedocument.wordprocessingml.document`
