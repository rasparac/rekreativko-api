package persistence

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rasparac/rekreativko-api/shared/store/postgres"
)

type (
	// AccountStateManager follows identity's account deletion and erasure in
	// this schema (activity.deleted_account, activity.erased_account).
	AccountStateManager struct {
		tx *postgres.TransactionManager
	}

	// accountErasureStep is one statement of the erasure; $1 is the account.
	accountErasureStep struct {
		name  string
		query string
	}
)

func NewAccountStateManager(tx *postgres.TransactionManager) *AccountStateManager {
	return &AccountStateManager{tx: tx}
}

// MarkDeleted records that the account was deleted. A repeat keeps the first
// deleted_at.
func (m *AccountStateManager) MarkDeleted(ctx context.Context, accountID uuid.UUID, deletedAt time.Time) error {
	_, err := m.tx.Querier(ctx).Exec(ctx, `
	INSERT INTO activity.deleted_account (account_id, deleted_at)
	VALUES ($1, $2)
	ON CONFLICT (account_id) DO NOTHING`, accountID, deletedAt)

	return err
}

// IsInactive reports whether the account was deleted or erased.
func (m *AccountStateManager) IsInactive(ctx context.Context, accountID uuid.UUID) (bool, error) {
	var inactive bool
	err := m.tx.Querier(ctx).QueryRow(ctx, `
	SELECT EXISTS (SELECT 1 FROM activity.deleted_account WHERE account_id = $1)
		OR EXISTS (SELECT 1 FROM activity.erased_account WHERE account_id = $1)`,
		accountID,
	).Scan(&inactive)

	return inactive, err
}

// ListUpcomingSessions returns the scheduled sessions the account has an
// RSVP for (any status, waitlist included).
func (m *AccountStateManager) ListUpcomingSessions(ctx context.Context, accountID uuid.UUID) ([]uuid.UUID, error) {
	return m.ids(ctx, `
	SELECT a.session_id
	FROM activity.session_attendee a
	JOIN activity.session s ON s.id = a.session_id
	WHERE a.account_id = $1 AND a.deleted_at IS NULL
		AND s.status = 'scheduled' AND s.deleted_at IS NULL
	ORDER BY s.start_time`, accountID)
}

// ListPendingGroupInvites returns the group invites sent to the account that
// it can still answer.
func (m *AccountStateManager) ListPendingGroupInvites(ctx context.Context, accountID uuid.UUID) ([]uuid.UUID, error) {
	return m.ids(ctx, `
	SELECT id FROM activity.group_invites
	WHERE invited_user_id = $1 AND status = 'pending' AND expires_at > NOW()`, accountID)
}

// ListPendingSessionInvites returns the session invites sent to the account
// that it can still answer.
func (m *AccountStateManager) ListPendingSessionInvites(ctx context.Context, accountID uuid.UUID) ([]uuid.UUID, error) {
	return m.ids(ctx, `
	SELECT id FROM activity.session_invites
	WHERE invited_user_id = $1 AND status = 'pending' AND expires_at > NOW()`, accountID)
}

// ListGroupsToLeave returns the groups the account is a confirmed member of
// without having created them (a creator cannot leave its group; the group
// goes at the erasure).
func (m *AccountStateManager) ListGroupsToLeave(ctx context.Context, accountID uuid.UUID) ([]uuid.UUID, error) {
	return m.ids(ctx, `
	SELECT activity_group_id FROM activity.member
	WHERE account_id = $1 AND deleted_at IS NULL
		AND status = 'confirmed' AND role <> 'creator'`, accountID)
}

func (m *AccountStateManager) ids(ctx context.Context, query string, accountID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := m.tx.Querier(ctx).Query(ctx, query, accountID)
	if err != nil {
		return nil, err
	}

	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

// What the account created: its groups (creator_id), every template of those
// groups or created by it, and every session of those groups or templates or
// created by it. Each step's subqueries run when the step does; every step
// deletes children before the parents these subqueries read.
const (
	erasedGroups    = `SELECT id FROM activity.activity_group WHERE creator_id = $1`
	erasedTemplates = `SELECT id FROM activity.session_template
		WHERE created_by_id = $1 OR activity_group_id IN (` + erasedGroups + `)`
	erasedSessions = `SELECT id FROM activity.session
		WHERE created_by_id = $1 OR activity_group_id IN (` + erasedGroups + `)
			OR session_template_id IN (` + erasedTemplates + `)`
	erasedRounds = `SELECT id FROM activity.session_team_voting_round WHERE session_id IN (` + erasedSessions + `)`
)

// accountErasureSteps run in this order, children before parents (no ON
// DELETE CASCADE, so what an erasure removes stays visible here):
//
//  1. everything the account created (decision dh7.3, .claude/docs/activity.md);
//  2. the rows about the account in what others created: memberships,
//     invites it received, invite-link uses and RSVPs it withdrew.
//
// Rows where it is only an actor in others' records - inviter, link creator,
// draft captain or pick, proposal author or member, voter, past attendee -
// keep its id: clients show a deleted user, and the id alone identifies nobody
// once identity has erased the account. A new table holding account data
// belongs in this list.
var accountErasureSteps = []accountErasureStep{
	{"team votes", `DELETE FROM activity.session_team_vote WHERE round_id IN (` + erasedRounds + `)`},
	{"team proposal members", `DELETE FROM activity.session_team_proposal_member WHERE proposal_id IN (
		SELECT id FROM activity.session_team_proposal WHERE round_id IN (` + erasedRounds + `))`},
	{"team proposals", `DELETE FROM activity.session_team_proposal WHERE round_id IN (` + erasedRounds + `)`},
	{"team voting rounds", `DELETE FROM activity.session_team_voting_round WHERE session_id IN (` + erasedSessions + `)`},
	{"team draft picks", `DELETE FROM activity.session_team_draft_pick WHERE draft_id IN (
		SELECT id FROM activity.session_team_draft WHERE session_id IN (` + erasedSessions + `))`},
	{"team drafts", `DELETE FROM activity.session_team_draft WHERE session_id IN (` + erasedSessions + `)`},
	{"session invites", `DELETE FROM activity.session_invites WHERE session_id IN (` + erasedSessions + `)`},
	{"session statistics", `DELETE FROM activity.session_statistics WHERE session_id IN (` + erasedSessions + `)`},
	{"session attendees", `DELETE FROM activity.session_attendee WHERE session_id IN (` + erasedSessions + `)`},
	{"session teams", `DELETE FROM activity.session_team WHERE session_id IN (` + erasedSessions + `)`},
	{"sessions", `DELETE FROM activity.session WHERE id IN (` + erasedSessions + `)`},
	{"session templates", `DELETE FROM activity.session_template WHERE id IN (` + erasedTemplates + `)`},
	{"group invite link uses", `DELETE FROM activity.group_invite_link_usage WHERE group_invite_link_id IN (
		SELECT id FROM activity.group_invite_links WHERE activity_group_id IN (` + erasedGroups + `))`},
	{"group invite links", `DELETE FROM activity.group_invite_links WHERE activity_group_id IN (` + erasedGroups + `)`},
	{"group invites", `DELETE FROM activity.group_invites WHERE activity_group_id IN (` + erasedGroups + `)`},
	{"group members", `DELETE FROM activity.member WHERE activity_group_id IN (` + erasedGroups + `)`},
	{"group statistics", `DELETE FROM activity.activity_group_statistics WHERE activity_group_id IN (` + erasedGroups + `)`},
	{"groups", `DELETE FROM activity.activity_group WHERE id IN (` + erasedGroups + `)`},

	{"its memberships", `DELETE FROM activity.member WHERE account_id = $1`},
	{"group invites it received", `DELETE FROM activity.group_invites WHERE invited_user_id = $1`},
	{"session invites it received", `DELETE FROM activity.session_invites WHERE invited_user_id = $1`},
	{"its invite link uses", `DELETE FROM activity.group_invite_link_usage WHERE account_id = $1`},
	{"its withdrawn RSVPs", `DELETE FROM activity.session_attendee WHERE account_id = $1 AND deleted_at IS NOT NULL`},
}

// Erase records the erasure and runs accountErasureSteps. It must run in the
// caller's transaction. It returns false, deleting nothing, if the account was
// already erased: the erased_account row is inserted first, so of two
// concurrent deliveries of the same event the second waits for the first and
// then finds it erased.
func (m *AccountStateManager) Erase(ctx context.Context, accountID uuid.UUID, erasedAt time.Time) (bool, error) {
	tx := m.tx.Querier(ctx)

	tag, err := tx.Exec(ctx, `
	INSERT INTO activity.erased_account (account_id, erased_at)
	VALUES ($1, $2)
	ON CONFLICT (account_id) DO NOTHING`, accountID, erasedAt)
	if err != nil {
		return false, fmt.Errorf("record erasure: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return false, nil
	}

	for _, step := range accountErasureSteps {
		if _, err := tx.Exec(ctx, step.query, accountID); err != nil {
			return false, fmt.Errorf("delete %s: %w", step.name, err)
		}
	}

	return true, nil
}
