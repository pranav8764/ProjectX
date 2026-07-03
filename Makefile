.PHONY: help infra-up infra-down services-up services-down compose-config ps logs db-seed

help:
	@echo "PlantBrainAI developer commands"
	@echo "  make infra-up       Start Postgres, Redis, and MinIO"
	@echo "  make services-up    Start API, AI, Postgres, Redis, and MinIO"
	@echo "  make infra-down     Stop the local compose stack"
	@echo "  make services-down  Stop the local compose stack"
	@echo "  make db-seed        Run database seeds (demo dataset)"
	@echo "  make compose-config Validate docker-compose.yml"
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

ps:
	docker compose ps

logs:
	docker compose logs -f
