package persistence

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/store/postgres"
)

// moderationManager stores reports (activity.report) and blocks
// (activity.account_block).
type moderationManager struct {
	tx     *postgres.TransactionManager
	logger *logger.Logger
}

func NewModerationRepository(tx *postgres.TransactionManager, logger *logger.Logger) *moderationManager {
	return &moderationManager{tx: tx, logger: logger}
}

// CreateReport stores the report. It returns false, storing nothing, when the
// reporter already has an open report on that target.
func (m *moderationManager) CreateReport(ctx context.Context, report *domain.Report) (bool, error) {
	var note *string
	if report.Note() != "" {
		n := report.Note()
		note = &n
	}

	tag, err := m.tx.Querier(ctx).Exec(ctx, `
		INSERT INTO activity.report (id, reporter_id, target_type, target_id, reason, note, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (reporter_id, target_type, target_id) WHERE status = 'open' DO NOTHING`,
		report.ID(), report.ReporterID(), string(report.TargetType()), report.TargetID(),
		string(report.Reason()), note, report.CreatedAt(),
	)
	if err != nil {
		return false, err
	}

	return tag.RowsAffected() == 1, nil
}

// Block stores the block; blocking an account again is a no-op.
func (m *moderationManager) Block(ctx context.Context, block *domain.AccountBlock) error {
	_, err := m.tx.Querier(ctx).Exec(ctx, `
		INSERT INTO activity.account_block (blocker_id, blocked_id, created_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (blocker_id, blocked_id) DO NOTHING`,
		block.BlockerID(), block.BlockedID(), block.CreatedAt(),
	)

	return err
}

// Unblock removes the block, if there is one.
func (m *moderationManager) Unblock(ctx context.Context, blockerID, blockedID uuid.UUID) error {
	_, err := m.tx.Querier(ctx).Exec(ctx,
		`DELETE FROM activity.account_block WHERE blocker_id = $1 AND blocked_id = $2`, blockerID, blockedID)

	return err
}

// ListBlocked returns the accounts blockerID has blocked, newest first, at
// most limit.
func (m *moderationManager) ListBlocked(ctx context.Context, blockerID uuid.UUID, limit int) ([]*domain.AccountBlock, error) {
	rows, err := m.tx.Querier(ctx).Query(ctx, `
		SELECT blocked_id, created_at FROM activity.account_block
		WHERE blocker_id = $1
		ORDER BY created_at DESC, blocked_id
		LIMIT $2`, blockerID, limit)
	if err != nil {
		return nil, err
	}

	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (*domain.AccountBlock, error) {
		var (
			blockedID uuid.UUID
			createdAt time.Time
		)
		if err := row.Scan(&blockedID, &createdAt); err != nil {
			return nil, err
		}
		return domain.ReconstructAccountBlock(blockerID, blockedID, createdAt), nil
	})
}

// IsBlockedEitherWay tells whether a has blocked b or b has blocked a.
func (m *moderationManager) IsBlockedEitherWay(ctx context.Context, a, b uuid.UUID) (bool, error) {
	var blocked bool
	err := m.tx.Querier(ctx).QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM activity.account_block
			WHERE (blocker_id = $1 AND blocked_id = $2) OR (blocker_id = $2 AND blocked_id = $1)
		)`, a, b).Scan(&blocked)

	return blocked, err
}

// The conditions below leave out what an account the requester ($n) has
// blocked created, unless the requester already shares it - a group they are
// a confirmed member of, a session they RSVP'd to or whose group they belong
// to. Blocking hides new content; it does not throw anyone out.

// groupNotHiddenByBlock is for queries on activity.activity_group.
func groupNotHiddenByBlock(param int) string {
	return fmt.Sprintf(` AND (
		creator_id NOT IN (SELECT blocked_id FROM activity.account_block WHERE blocker_id = $%[1]d)
		OR id IN (SELECT activity_group_id FROM activity.member WHERE account_id = $%[1]d AND status = 'confirmed' AND deleted_at IS NULL)
	)`, param)
}

// sessionNotHiddenByBlock is for queries on activity.session.
func sessionNotHiddenByBlock(param int) string {
	return fmt.Sprintf(` AND (
		created_by_id NOT IN (SELECT blocked_id FROM activity.account_block WHERE blocker_id = $%[1]d)
		OR id IN (SELECT session_id FROM activity.session_attendee WHERE account_id = $%[1]d AND deleted_at IS NULL)
		OR activity_group_id IN (SELECT activity_group_id FROM activity.member WHERE account_id = $%[1]d AND status = 'confirmed' AND deleted_at IS NULL)
	)`, param)
}

// inviteNotFromBlocked leaves out pending invites (group or session) whose
// inviter the invited user has blocked.
const inviteNotFromBlocked = `invited_by_user_id NOT IN (
	SELECT blocked_id FROM activity.account_block WHERE blocker_id = invited_user_id)`
