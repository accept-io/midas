#!/bin/bash
set -euo pipefail

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

MODE="${1:-all}"

# Prevent Git Bash from mangling paths
export MSYS_NO_PATHCONV=1

GO_TEST_CMD="go test -p 1 -count=1 ./... -v -timeout 120s"

# Helper function to run commands in Docker
run_in_docker() {
  local cmd="$1"

  docker run --rm \
    --network midas_default \
    -v "$(pwd):/app" \
    -w /app \
    -e DATABASE_URL="postgresql://midas:midas@postgres:5432/midas?sslmode=disable" \
    -e MIDAS_TEST_DATABASE_URL="postgresql://midas:midas@postgres:5432/midas?sslmode=disable" \
    golang:1.25-alpine \
    sh -c "$cmd"
}

# Helper function to wait for postgres to be ready
wait_for_postgres() {
  echo -e "${YELLOW}Waiting for Postgres to be ready...${NC}"

  for i in {1..30}; do
    if docker run --rm --network midas_default postgres:17-alpine \
      sh -c "PGPASSWORD=midas psql -h postgres -U midas -d postgres -c 'SELECT 1;'" > /dev/null 2>&1; then
      echo -e "${GREEN}✓ Postgres is ready${NC}"
      return 0
    fi
    echo -n "."
    sleep 1
  done

  echo ""
  echo -e "${RED}✗ Postgres failed to become ready${NC}"
  return 1
}

# Helper function to initialize database schema
init_database() {
  echo -e "${YELLOW}Initializing database schema...${NC}"

  # Drop and recreate database
  docker run --rm \
    --network midas_default \
    postgres:17-alpine \
    sh -c "
      export PGPASSWORD=midas
      psql -h postgres -U midas -d postgres -c 'DROP DATABASE IF EXISTS midas;'
      psql -h postgres -U midas -d postgres -c 'CREATE DATABASE midas;'
    " > /dev/null 2>&1

  # Apply schema from internal/store/postgres/schema.sql
  docker run --rm \
    --network midas_default \
    -v "$(pwd):/app" \
    -w /app \
    postgres:17-alpine \
    sh -c "export PGPASSWORD=midas && psql -h postgres -U midas -d midas -f /app/internal/store/postgres/schema.sql" > /dev/null 2>&1

  echo -e "${GREEN}✓ Database schema initialized${NC}"
}

case "$MODE" in
  all)
    echo -e "${GREEN}Running all MIDAS tests...${NC}"
    echo ""

    # Ensure Docker Postgres is running
    if ! docker compose ps | grep -q "postgres.*Up"; then
      echo -e "${YELLOW}Starting Docker Postgres...${NC}"
      docker compose down > /dev/null 2>&1 || true
      docker compose up -d postgres
      wait_for_postgres
    else
      echo -e "${GREEN}✓ Postgres is already running${NC}"
      wait_for_postgres
    fi

    # Initialize database with fresh schema
    init_database

    echo ""
    echo -e "${YELLOW}Running:${NC} ${GO_TEST_CMD}"
    run_in_docker "${GO_TEST_CMD}"
    ;;

  build)
    echo -e "${GREEN}Running build...${NC}"
    run_in_docker "go build ./..."
    ;;

  vet)
    echo -e "${GREEN}Running go vet...${NC}"
    run_in_docker "go vet ./..."
    ;;

  decision)
    echo -e "${GREEN}Running decision tests...${NC}"
    init_database
    run_in_docker "go test -p 1 -count=1 ./internal/decision/... -v -timeout 120s"
    ;;

  postgres)
    echo -e "${GREEN}Running postgres tests...${NC}"
    init_database
    run_in_docker "go test -p 1 -count=1 ./internal/store/postgres/... -v -timeout 120s"
    ;;

  *)
    echo -e "${RED}Unknown mode: $MODE${NC}"
    echo "Usage: $0 [all|build|vet|decision|postgres]"
    exit 1
    ;;
esac

echo ""
echo -e "${GREEN}✓ Tests complete${NC}"