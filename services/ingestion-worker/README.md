# Ingestion Worker

Processes uploaded documents asynchronously.

## Responsibilities

- Consume `document.uploaded`
- Extract native text
- Run OCR fallback
- Convert extracted content to markdown
- Detect tables and forms
- Chunk text
- Extract industrial entities
- Generate embeddings
- Store chunks and processing artifacts
- Emit indexing and failure events

## Owns Schema

`ingestion`

## Emits

- `document.processing_started`
- `document.text_extracted`
- `document.entities_extracted`
- `document.indexed`
- `document.processing_failed`

