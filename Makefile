.PHONY: build test test-unit test-integration test-db lint docker seed dev run tidy help-build help-clean

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
	docker build -f Dockerfile -t accept-midas:latest .

seed:
	./scripts/seed.sh

dev:
	docker compose -f docker-compose.yml up --build

run:
	go run ./cmd/midas

tidy:
	go mod tidy

# D33x-help-1 — Build the MIDAS User Guide.
# Source:   userguide/src/**/*.md  +  userguide/mkdocs.yml
# Output:   internal/httpapi/help/static/  (committed; //go:embed-ed by
#                                          internal/httpapi/help.go)
# Tool:     squidfunk/mkdocs-material via Docker — no Python install
#           required on the host.
help-build:
	@echo "Building MIDAS User Guide -> internal/httpapi/help/static/ ..."
	@mkdir -p internal/httpapi/help/static
	docker run --rm \
		-v "$(CURDIR)/userguide:/docs" \
		-v "$(CURDIR)/internal/httpapi/help:/host_help" \
		squidfunk/mkdocs-material:latest \
		build -f /docs/mkdocs.yml -d /host_help/static --clean
	@echo "✓ MIDAS User Guide built. Served at /help/ at runtime."

# Remove the generated MIDAS User Guide output. Source under userguide/
# is untouched. After running this, `make help-build` regenerates the
# static site; nothing else needs to know.
help-clean:
	rm -rf internal/httpapi/help/static
	@echo "✓ MIDAS User Guide static output removed."