.PHONY: help infra-up infra-down services-up services-down ps logs

help:
	@echo "PlantBrainAI developer commands"
	@echo "  make infra-up     Start local Postgres, Redis, and MinIO"
	@echo "  make infra-down   Stop local infrastructure"
	@echo "  make services-up  Start placeholder microservices too"
	@echo "  make services-down Stop all compose services"
	@echo "  make ps           Show compose services"
	@echo "  make logs         Follow compose logs"

infra-up:
	docker compose up -d

infra-down:
	docker compose down

services-up:
	docker compose --profile services up -d

services-down:
	docker compose --profile services down

ps:
	docker compose ps

logs:
	docker compose --profile services logs -f
