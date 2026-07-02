# Deployment

## Local MVP

Use Docker Compose for shared infrastructure:

```bash
make infra-up
```

Start placeholder services:

```bash
make services-up
```

## Service Runtime Strategy

Each service has its own `Dockerfile` and README. During implementation, replace the placeholder Dockerfile with the actual runtime for that service.

Recommended split:

- Frontend: Vercel or container
- API gateway: container
- Python AI services/workers: containers
- PostgreSQL + pgvector: managed Postgres or self-hosted
- Object storage: S3, R2, or MinIO
- Event bus: Redis Streams for MVP, Kafka/NATS/SQS later

## Kubernetes

Kubernetes placeholders live in `infra/k8s`. They are intentionally minimal until services have real health endpoints and runtime configs.

