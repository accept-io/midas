# MIDAS Testing Guide

This document explains how to run tests in the MIDAS project.

## Quick Start

```bash
# Run all tests (starts database automatically)
./test.sh

# Or use Make
make test
```

## Test Categories

MIDAS has three types of tests:

### 1. Unit Tests (No Database Required)
Fast in-memory tests that don't require external dependencies.

```bash
# Via script
./test.sh unit

# Via Make
make test-unit

# Manually
go test ./internal/audit/... ./internal/envelope/...
```

**Packages with unit tests:**
- `internal/audit` - Audit event creation, hash chain
- `internal/envelope` - Envelope state machine, five-section structure
- `internal/decision` - Orchestrator logic (in-memory tests only)

### 2. Integration Tests (Requires PostgreSQL)
Tests that validate behavior against a real PostgreSQL database.

```bash
# Via script (starts database automatically)
./test.sh db

# Via Make (starts database automatically)
make test-integration

# Manually (requires DATABASE_URL)
DATABASE_URL="postgresql://midas:midas@127.0.0.1:5432/midas?sslmode=disable" \
  go test ./internal/decision/... ./internal/store/postgres/... -run Postgres
```

**Packages with integration tests:**
- `internal/decision` - Postgres orchestrator tests (atomicity, persistence)
- `internal/store/postgres` - Store transaction tests

### 3. All Tests

```bash
# Via script (recommended)
./test.sh

# Via Make
make test

# Manually (requires DATABASE_URL)
DATABASE_URL="postgresql://midas:midas@127.0.0.1:5432/midas?sslmode=disable" \
  go test ./...
```

## Database Setup for Integration Tests

Integration tests require a PostgreSQL database. The test scripts handle this automatically, but you can also manage it manually:

### Automatic (Recommended)

The `./test.sh` and `make test` commands automatically:
1. Start Docker Postgres if not running
2. Set the correct `DATABASE_URL`
3. Run the tests

### Manual Setup

```bash
# Start database
docker compose up -d postgres

# Set environment variable
export DATABASE_URL="postgresql://midas:midas@127.0.0.1:5432/midas?sslmode=disable"

# Run tests
go test ./...

# Stop database when done
docker compose down
```

## Database Credentials

The Docker Postgres instance uses these credentials:
- **User:** `midas`
- **Password:** `midas`
- **Database:** `midas`
- **Port:** `5432`

**Important:** Integration tests will **skip gracefully** if `DATABASE_URL` is not set. You'll see:
```
?       github.com/accept-io/midas/internal/decision/...    [no test files]
```

## Coverage Reports

```bash
# Generate coverage report
./test.sh coverage

# View in browser
go tool cover -html=coverage.out
```

## Continuous Integration

For CI environments, use:

```bash
# Start database
docker compose up -d postgres
sleep 5

# Run tests with proper credentials
DATABASE_URL="postgresql://midas:midas@127.0.0.1:5432/midas?sslmode=disable" \
  go test ./... -v

# Cleanup
docker compose down -v
```

## Test Structure

```
internal/
├── audit/           ← Hash chain, event creation
│   ├── *_test.go
├── decision/        ← Orchestrator (unit + integration)
│   ├── orchestrator_test.go                    # Unit tests
│   ├── orchestrator_lifecycle_test.go          # Lifecycle tests
│   ├── orchestrator_postgres_test.go           # Integration tests
│   └── orchestrator_postgres_atomicity_test.go # Atomicity tests
├── envelope/        ← State machine, sections
│   └── envelope_test.go
└── store/postgres/  ← Store transactions
    └── store_test.go
```

## Common Issues

### "password authentication failed for user postgres"

This means `DATABASE_URL` is set but pointing to wrong credentials. Either:
1. Use `./test.sh` or `make test` which set the correct URL
2. Unset `DATABASE_URL` to skip Postgres tests: `unset DATABASE_URL && go test ./...`
3. Set it correctly: `export DATABASE_URL="postgresql://midas:midas@127.0.0.1:5432/midas?sslmode=disable"`

### "connection refused"

Database isn't running. Start it with:
```bash
docker compose up -d postgres
sleep 5  # Wait for initialization
```

### Tests hang or timeout

Database might be initializing. Wait 10 seconds and try again, or check logs:
```bash
docker compose logs postgres
```

## Best Practices

1. **Use the test script:** `./test.sh` handles all setup automatically
2. **Run unit tests frequently:** They're fast and don't need infrastructure
3. **Run integration tests before commits:** Ensures database behavior is correct
4. **Clean database between test runs:** `docker compose down -v` removes old data
5. **Check coverage periodically:** `./test.sh coverage` shows gaps

## Test Commands Summary

| Command | Description |
|---------|-------------|
| `./test.sh` | Run all tests (starts DB automatically) |
| `./test.sh unit` | Unit tests only (no DB) |
| `./test.sh db` | Integration tests only (starts DB) |
| `./test.sh coverage` | Generate coverage report |
| `make test` | All tests via Makefile |
| `make test-unit` | Unit tests via Makefile |
| `make test-integration` | Integration tests via Makefile |
| `go test ./...` | Raw Go command (manual DB setup) |
