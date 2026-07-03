# API Contracts

Canonical machine-readable runtime contracts:

- `contracts/openapi/api.yaml` for the public Go API/BFF
- `contracts/openapi/ai.yaml` for the internal Python AI service

Older `contracts/openapi/*-service.yaml` files are legacy planning sketches for possible future domain splits. They are not deployable service definitions.

This document keeps the most important MVP examples readable for the team. Whenever an endpoint request or response changes, update this file and the relevant consolidated OpenAPI contract.

Authenticated `/api/*` examples use the development bearer token seeded by the local API when the database is empty:

```http
Authorization: Bearer dev-token
```

## Role Gates

The API enforces coarse RBAC after authentication. `Admin`, `Engineer`, `Technician`, `Compliance Officer`, `Plant Manager`, and `Viewer` are normalized with demo aliases such as `Maintenance Engineer`, `Reliability Engineer`, and `Field Technician`.

| Endpoint/action | Roles |
| --- | --- |
| Read documents/assets/graph, ask Copilot, upload documents | Any authenticated user in the plant workspace |
| Archive documents | Admin, Engineer |
| Dashboard metrics | Admin, Plant Manager, Engineer, Compliance Officer |
| Generate RCA | Admin, Plant Manager, Engineer |
| View compliance gaps | Admin, Plant Manager, Engineer, Compliance Officer |
| Run compliance scan | Admin, Plant Manager, Compliance Officer |
| List/create/download reports | Admin, Plant Manager, Engineer, Compliance Officer |

## Upload Document

`POST /api/documents/upload`

Request:

```json
{
  "plantId": "plant_123",
  "documentType": "maintenance_report",
  "file": "binary"
}
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

## Document Lifecycle

`GET /api/documents/{id}` returns document metadata, current version, page text, chunks, and extracted entities.

`GET /api/documents/{id}/status` returns the latest processing status and processing-job progress.

`GET /api/documents/{id}/download` returns a short-lived signed URL for the original uploaded file:

```json
{
  "documentId": "doc_123",
  "downloadUrl": "http://localhost:9000/plantbrain-documents/uploads/...",
  "expiresAt": "2026-07-03T12:10:00Z",
  "expiresInSec": 600
}
```

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

Response:

```json
{
  "id": "report_123",
  "status": "COMPLETED",
  "format": "pdf",
  "downloadUrl": "/api/reports/report_123/download",
  "message": "Report generated successfully"
}
```
