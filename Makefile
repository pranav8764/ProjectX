.PHONY: help infra-up infra-down services-up services-down compose-config ps logs db-migrate db-seed seed-demo-auth local-db-up smoke test test-api test-ai test-web

# Load .env so DATABASE_URL etc. are available to db-migrate / db-seed
-include .env
export

help:
	@echo "PlantBrainAI developer commands"
	@echo "  make infra-up       Start Redis and MinIO (DB is DATABASE_URL / Supabase)"
	@echo "  make local-db-up    Start the bundled local Postgres (local-db profile)"
	@echo "  make services-up    Start API, AI, web, Redis, and MinIO"
	@echo "  make infra-down     Stop the local compose stack"
	@echo "  make services-down  Stop the local compose stack"
	@echo "  make db-migrate     Apply schema migrations to \$$DATABASE_URL"
	@echo "  make db-seed        Apply the demo dataset to \$$DATABASE_URL"
	@echo "  make seed-demo-auth Create demo persona logins (password PlantBrain#2026)"
	@echo "  make compose-config Validate docker-compose.yml"
	@echo "  make smoke          Check live API/AI health and API auth context"
	@echo "  make test           Run API, AI, and web validation checks"
	@echo "  make test-api       Run Go API tests"
	@echo "  make test-ai        Run AI integration tests in mock mode by default"
	@echo "  make test-web       Build the Next.js web app"
	@echo "  make ps             Show compose services"
	@echo "  make logs           Follow compose logs"

infra-up:
	docker compose up -d redis minio

local-db-up:
	docker compose --profile local-db up -d postgres

infra-down:
	docker compose down

# Apply schema + seeds directly to DATABASE_URL. Required for Supabase (no initdb mount);
# for the local-db profile the migration also runs automatically on first boot.
db-migrate:
	@test -n "$(DATABASE_URL)" || (echo "DATABASE_URL is not set (copy .env.example to .env)"; exit 1)
	psql "$(DATABASE_URL)" -v ON_ERROR_STOP=1 -f infra/db/migrations/001_init.sql

db-seed:
	@test -n "$(DATABASE_URL)" || (echo "DATABASE_URL is not set (copy .env.example to .env)"; exit 1)
	psql "$(DATABASE_URL)" -v ON_ERROR_STOP=1 -f infra/db/seeds/demo_seed.sql

# Create BetterAuth credential accounts for the demo personas (run after db-seed) so
# they can log in with email + password (default password PlantBrain#2026).
seed-demo-auth:
	node apps/web/scripts/seed-demo-auth.mjs

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
	@PY=$$( [ -f .venv/bin/python3 ] && echo .venv/bin/python3 || echo python3 ); \
	echo "Byte-compiling AI service..."; $$PY -m compileall -q services/ai/app tests; \
	echo "Running AI unit tests..."; $$PY tests/unit_ai_test.py; \
	if [ -n "$$PLANTBRAIN_TEST_URL" ]; then \
		echo "Running live integration tests against $$PLANTBRAIN_TEST_URL..."; \
		$$PY tests/integration_test.py; \
	else \
		echo "Skipping live integration tests (set PLANTBRAIN_TEST_URL to enable)"; \
	fi

test-web:
	npm --prefix apps/web ci
	npm --prefix apps/web run build
