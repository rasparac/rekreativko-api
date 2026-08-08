# Rekreativko API

A microservices-based backend API for the Rekreativko platform, built with Go using Domain-Driven Design (DDD) principles.

## Architecture

This is a **service-centric monorepo** where each service is independently deployable but shares common infrastructure code.

### Services

- **Gateway** - API gateway with authentication, routing, and notification handling
- **Identity** - User authentication, registration, and account management
- **Account Profile** - User profiles, settings, and statistics
- **Activity** - Activity session templates and scheduling
- **Activity Cron** - Background jobs for activity-related tasks
- **Outbox Publisher** - Reliable event publishing from database outbox

### Technology Stack

- **Language**: Go 1.26
- **Database**: PostgreSQL
- **Message Broker**: NATS
- **Cache**: Redis
- **Observability**: OpenTelemetry, Jaeger, Prometheus, Grafana
- **Containerization**: Docker

## Project Structure

```
.
├── activity/              # Activity service
│   ├── cmd/
│   │   ├── api/          # HTTP API server
│   │   └── cron/         # Background job worker
│   └── internal/
│       ├── application/  # Use cases
│       ├── domain/       # Business logic
│       ├── infrastructure/
│       │   └── persistence/
│       │       └── migrations/
│       └── interfaces/   # HTTP handlers
├── account-profile/       # Account profile service
│   ├── cmd/api/
│   └── internal/
├── identity/             # Identity service
│   ├── cmd/api/
│   └── internal/
├── gateway/              # API Gateway
│   ├── cmd/api/
│   └── internal/
├── outbox-publisher/     # Outbox pattern publisher
│   ├── cmd/worker/
│   └── internal/
├── shared/               # Shared libraries
│   ├── api/             # HTTP utilities
│   ├── config/          # Configuration
│   ├── domainerror/     # Error handling
│   ├── events/          # Event broker
│   ├── logger/          # Structured logging
│   ├── middleware/      # HTTP middleware
│   ├── notification/    # Email/SMS notifications
│   ├── store/           # Database connections
│   ├── telemetry/       # Observability
│   └── token/           # JWT & password utilities
└── build/
    └── Dockerfile       # Multi-service Dockerfile
```

## Getting Started

### Prerequisites

- [Go 1.26+](https://golang.org/dl/)
- [Docker & Docker Compose](https://docs.docker.com/get-docker/)
- [Task](https://taskfile.dev/) - Task runner (replaces Make)
- [golang-migrate](https://github.com/golang-migrate/migrate) - Database migrations

### Install Development Tools

```bash
task tools:install
```

This installs:
- golangci-lint
- golang-migrate
- swag (Swagger generator)

### Local Development Setup

1. **Clone the repository**
   ```bash
   git clone https://github.com/rasparac/rekreativko-api.git
   cd rekreativko-api
   ```

2. **Start infrastructure services**
   ```bash
   task docker:run:local
   ```
   This starts PostgreSQL, Redis, NATS, Jaeger, Prometheus, and Grafana.

3. **Run migrations**
   ```bash
   task migrate:all:up
   ```

4. **Run a service locally**
   ```bash
   task run:gateway      # API Gateway
   task run:identity     # Identity service
   task run:activity     # Activity service
   ```

5. **Build all services**
   ```bash
   task build
   ```

## Development Commands

### Building

```bash
task build                    # Build all services
task build:gateway            # Build specific service
task build:activity           # Build activity (api + cron)
task build:identity           # Build identity service
task build:account-profile    # Build account-profile service
```

### Testing

```bash
task test                     # Run unit tests
task test:integration         # Run integration tests (requires Docker)
task test:all                 # Run all tests
task test:coverage            # Generate coverage report
task test:verbose             # Run tests with verbose output

# Per-service integration tests
task test:activity:integration
task test:identity:integration
task test:account-profile:integration
```

### Database Migrations

```bash
# Run migrations for all services
task migrate:all:up

# Per-service migrations
task migrate:activity:up
task migrate:identity:up
task migrate:account-profile:up

# Rollback migrations
task migrate:activity:down

# Create new migration
task migrate:create -- activity create_sessions_table
task migrate:create -- identity add_user_roles
```

### Code Quality

```bash
task lint                     # Run linter
task fmt                      # Format code
task check                    # Run fmt + lint + test:all
task clean                    # Clean test cache and containers
```

### Docker

```bash
task docker:up                # Start all services
task docker:down              # Stop all services
task docker:build             # Build all Docker images

# Build specific service images
task docker:build:gateway
task docker:build:activity
task docker:build:identity
```

### Running Services

```bash
task run:gateway              # Run gateway locally
task run:identity             # Run identity service
task run:activity             # Run activity API
task run:activity:cron        # Run activity cron jobs
task run:account-profile      # Run account-profile service
task run:outbox               # Run outbox publisher
```

## Environment Variables

Create a `.env` file in the root directory:

```env
# Database
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=postgres
DB_NAME=rekreativko

# JWT
JWT_SECRET=your-secret-key
JWT_ACCESS_TOKEN_DURATION=15m
JWT_REFRESH_TOKEN_DURATION=7d

# NATS
NATS_URL=nats://localhost:4222

# Redis
REDIS_HOST=localhost:6379

# Telemetry
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
OTEL_TRACES_SAMPLE_RATE=1.0
```

## API Documentation

When running in development mode, Swagger UI is available at:

```
http://localhost:8080/swagger/index.html
```

Generate/update Swagger docs:
```bash
task docs:swagger
```

## Testing

### Unit Tests
Fast tests with no external dependencies:
```bash
task test:unit
```

### Integration Tests
Tests that require Docker (PostgreSQL via testcontainers):
```bash
task test:integration
```

### Coverage Report
```bash
task test:coverage
open coverage.html
```

## CI/CD

GitHub Actions workflow automatically builds and pushes Docker images to GitHub Container Registry on push to `main`.

Manual builds can be triggered via GitHub Actions UI by selecting a service and version.

## Observability

### Metrics
- Prometheus: http://localhost:9090
- Grafana: http://localhost:3000

### Tracing
- Jaeger UI: http://localhost:16686

### Logs
Structured JSON logs with configurable levels (debug, info, warn, error).

## Architecture Patterns

- **Domain-Driven Design (DDD)**: Each service follows DDD with clear domain, application, and infrastructure layers
- **Hexagonal Architecture**: Dependencies point inward toward the domain
- **Event-Driven**: Services communicate via NATS event streams
- **Outbox Pattern**: Reliable event publishing using database outbox
- **CQRS**: Command-Query separation where applicable

## Contributing

1. Create a feature branch from `main`
2. Make your changes
3. Run `task check` to ensure code quality
4. Create a pull request

## License

[Your License Here]
