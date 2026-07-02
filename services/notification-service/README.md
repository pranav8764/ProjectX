# Notification Service

Owns async alerts and notification delivery state.

## Responsibilities

- Processing failure alerts
- Compliance gap alerts
- Report ready notifications
- Future email, Slack, or webhook delivery

## Owns Schema

`notification`

## Consumes

- `document.processing_failed`
- `compliance.gap_detected`
- `report.generated`

