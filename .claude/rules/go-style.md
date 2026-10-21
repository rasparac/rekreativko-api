---
paths:
  - "**/*.go"
---

# Go style

Match the surrounding code first; these are the conventions it follows.

## Errors

- Wrap with what failed: `fmt.Errorf("persist member leave: %w", err)`. Some older code wraps with
  `"failed to ..."`; prefer the short form. Log messages do say it: `log.Error(ctx, "failed to leave group", "error", err)`.
- Business rule violations are sentinel errors in `<service>/internal/domain/errors.go` (`ErrCreatorCannotLeave`).
- Application services turn them into `*domainerror.AppError` in `mapToAppErr` (`domainerror.NotFound`,
  `BadRequest`, `Forbidden`, `Conflict`, `InternalWithErr`); HTTP handlers write them with `handleServiceError`.
  A new sentinel error needs a case there, or it becomes a 500.

## Domain

- Aggregates keep fields unexported. `New...` validates and creates; `Reconstruct...` rebuilds from persistence
  without validation. Value objects (`NewEmail`, `NewTitle`...) validate on construction.
- State changes go through methods that enforce the invariant and raise an event (`addEvent`), never through
  setters from the application layer.

## Application services

- One public method = one use case = one `txManager.WithTransaction`. Write the aggregate's events with
  `eventWriter.InsertEvents(tCtx, schema, aggregate.Events())` inside that transaction, then `ClearEvents()`.
- Depend on small interfaces declared next to the consumer (`accountStates`, `rsvpCanceller`), not on
  concrete repositories, so unit tests can use fakes.
- Each public method opens a span: `ctx, span := s.tracer.Start(ctx, "activity.service.CancelRSVP")`, with
  `s.tracer = telemetry.Tracer(telemetry.Tracer<Service>Service)`; end with `span.SetStatus(codes.Ok, ...)` or
  `codes.Error`. Span attributes are ids, counts and enums only.
- Logger: `logger.WithName("<context>.<component>")` in the constructor, `WithValues("method", ...)` per call.
  Debug for per-event detail, Info for state changes worth seeing in production, Warn/Error for problems.

## Small things

- Sets are `map[K]struct{}` (`m[k] = struct{}{}`, `_, ok := m[k]`), never `map[K]bool`.
- Time is UTC: `time.Now().UTC()`.
- Config is `kelseyhightower/envconfig` in `shared/config`. Every binary loads the whole struct, so a
  `required:"true"` field must be set for every service (`rekreativko-api-uo5`) - prefer a default.
- HTTP handlers carry swagger annotations; regenerate with `task docs:ui`.
