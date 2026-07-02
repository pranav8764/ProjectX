# Docker

Root `docker-compose.yml` is the main local development entry point.

## Modes

Start shared infrastructure only:

```bash
make infra-up
```

Start infrastructure and placeholder microservice containers:

```bash
make services-up
```

Each service owns its own `Dockerfile` under `services/<service-name>/Dockerfile`.
