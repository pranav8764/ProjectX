# Asset Service

Owns asset records, aliases, asset profile aggregation, timelines, and risk summaries.

## Responsibilities

- Upsert discovered assets
- Normalize asset tags and aliases
- Serve asset profiles
- Build asset timelines
- Calculate basic risk summaries

## Owns Schema

`asset`

## Consumes

- `document.entities_extracted`

