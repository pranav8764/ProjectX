# Event Contracts

The canonical async contract lives in `contracts/events/asyncapi.yaml`.

## Event Naming

Use past-tense domain events:

```text
document.uploaded
document.text_extracted
document.indexed
asset.upserted
compliance.gap_detected
```

## Event Envelope

Every event should use this envelope:

```json
{
  "eventId": "evt_123",
  "eventType": "document.uploaded",
  "occurredAt": "2026-07-02T10:00:00Z",
  "producer": "document-service",
  "organizationId": "org_123",
  "plantId": "plant_123",
  "correlationId": "req_123",
  "payload": {}
}
```

## Reliability Rules

- Consumers must be idempotent.
- Events must include `correlationId` for tracing.
- Events must not include raw document content.
- Services should store processing state before emitting the next event.
- Failed processing should emit an explicit failure event.

