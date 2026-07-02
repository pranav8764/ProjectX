# Report Service

Owns report jobs and generated export metadata.

## Responsibilities

- Generate PDF, DOCX, and CSV reports
- Store report jobs and status
- Save generated files to object storage
- Emit report completion events

## Owns Schema

`report`

## Emits

- `report.generated`
- `report.failed`

