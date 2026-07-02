# Document Service

Owns document metadata, upload lifecycle, versions, status, and file pointers.

## Responsibilities

- Create upload sessions
- Validate metadata
- Store object-storage file references
- Track processing status
- Manage versions and duplicates
- Emit document lifecycle events

## Owns Schema

`document`

## Emits

- `document.uploaded`
- `document.version_created`
- `document.deleted`

