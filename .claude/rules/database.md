---
paths:
  - "**/infrastructure/persistence/**"
  - "**/migrations/**"
  - "shared/store/**"
---

# Database

## Schema

- One PostgreSQL schema per service (`identity`, `account_profile`, `activity`, `notifications`, `location`), all
  in one database. No foreign keys across schemas: an account id from identity is just a `uuid` elsewhere.
- **Every schema change goes into `migrations/000001_<service>_schema.up.sql` and its `.down.sql`** (drop in
  reverse order). Never add `000002_...`; the project is not in production.
- Comment each table and each index with what it is for (the query an index serves). Use partial indexes for
  hot filtered queries (`WHERE deleted_at IS NULL`, `WHERE status = 'pending'`).
- No `ON DELETE CASCADE`: delete children explicitly, before their parents, so what a delete removes is visible
  in the repository code.

## Queries

- Go through the transaction manager: `m.tx.Querier(ctx)` joins the caller's transaction if there is one.
- Parameterize everything (`$1`). A dynamic `ORDER BY` column comes from an allowlist map, never from input.
- Jobs that several instances may run at once take rows with `FOR UPDATE SKIP LOCKED`, in batches.
- Idempotent writes: `INSERT ... ON CONFLICT ... DO NOTHING` and check `RowsAffected()`.
- Soft-deleted rows (`deleted_at`) must be filtered in every query that should not see them
  (`rekreativko-api-1uo.3` tracks making that harder to forget).

## Connections

Services reach Postgres through PgBouncer in transaction pooling mode: consecutive transactions of one pgx
connection may run on different server connections. Keep everything a query relies on inside its transaction -
`SET LOCAL`, `pg_advisory_xact_lock`, `FOR UPDATE`. Never use session state: `SET`/`set_config(..., false)`,
`pg_advisory_lock`, `LISTEN`, temp tables, `WITH HOLD` cursors. Something that truly needs a session connects
to Postgres directly (`PostgresConfig.DirectDSN`, as the outbox listener does).

## Outbox

Every schema that raises events has an `event_outbox` table with the same columns, indexes and the
`notify_event_outbox` trigger (copy them from another service). The outbox-publisher only polls schemas listed in
`OUTBOX_SCHEMAS` - add a new one there in every environment (`.env` is not committed).

## Personal data and erasure (GDPR Art. 17)

When identity erases an account, every context deletes that account's data (README "Account erasure" sections
of identity, account-profile, notifications, activity). **A new table or column holding account data must be
added to its context's erasure**, or it is never erased:

| Context | Where the erasure is listed |
|---|---|
| identity | `application.ErasureService.erase` |
| account-profile | `persistence.accountDataTables` |
| notifications | `persistence.AccountStateManager.Erase` |
| activity | `persistence.accountErasureSteps` - `TestAccountErasureSteps_CoverEveryTable` fails until you decide |

Rows where the account only appears as an actor in someone else's record (inviter, voter, past attendee) may keep
its id; rows about the account itself go.
