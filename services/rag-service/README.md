# RAG Service

Owns retrieval, cited answers, query history, and answer feedback.

## Responsibilities

- Hybrid retrieval with vector, keyword, metadata, and graph expansion
- Reranking
- Context construction
- Cited answer generation
- Confidence scoring
- Query history and user feedback

## Owns Schema

`rag`

## Depends On

- `document-service`
- `asset-service`
- `graph-service`
- `ai-orchestrator-service`

