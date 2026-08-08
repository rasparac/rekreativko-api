.PHONY: help test test-unit test-integration test-all test-coverage test-verbose clean

# Default target
help:
	@echo "Available targets:"
	@echo "  make test              - Run unit tests only (default)"
	@echo "  make test-unit         - Run unit tests only"
	@echo "  make test-integration  - Run integration tests (requires Docker)"
	@echo "  make test-all          - Run both unit and integration tests"
	@echo "  make test-coverage     - Run tests with coverage report"
	@echo "  make test-verbose      - Run tests with verbose output"
	@echo "  make clean             - Clean test cache and containers"
	@echo "  make build             - Build all services"
	@echo "  make docker-build      - Build all Docker images"

# Run only unit tests (no integration tag)
test: test-unit

# Run unit tests explicitly
test-unit:
	@echo "Running unit tests..."
	@go test ./... -short

# Run integration tests (requires Docker)
test-integration:
	@echo "Running integration tests..."
	@echo "Note: This requires Docker to be running"
	@go test -tags=integration ./...

# Run all tests
test-all:
	@echo "Running all tests..."
	@go test -tags=integration ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	@go test -tags=integration -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run tests with verbose output
test-verbose:
	@echo "Running tests with verbose output..."
	@go test -tags=integration -v ./...

# Run integration tests for specific service
test-activity-integration:
	@echo "Running activity integration tests..."
	@go test -tags=integration -v ./activity/internal/infrastructure/persistence/...

test-identity-integration:
	@echo "Running identity integration tests..."
	@go test -tags=integration -v ./identity/internal/infrastructure/...

test-account-profile-integration:
	@echo "Running account-profile integration tests..."
	@go test -tags=integration -v ./account-profile/internal/infrastructure/...

# Clean test cache and Docker containers
clean:
	@echo "Cleaning test cache..."
	@go clean -testcache
	@echo "Stopping testcontainers..."
	@docker ps | grep testcontainers | awk '{print $$1}' | xargs -r docker kill || true
	@echo "Clean complete"

# Build all services
build:
	@echo "Building all services..."
	@go build -o bin/activity-api ./activity/cmd/api/...
	@go build -o bin/activity-cron ./activity/cmd/cron/...
	@go build -o bin/account-profile ./account-profile/cmd/api/...
	@go build -o bin/identity ./identity/cmd/api/...
	@go build -o bin/gateway ./gateway/cmd/api/...
	@go build -o bin/outbox-publisher ./outbox-publisher/cmd/worker/...
	@echo "Build complete! Binaries in ./bin/"

# Build specific services
build-activity:
	@echo "Building activity service..."
	@go build -o bin/activity-api ./activity/cmd/api/...
	@go build -o bin/activity-cron ./activity/cmd/cron/...

build-identity:
	@echo "Building identity service..."
	@go build -o bin/identity ./identity/cmd/api/...

build-account-profile:
	@echo "Building account-profile service..."
	@go build -o bin/account-profile ./account-profile/cmd/api/...

build-gateway:
	@echo "Building gateway service..."
	@go build -o bin/gateway ./gateway/cmd/api/...

# Run linter
lint:
	@echo "Running linter..."
	@golangci-lint run ./...

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...

# Tidy dependencies
tidy:
	@echo "Tidying dependencies..."
	@go mod tidy

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	@go mod download

# Run all checks (fmt, lint, test)
check: fmt lint test-all
	@echo "All checks passed!"

# Docker compose for local development
docker-up:
	@echo "Starting local services..."
	@docker-compose up -d

docker-down:
	@echo "Stopping local services..."
	@docker-compose down

# Build Docker images for all services
docker-build:
	@echo "Building Docker images for all services..."
	@docker-compose -f docker-compose.build.yml build

# Build Docker images for specific services
docker-build-activity:
	@docker-compose -f docker-compose.build.yml build activity activity-cron

docker-build-identity:
	@docker-compose -f docker-compose.build.yml build identity

docker-build-account-profile:
	@docker-compose -f docker-compose.build.yml build account-profile

docker-build-gateway:
	@docker-compose -f docker-compose.build.yml build gateway

# Database migrations (per service)
# Note: migrations are now per-service in their persistence folders
migrate-activity-up:
	@echo "Running activity migrations..."
	@migrate -path ./activity/internal/infrastructure/persistence/migrations -database "$(DB_URL)" up

migrate-identity-up:
	@echo "Running identity migrations..."
	@migrate -path ./identity/internal/infrastructure/persistence/migrations -database "$(DB_URL)" up

migrate-account-profile-up:
	@echo "Running account-profile migrations..."
	@migrate -path ./account-profile/internal/infrastructure/persistence/migrations -database "$(DB_URL)" up
