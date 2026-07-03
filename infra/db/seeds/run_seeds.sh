#!/usr/bin/env bash

# Exit immediately if a command exits with a non-zero status
set -e

# Resolve paths relative to this script
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
SQL_FILE="${SCRIPT_DIR}/demo_seed.sql"

echo "=== PlantBrainAI Database Seeder ==="

# Check if SQL file exists
if [ ! -f "$SQL_FILE" ]; then
    echo "Error: demo_seed.sql not found at $SQL_FILE"
    exit 1
fi

# Detect if the Docker container 'plantbrain-postgres' is running
CONTAINER_NAME="plantbrain-postgres"
if command -v docker &> /dev/null; then
    CONTAINER_RUNNING=$(docker ps --filter "name=${CONTAINER_NAME}" --filter "status=running" --format '{{.Names}}' || true)
else
    CONTAINER_RUNNING=""
fi

if [ -n "${CONTAINER_RUNNING}" ]; then
    echo "Detected running Docker container: ${CONTAINER_NAME}"
    echo "Running seeds via docker exec..."
    docker exec -i "${CONTAINER_NAME}" psql -U plantbrain -d plantbrain < "$SQL_FILE"
    echo "Seeds executed successfully inside container!"
else
    echo "Docker container ${CONTAINER_NAME} is not running or docker command is not available."
    echo "Attempting to run seeds locally via psql..."
    
    # Fallback to local psql connection
    DB_URL="postgresql://plantbrain:plantbrain@localhost:5432/plantbrain"
    
    if command -v psql &> /dev/null; then
        psql "$DB_URL" < "$SQL_FILE"
        echo "Seeds executed successfully via local psql!"
    else
        echo "Error: 'psql' command not found and Docker container is not running."
        echo "Please start the database container (e.g., 'docker compose up -d postgres') or install 'psql' to run seeds."
        exit 1
    fi
fi

echo "Seeding completed successfully!"
