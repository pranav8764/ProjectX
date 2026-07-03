# Docker

The root `docker-compose.yml` is the main local development entry point.

## Modes

Start shared infrastructure only:

```bash
make infra-up
```

This starts PostgreSQL, Redis, and MinIO.

Start the full backend stack:

```bash
make services-up
```

This starts PostgreSQL, Redis, MinIO, `services/api`, and `services/ai`.

Validate compose configuration:

```bash
make compose-config
```

Only the two runnable backend services own Dockerfiles:

- `services/api/Dockerfile`
- `services/ai/Dockerfile`
