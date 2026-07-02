# API Contracts

Canonical machine-readable contracts now live in `contracts/openapi`.

This document keeps the most important MVP examples readable for the team. Whenever an endpoint request or response changes, update both this file and the relevant OpenAPI contract.

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

`GET /api/assets/{assetId}`

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
  "failureDescription": "Repeated seal leakage"
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
