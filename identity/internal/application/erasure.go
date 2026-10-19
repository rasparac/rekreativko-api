package application

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/identity/internal/domain"
	"github.com/rasparac/rekreativko-api/identity/internal/metrics"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type (
	erasableAccountStore interface {
		ListDueForErasure(ctx context.Context, now time.Time, limit int) ([]*domain.Account, error)
		UpdateAccount(ctx context.Context, account *domain.Account) error
	}

	// accountDataDeleter deletes the rows an account owns in one table.
	accountDataDeleter interface {
		DeleteAllForAccount(ctx context.Context, accountID uuid.UUID) error
	}

	erasureTracker interface {
		RecordCompleted(ctx context.Context, accountID uuid.UUID, context string, completedAt time.Time) error
		CountIncomplete(ctx context.Context, contexts []string, erasedBefore time.Time) (int64, error)
	}

	transactor interface {
		WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error
	}

	// ErasurePolicy configures the erase job.
	ErasurePolicy struct {
		// Batch is how many accounts are erased per transaction.
		Batch int
		// Contexts must each confirm (<context>.account.erased) that they have
		// erased their data of an erased account.
		Contexts []string
		// IncompleteAfter is how long after the erasure a missing confirmation
		// counts as incomplete.
		IncompleteAfter time.Duration
	}

	// ErasureService erases deleted accounts once their grace period has
	// passed and tracks which contexts have erased their own data since.
	ErasureService struct {
		accounts          erasableAccountStore
		refreshTokens     accountDataDeleter
		verificationCodes accountDataDeleter
		erasures          erasureTracker
		txManager         transactor
		domainWriter      domainevent.EventWriter
		logger            *logger.Logger
		metrics           *metrics.Metrics
		tracer            trace.Tracer
		policy            ErasurePolicy
	}
)

func NewErasureService(
	accounts erasableAccountStore,
	refreshTokens accountDataDeleter,
	verificationCodes accountDataDeleter,
	erasures erasureTracker,
	txManager transactor,
	domainWriter domainevent.EventWriter,
	logger *logger.Logger,
	metrics *metrics.Metrics,
	policy ErasurePolicy,
) (*ErasureService, error) {
	if policy.Batch <= 0 || policy.IncompleteAfter <= 0 {
		return nil, fmt.Errorf("account erasure needs a positive batch and incomplete-after (got %d, %s)", policy.Batch, policy.IncompleteAfter)
	}

	return &ErasureService{
		accounts:          accounts,
		refreshTokens:     refreshTokens,
		verificationCodes: verificationCodes,
		erasures:          erasures,
		txManager:         txManager,
		domainWriter:      domainWriter,
		logger:            logger.WithName("identity.erasure"),
		metrics:           metrics,
		tracer:            telemetry.Tracer(telemetry.TracerIdentityService),
		policy:            policy,
	}, nil
}

// EraseDueAccounts erases one batch of the deleted accounts whose grace period
// has passed by now, and returns how many it erased. Each account's email,
// phone and password are removed, its verification codes and refresh tokens
// deleted and identity.account.erased raised, all in one transaction. Safe to
// run from several instances at once: an account another instance is erasing
// is skipped.
func (s *ErasureService) EraseDueAccounts(ctx context.Context, now time.Time) (int, error) {
	ctx, span := s.tracer.Start(ctx, "identity.erasure.EraseDueAccounts")
	defer span.End()

	var erased int
	err := s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		accounts, err := s.accounts.ListDueForErasure(tCtx, now, s.policy.Batch)
		if err != nil {
			return fmt.Errorf("list accounts due for erasure: %w", err)
		}

		var events []domainevent.Event
		for _, account := range accounts {
			if err := s.erase(tCtx, account); err != nil {
				return fmt.Errorf("erase account %s: %w", account.ID(), err)
			}
			events = append(events, account.Events()...)
		}

		if err := s.domainWriter.InsertEvents(tCtx, schema, events); err != nil {
			return fmt.Errorf("insert events: %w", err)
		}

		for _, account := range accounts {
			account.ClearEvents()
		}

		erased = len(accounts)
		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}

	span.SetAttributes(attribute.Int("erased", erased))
	span.SetStatus(codes.Ok, "due accounts erased")

	return erased, nil
}

func (s *ErasureService) erase(ctx context.Context, account *domain.Account) error {
	if err := account.Erase(); err != nil {
		return err
	}

	if err := s.accounts.UpdateAccount(ctx, account); err != nil {
		return fmt.Errorf("update account: %w", err)
	}

	if err := s.verificationCodes.DeleteAllForAccount(ctx, account.ID()); err != nil {
		return fmt.Errorf("delete verification codes: %w", err)
	}

	if err := s.refreshTokens.DeleteAllForAccount(ctx, account.ID()); err != nil {
		return fmt.Errorf("delete refresh tokens: %w", err)
	}

	return nil
}

// RecordContextErasure records that context has erased its data of the
// account. Idempotent: events are delivered at least once.
func (s *ErasureService) RecordContextErasure(ctx context.Context, accountID uuid.UUID, context string) error {
	return s.erasures.RecordCompleted(ctx, accountID, context, time.Now().UTC())
}

// RunOnce erases every account due by now, batch by batch until one comes back
// short, then refreshes the incomplete-erasures gauge. Failures are logged; the
// next run retries.
func (s *ErasureService) RunOnce(ctx context.Context, now time.Time) {
	var total int
	for ctx.Err() == nil {
		erased, err := s.EraseDueAccounts(ctx, now)
		if err != nil {
			s.logger.Error(ctx, "failed to erase deleted accounts", "error", err)
			break
		}

		total += erased
		if erased < s.policy.Batch {
			break
		}
	}

	if total > 0 {
		s.metrics.AccountsErasedTotal.Add(float64(total))
		s.logger.Info(ctx, "erased deleted accounts", "erased", total)
	}

	incomplete, err := s.erasures.CountIncomplete(ctx, s.policy.Contexts, now.Add(-s.policy.IncompleteAfter))
	if err != nil {
		s.logger.Error(ctx, "failed to count incomplete erasures", "error", err)
		return
	}

	s.metrics.ErasuresIncomplete.Set(float64(incomplete))
	if incomplete > 0 {
		s.logger.Warn(ctx, "account erasures not confirmed by every context",
			"incomplete", incomplete, "contexts", s.policy.Contexts, "after", s.policy.IncompleteAfter)
	}
}

// Run calls RunOnce right away and then every interval, until ctx is done.
func (s *ErasureService) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		s.RunOnce(ctx, time.Now().UTC())

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
