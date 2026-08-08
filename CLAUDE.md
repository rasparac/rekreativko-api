# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Rekreativko API is a Go-based microservices application implementing a modular monolith architecture with domain-driven design (DDD) principles. The system provides identity management, account profiles, and activity tracking capabilities.

## Architecture

### Service Structure

The application is organized as a modular monolith with separate bounded contexts:

- **Gateway Service** (port 8080): API gateway handling routing, authentication, and notification subscriptions
- **Identity Service** (port 8081): User authentication, registration, and account management
- **Account Profile Service** (port 8082): User profiles, settings, and statistics
- **Outbox Publisher**: Background service publishing domain events to NATS message broker

Each service can run independently but shares the same PostgreSQL database.

### Domain-Driven Design

Each bounded context follows DDD layering:

```
internal/{service}/
├── domain/           # Business logic, aggregates, value objects, domain events
├── application/      # Use cases, orchestration, application services
├── infrastructure/   # External concerns (database, persistence)
├── interfaces/       # HTTP handlers, DTOs, event subscribers
└── metrics/          # Service-specific Prometheus metrics
```

**Key patterns:**
- Domain aggregates raise events that are persisted to an outbox table
- Outbox publisher polls for unpublished events and publishes to NATS
- Domain events enable eventual consistency between bounded contexts
- Each aggregate owns its invariants and encapsulates business rules

### Event-Driven Communication

The system uses NATS for asynchronous communication:

1. Domain events are stored in `event_outbox` table during transactions
2. Outbox publisher service polls and publishes events to NATS
3. Services subscribe to events (e.g., gateway subscribes to `identity.account.verified`)
4. Event types follow pattern: `{context}.{aggregate}.{event}`

Examples:
- `identity.account.verified`
- `identity.account.locked`
- `identity.verification_code.created`

### Shared Infrastructure

The `internal/shared/` package contains reusable components:

- `domainevent/`: Base event types and domain event manager
- `events/`: NATS message broker, outbox publisher implementation
- `store/`: PostgreSQL connection pooling, transaction management, Redis client
- `middleware/`: HTTP middleware (auth, CORS, rate limiting, tracing, metrics)
- `token/`: JWT generation, password hashing, verification codes
- `telemetry/`: OpenTelemetry tracing setup
- `logger/`: Structured logging with slog
- `config/`: Configuration loading from environment variables

## Development Commands

### Task Runner

This project uses [Task](https://taskfile.dev) for command execution. View all commands:

```bash
task
```

### Local Development

**Run the application:**
```bash
task run  # Runs cmd/api/main.go
```

**Build:**
```bash
task build  # Outputs to bin/rekreativko
```

**Run tests:**
```bash
task test  # Runs all tests with verbose output
```

**Lint code:**
```bash
task lint  # Runs golangci-lint
```

### Docker Development

**Start all services:**
```bash
task docker:up  # Starts all containers (gateway, identity, account-profile, db, redis, nats, observability)
```

**Start only infrastructure (for local development):**
```bash
task docker:run:local-testing  # Starts db, redis, nats, jaeger, prometheus, grafana
```

**Build services:**
```bash
task docker:build  # Builds all services
task docker:build-by-name -- gateway  # Build specific service
```

**Stop services:**
```bash
task docker:down
```

### Database Migrations

**Run migrations:**
```bash
task migrate:up  # Applies all pending migrations
```

**Rollback last migration:**
```bash
task migrate:down
```

**Create new migration:**
```bash
task migrate:create -- <migration_name>
```

Migrations use [golang-migrate](https://github.com/golang-migrate/migrate) and are located in `migrations/` directory.

### Tools

**Install development tools:**
```bash
task tools:install  # Installs golangci-lint, migrate, goimports
```

**Generate Swagger docs:**
```bash
task docs:swagger  # Generates OpenAPI docs from code annotations
```

Swagger UI is available at `/swagger/index.html` when running in dev mode.

### Dependencies

**Install dependencies:**
```bash
task deps:install
```

**Update dependencies:**
```bash
task deps:update
```

## Testing

**Run all tests:**
```bash
task test
```

**Run specific package tests:**
```bash
go test -v ./internal/identity/domain/...
```

**Run single test:**
```bash
go test -v -run TestName ./path/to/package
```

Tests use `testify/assert` for assertions and `miniredis` for Redis mocking.

## Configuration

Configuration is loaded via environment variables using `kelseyhightower/envconfig`. See `.env` file for all available options.

**Key configuration areas:**
- Service metadata (name, version, environment)
- Server (host, port)
- PostgreSQL connection and pooling
- Redis connection
- JWT tokens (secret, durations)
- NATS messaging
- OpenTelemetry (OTLP endpoint, sampling rates)
- Service URLs for gateway routing

## Gateway Routing

The gateway uses custom routing logic in `internal/gateway/router.go`:

- Routes are defined with path prefixes that map to backend services
- Each route has auth rules using regex patterns
- Public endpoints (login, register) are explicitly allowed
- Authenticated requests have user context headers added (`X-User-ID`, `X-User-Roles`)
- Paths are stripped before proxying to backend services

## Authentication Flow

1. User registers via `/identity/api/v1/register`
2. Verification code is generated and event published
3. User verifies account via `/identity/api/v1/verify-account`
4. User logs in via `/identity/api/v1/login` receiving access + refresh tokens
5. Gateway validates JWT on protected routes using `middleware.RequireAuth`
6. User context is extracted and propagated via headers to backend services

## Observability

**Tracing:** OpenTelemetry traces exported to Jaeger (http://localhost:16686)

**Metrics:** Prometheus metrics exposed on `/metrics` endpoint (http://localhost:9090)

**Dashboards:** Grafana dashboards at http://localhost:3000

**Service metrics include:**
- HTTP request duration and counts
- Database query performance
- Event publishing metrics
- Custom business metrics per service

## Key Technical Decisions

- **Go 1.26** with standard library HTTP server
- **PostgreSQL 17** as primary database (shared across services)
- **NATS JetStream** for reliable event streaming
- **Redis** for caching and rate limiting
- **Transactional outbox pattern** ensures eventual consistency
- **OpenTelemetry** for distributed tracing
- **Prometheus + Grafana** for metrics and visualization

## Adding a New Service

1. Create directory under `internal/{service-name}/` with DDD layers
2. Add main entry point under `cmd/{service-name}/main.go`
3. Create database migration for service schema
4. Register route in gateway router (`internal/gateway/router.go`)
5. Add service to `docker-compose.yaml`
6. Add service-specific metrics in `internal/{service-name}/metrics/`
7. Subscribe to relevant domain events if needed

## Domain Documentation

Detailed domain documentation is located in `.claude/docs/`. These files provide in-depth context about each bounded context:

- **`.claude/docs/arhitecture.md`** - High-level architecture and domain boundaries
- **`.claude/docs/identity_context.md`** - Identity & authentication domain (accounts, tokens, verification)
- **`.claude/docs/account_profile.md`** - User profiles, settings, interests, and statistics
- **`.claude/docs/activity.md`** - Activity groups, sessions, invites, and attendance (CORE domain)

**When to read these:**
- Before working on a specific domain/bounded context
- To understand business rules, invariants, and domain events
- When adding features that cross domain boundaries
- To understand the event-driven architecture and event flows

These files document the actual implementation including aggregates, value objects, database schemas, API contracts, and business rules.
