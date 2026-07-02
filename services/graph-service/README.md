# Graph Service

Owns extracted entities, relationships, graph expansion, deduplication, and graph visualization APIs.

## Responsibilities

- Store entities and relationships
- Deduplicate graph nodes
- Keep source document evidence on edges
- Support graph expansion for retrieval
- Provide graph views for asset pages

## Owns Schema

`graph`

## Consumes

- `document.entities_extracted`
- `asset.upserted`

