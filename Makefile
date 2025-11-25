.PHONY: build up down logs migrate test lint lint-fix install-lint

build:
	go build -o app ./cmd/app

up:
	docker-compose up --build

down:
	docker-compose down --volumes

logs:
	docker-compose logs -f

migrate:
	docker run --rm \
		-v $(shell pwd)/migrations:/migrations \
		--network host \
		migrate/migrate:v4.15.2 \
		-path=/migrations \
		-database "postgres://postgres:postgres@localhost:5432/pr_service?sslmode=disable" \
		up

test:
	go test ./...

install-lint:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

lint:
	golangci-lint run ./...

lint-fix:
	golangci-lint run ./... --fix

# Дополнительные полезные команды
test-race:
	go test -race ./...

test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

bench:
	go test -bench=. -benchmem ./...

clean:
	rm -f app coverage.out

pre-commit: lint test

help:
	@echo "Available commands:"
	@echo "  build        - Build application"
	@echo "  up           - Start docker-compose"
	@echo "  down         - Stop docker-compose"
	@echo "  logs         - Show logs"
	@echo "  migrate      - Run database migrations"
	@echo "  test         - Run tests"
	@echo "  test-race    - Run tests with race detector"
	@echo "  test-coverage- Run tests with coverage report"
	@echo "  bench        - Run benchmarks"
	@echo "  install-lint - Install golangci-lint"
	@echo "  lint         - Run linters"
	@echo "  lint-fix     - Run linters with auto-fix"
	@echo "  pre-commit   - Run lint and test"
	@echo "  clean        - Clean build artifacts"