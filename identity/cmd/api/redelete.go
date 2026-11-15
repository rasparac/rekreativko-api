package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/identity/internal/application"
	"github.com/rasparac/rekreativko-api/identity/internal/domain"
	"github.com/rasparac/rekreativko-api/identity/internal/infrastructure/persistence"
	"github.com/rasparac/rekreativko-api/identity/internal/metrics"
	"github.com/rasparac/rekreativko-api/shared/config"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
	"github.com/rasparac/rekreativko-api/shared/logger"
	metricstracer "github.com/rasparac/rekreativko-api/shared/store/metrics_tracer"
	"github.com/rasparac/rekreativko-api/shared/store/postgres"
	"github.com/rasparac/rekreativko-api/shared/token"
)

const redeleteUsage = `usage: service redelete <account_id>[@<deleted_at>] ...

Deletes again accounts whose deletion a backup restore lost (identity README,
"After restoring a backup"), exactly like DELETE /me. deleted_at (RFC 3339) is
when the user deleted the account - deleted_at in the damaged database, or
occurred_at of identity.account.deleted - so the erasure is still due
ACCOUNT_ERASURE_GRACE after it; without it the deletion counts from now.`

// redeletion is one account to delete again, as of deletedAt.
type redeletion struct {
	accountID uuid.UUID
	deletedAt time.Time
}

// parseRedeletions reads the redelete arguments; it accepts all or nothing,
// so a typo doesn't leave half the list done.
func parseRedeletions(args []string, now time.Time) ([]redeletion, error) {
	if len(args) == 0 {
		return nil, errors.New("no account ids given")
	}

	redeletions := make([]redeletion, 0, len(args))
	seen := make(map[uuid.UUID]struct{}, len(args))
	for _, arg := range args {
		rawID, rawDeletedAt, hasTime := strings.Cut(arg, "@")

		accountID, err := uuid.Parse(rawID)
		if err != nil {
			return nil, fmt.Errorf("%q: invalid account id", arg)
		}

		if _, ok := seen[accountID]; ok {
			return nil, fmt.Errorf("%q: account given twice", arg)
		}
		seen[accountID] = struct{}{}

		deletedAt := now
		if hasTime {
			deletedAt, err = time.Parse(time.RFC3339, rawDeletedAt)
			if err != nil {
				return nil, fmt.Errorf("%q: deleted_at must be RFC 3339, like 2026-11-10T14:05:00Z", arg)
			}
			if deletedAt.After(now) {
				return nil, fmt.Errorf("%q: deleted_at is in the future", arg)
			}
		}

		redeletions = append(redeletions, redeletion{accountID: accountID, deletedAt: deletedAt.UTC()})
	}

	return redeletions, nil
}

// accountRedeleter is the one use case redelete needs.
type accountRedeleter interface {
	RedoAccountDeletion(ctx context.Context, accountID uuid.UUID, deletedAt time.Time) error
}

// redeleteAll deletes each account in its own transaction and reports one line
// per account. An account that is no longer active is skipped: it was already
// deleted again, or never got further than the backup shows. It fails if any
// account could not be deleted.
func redeleteAll(ctx context.Context, svc accountRedeleter, redeletions []redeletion, out io.Writer) error {
	var failed int
	for _, r := range redeletions {
		err := svc.RedoAccountDeletion(ctx, r.accountID, r.deletedAt)
		switch {
		case err == nil:
			_, _ = fmt.Fprintf(out, "deleted  %s  as of %s\n", r.accountID, r.deletedAt.Format(time.RFC3339))
		case errors.Is(err, domain.ErrAccountDeleted):
			_, _ = fmt.Fprintf(out, "skipped  %s  not active (already deleted?)\n", r.accountID)
		default:
			failed++
			_, _ = fmt.Fprintf(out, "FAILED   %s  %v\n", r.accountID, err)
		}
	}

	if failed > 0 {
		return fmt.Errorf("%d of %d accounts not deleted", failed, len(redeletions))
	}

	return nil
}

// runRedelete wires only what the deletion needs: the database and the outbox
// (the outbox-publisher relays identity.account.deleted as for DELETE /me).
func runRedelete(ctx context.Context, cfg *config.Config, log *logger.Logger, args []string, out io.Writer) error {
	redeletions, err := parseRedeletions(args, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("%w\n\n%s", err, redeleteUsage)
	}

	grace := cfg.IdentityServiceConfig.ErasureGrace
	if grace < 0 {
		return fmt.Errorf("ACCOUNT_ERASURE_GRACE must not be negative, got %s", grace)
	}

	appMetrics := metrics.New()

	pg, err := postgres.New(ctx, log, cfg.Postgres, metricstracer.New(appMetrics))
	if err != nil {
		return err
	}
	defer pg.Close()

	txManager := postgres.NewTransactionManager(pg.Pool)

	eventWriter, err := domainevent.NewEventWriter(txManager, log, "identity")
	if err != nil {
		return err
	}

	svc := application.NewService(
		persistence.NewAccountManager(pg.Pool, txManager, log),
		persistence.NewVerificationCodeManager(pg.Pool, txManager, log),
		persistence.NewRefreshTokenManager(pg.Pool, txManager, log),
		txManager,
		eventWriter,
		token.NewGenerator([]byte(cfg.JWT.Secret), cfg.JWT.AccessTokenDuration, cfg.JWT.RefreshTokenDuration),
		log,
		token.NewVerificationCodeGenerator(6),
		token.NewPasswordHasher(10),
		appMetrics,
		cfg.Features.PhoneRegistrationEnabled,
		cfg.Features.AccountVerificationRequired,
		grace,
	)

	return redeleteAll(ctx, svc, redeletions, out)
}
