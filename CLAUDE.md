# CLAUDE.md

Rekreativko API: a Go 1.26 modular monolith for recreational sports groups - one repo and one `go.mod`, several
services, each owning its own schema in one PostgreSQL 17 database. Services talk through domain events
(transactional outbox -> NATS JetStream); clients reach them only through the gateway. `README.md` explains the
architecture, request flow, events and setup for humans.

## Services

| Service | Port | Owns | Docs |
|---|---|---|---|
| `gateway` | 8080 | routing, JWT check, rate limiting; no database | `gateway/README.md` |
| `identity` | 8081 | accounts, login, tokens, verification, account erasure | `identity/README.md` |
| `account-profile` | 8082 | profiles, interests, settings | `account-profile/README.md` |
| `notifications` | 8083 | in-app feed and email/SMS delivery | `notifications/README.md` |
| `activity` | 8084 | groups, sessions, attendance, invites, teams (core domain); `cmd/cron` for jobs | `activity/README.md` |
| `location` | 8085 | geocoding (cached); no per-account data | `location/README.md` |
| `outbox-publisher` | - | relays every schema's `event_outbox` to NATS, purges old rows | `outbox-publisher/README.md` |

Read the service's README before changing it - it is the source of truth for that service's behavior.
`.claude/docs/*.md` hold older domain design notes; where they disagree with a README or the code, those win.

## Layout

```
<service>/cmd/<binary>/main.go          wiring only
<service>/internal/domain/              aggregates, value objects, domain events, sentinel errors
<service>/internal/application/         use cases; one transaction per use case
<service>/internal/infrastructure/persistence/   repositories; migrations/000001_<service>_schema.*.sql
<service>/internal/interfaces/http/     handlers, DTOs, mappers
<service>/internal/interfaces/events/   consumers of other contexts' events
<service>/internal/metrics/             Prometheus metrics
shared/                                 config, logger, telemetry, middleware, api, domainevent, events, store, token, testing
```

`interfaces` -> `application` -> `domain`; `infrastructure` implements interfaces declared in `application`. A
service never imports another service's `internal` packages - contexts share only the JSON shape of events.

## Commands

```bash
task                          # list every task
go build ./...
task test                     # unit tests (go test ./... -short)
go test -tags integration ./<service>/...   # integration tests, need Docker (testcontainers)
task lint                     # golangci-lint run ./... - untouched files have known findings
task fmt
task docker:up                # whole stack; task docker:run:local = infrastructure only
task migrate:all:up
task docs:ui                  # regenerate swagger for every service, serve on :8090
```

Before calling work done: `go build ./...`, `go test ./...`, the integration tests of any service whose
persistence layer changed, and `golangci-lint run` on the packages you touched.

## Rules that always apply

- **Schema changes** go into the service's existing `000001_<service>_schema.up.sql` / `.down.sql`. Never add a
  migration file (the project is not in production) - so never use `task migrate:create`.
- **Personal data**: identify people by `account_id` only in logs, spans and logged errors
  (`.claude/rules/observability.md`). A new table holding account data must be covered by that context's
  erasure (`.claude/rules/database.md`).
- **Write code like the code around it**: same naming, idioms and comment density; comments say why.
- **Git**: commit or push only when asked; tickets live in beads (`.claude/rules/git-and-beads.md`).

## Where the rest lives

`.claude/rules/` - loaded automatically; those with `paths:` only while working on matching files:

| File | Loaded for | Covers |
|---|---|---|
| `go-style.md` | `**/*.go` | errors, aggregates, application services, tracing, logging, config |
| `testing.md` | `**/*_test.go` | unit vs integration tests, testutil, fakes, metrics in tests |
| `database.md` | persistence, migrations | schemas, queries, locking, deletes, outbox, GDPR erasure lists |
| `events.md` | event code | naming, outbox, consumers, idempotency, dead-lettering |
| `gateway.md` | `gateway/**` | routing, public paths, `X-User-ID`, streaming |
| `observability.md` | always | personal data, query redaction, retention, tracing an event |
| `git-and-beads.md` | always | tickets, commits, session end |

Also: `<service>/CLAUDE.md` (pointer to the service docs), `.claude/skills/resolve-ticket` (`/resolve-ticket`),
`.claude/agents/` (`ticket-writer`, `architecture-reviewer`).
