# Audit Service

Owns immutable audit logs and access events.

## Responsibilities

- Store document access logs
- Store administrative actions
- Store AI query audit entries
- Support compliance audit evidence
- Consume audit-worthy events

## Owns Schema

`audit`

## Consumes

- All security-sensitive domain events

