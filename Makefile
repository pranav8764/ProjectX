.PHONY: help infra-up infra-down services-up services-down compose-config ps logs db-seed smoke test test-api test-ai test-web

help:
	@echo "PlantBrainAI developer commands"
	@echo "  make infra-up       Start Postgres, Redis, and MinIO"
	@echo "  make services-up    Start API, AI, Postgres, Redis, and MinIO"
	@echo "  make infra-down     Stop the local compose stack"
	@echo "  make services-down  Stop the local compose stack"
	@echo "  make db-seed        Run database seeds (demo dataset)"
	@echo "  make compose-config Validate docker-compose.yml"
	@echo "  make smoke          Check live API/AI health and API auth context"
	@echo "  make test           Run API, AI, and web validation checks"
	@echo "  make test-api       Run Go API tests"
	@echo "  make test-ai        Run AI integration tests in mock mode by default"
	@echo "  make test-web       Build the Next.js web app"
	@echo "  make ps             Show compose services"
	@echo "  make logs           Follow compose logs"

infra-up:
	docker compose up -d postgres redis minio

infra-down:
	docker compose down

db-seed:
	./infra/db/seeds/run_seeds.sh

services-up:
	docker compose up -d

services-down:
	docker compose down

compose-config:
	docker compose config

smoke:
	./scripts/smoke.sh

ps:
	docker compose ps

logs:
	docker compose logs -f

test: test-api test-ai test-web

test-api:
	go -C services/api test ./...

test-ai:
	@if [ -f .venv/bin/python3 ]; then .venv/bin/python3 tests/integration_test.py; else python3 tests/integration_test.py; fi

test-web:
	npm --prefix apps/web ci
	npm --prefix apps/web run build
