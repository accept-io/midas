.PHONY: build test lint docker migrate seed dev run tidy

build:
	mkdir -p bin
	go build -o bin/midas ./cmd/midas

test:
	go test ./...

lint:
	go vet ./...

docker:
	docker build -f deploy/docker/Dockerfile -t accept-midas:latest .

migrate:
	./scripts/migrate.sh

seed:
	./scripts/seed.sh

dev:
	docker compose -f deploy/compose/docker-compose.yml up --build

run:
	go run ./cmd/midas

tidy:
	go mod tidy
