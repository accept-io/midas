.PHONY: build test test-unit test-integration test-db lint docker seed dev run tidy

build:
	mkdir -p bin
	go build -o bin/midas ./cmd/midas

test:
	@echo "Ensuring Docker Postgres is ready..."
	@docker compose down > /dev/null 2>&1 || true
	@docker compose up -d postgres > /dev/null 2>&1
	@sleep 5
	@echo "Running tests..."
	DATABASE_URL="postgresql://midas:midas@127.0.0.1:5432/midas?sslmode=disable" go test ./...

test-unit:
	@echo "Running unit tests (no database required)..."
	go test ./internal/decision -run "^(TestLifecycle|TestEvaluate).*" -v

test-integration:
	@echo "Ensuring Docker Postgres is ready..."
	@docker compose down > /dev/null 2>&1 || true
	@docker compose up -d postgres > /dev/null 2>&1
	@sleep 5
	@echo "Running integration tests with Postgres..."
	DATABASE_URL="postgresql://midas:midas@127.0.0.1:5432/midas?sslmode=disable" go test ./internal/decision -run "Postgres" -v

test-db:
	@echo "Starting Docker Postgres..."
	@docker compose down > /dev/null 2>&1 || true
	@docker compose up -d postgres
	@sleep 5
	@echo ""
	@echo "✓ Postgres is running"
	@echo "  Connection: postgresql://midas:midas@127.0.0.1:5432/midas"
	@echo ""
	@echo "To run tests: make test"
	@echo "To stop:      docker compose down"

lint:
	go vet ./...

docker:
	docker build -f deploy/docker/Dockerfile -t accept-midas:latest .

seed:
	./scripts/seed.sh

dev:
	docker compose -f deploy/compose/docker-compose.yml up --build

run:
	go run ./cmd/midas

tidy:
	go mod tidy