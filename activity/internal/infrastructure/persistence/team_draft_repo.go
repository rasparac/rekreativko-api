package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/store/postgres"
)

// TeamDraftRepository defines the interface for captain draft persistence
type TeamDraftRepository interface {
	CreateDraft(ctx context.Context, draft *domain.TeamDraft) error
	UpdateDraft(ctx context.Context, draft *domain.TeamDraft) error
	GetActiveDraft(ctx context.Context, sessionID uuid.UUID) (*domain.TeamDraft, error)
	GetLatestDraft(ctx context.Context, sessionID uuid.UUID) (*domain.TeamDraft, error)
}

type teamDraftManager struct {
	tx     *postgres.TransactionManager
	logger *logger.Logger
}

// NewTeamDraftRepository creates a new captain draft repository
func NewTeamDraftRepository(
	tx *postgres.TransactionManager,
	logger *logger.Logger,
) TeamDraftRepository {
	return &teamDraftManager{
		tx:     tx,
		logger: logger,
	}
}

func (m *teamDraftManager) CreateDraft(ctx context.Context, draft *domain.TeamDraft) error {
	query := `
		INSERT INTO activity.session_team_draft (
			id, session_id, pick_order, status, captain_a_id, captain_b_id,
			min_players_per_team, team_colors, turn, started_by, started_at, ended_at, cancel_reason, version,
			paused_reason
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`

	captains := draft.Captains()
	config := draft.TeamConfig()

	_, err := m.tx.Querier(ctx).Exec(
		ctx,
		query,
		draft.ID(),
		draft.SessionID(),
		draft.PickOrder().String(),
		string(draft.Status()),
		nullCaptain(captains[domain.DraftSideA]),
		nullCaptain(captains[domain.DraftSideB]),
		nullInt32(config.MinPlayersPerTeam()),
		nullColors(config.Colors()),
		draft.Turn(),
		draft.StartedBy(),
		draft.StartedAt(),
		draft.EndedAt(),
		nullCancelReason(draft.CancelReason()),
		draft.Version(),
		nullPauseReason(draft.PausedReason()),
	)
	if err != nil {
		return fmt.Errorf("failed to create team draft: %w", err)
	}

	return m.insertPicks(ctx, draft)
}

// UpdateDraft saves the draft's state and its current picks (picks are
// rewritten - a draft has at most a few dozen).
func (m *teamDraftManager) UpdateDraft(ctx context.Context, draft *domain.TeamDraft) error {
	q := m.tx.Querier(ctx)

	result, err := q.Exec(
		ctx,
		`UPDATE activity.session_team_draft
		SET status = $2, turn = $3, ended_at = $4, cancel_reason = $5, version = $6,
			paused_reason = $7, captain_a_id = $8, captain_b_id = $9
		WHERE id = $1`,
		draft.ID(),
		string(draft.Status()),
		draft.Turn(),
		draft.EndedAt(),
		nullCancelReason(draft.CancelReason()),
		draft.Version(),
		nullPauseReason(draft.PausedReason()),
		nullCaptain(draft.Captains()[domain.DraftSideA]),
		nullCaptain(draft.Captains()[domain.DraftSideB]),
	)
	if err != nil {
		return fmt.Errorf("failed to update team draft: %w", err)
	}

	if result.RowsAffected() == 0 {
		return domain.ErrDraftNotFound
	}

	if _, err := q.Exec(ctx, `DELETE FROM activity.session_team_draft_pick WHERE draft_id = $1`, draft.ID()); err != nil {
		return fmt.Errorf("failed to clear team draft picks: %w", err)
	}

	return m.insertPicks(ctx, draft)
}

func (m *teamDraftManager) insertPicks(ctx context.Context, draft *domain.TeamDraft) error {
	for _, pick := range draft.Picks() {
		_, err := m.tx.Querier(ctx).Exec(
			ctx,
			`INSERT INTO activity.session_team_draft_pick (draft_id, pick_number, side, account_id, picked_at)
			VALUES ($1, $2, $3, $4, $5)`,
			draft.ID(),
			pick.PickNumber(),
			int(pick.Side()),
			pick.UserID(),
			pick.PickedAt(),
		)
		if err != nil {
			return fmt.Errorf("failed to create team draft pick: %w", err)
		}
	}

	return nil
}

// GetActiveDraft returns the session's running (active or paused) draft, or
// domain.ErrDraftNotFound when there is none.
func (m *teamDraftManager) GetActiveDraft(ctx context.Context, sessionID uuid.UUID) (*domain.TeamDraft, error) {
	return m.getDraft(ctx, `WHERE d.session_id = $1 AND d.status IN ('active', 'paused')`, sessionID)
}

// GetLatestDraft returns the session's most recent draft in any status, or
// domain.ErrDraftNotFound when the session never had one.
func (m *teamDraftManager) GetLatestDraft(ctx context.Context, sessionID uuid.UUID) (*domain.TeamDraft, error) {
	return m.getDraft(ctx, `WHERE d.session_id = $1 ORDER BY d.started_at DESC LIMIT 1`, sessionID)
}

func (m *teamDraftManager) getDraft(ctx context.Context, where string, sessionID uuid.UUID) (*domain.TeamDraft, error) {
	query := `
		SELECT
			d.id, d.session_id, s.activity_group_id, d.pick_order, d.status, d.paused_reason,
			d.captain_a_id, d.captain_b_id, d.min_players_per_team, d.team_colors,
			d.turn, d.version, d.started_by, d.started_at, d.ended_at, d.cancel_reason
		FROM activity.session_team_draft d
		JOIN activity.session s ON s.id = d.session_id
	` + where

	var (
		id, draftSessionID, startedBy       uuid.UUID
		activityGroupID, captainA, captainB uuid.NullUUID
		pickOrder, status                   string
		playersPerTeam                      sql.NullInt32
		colors                              []string
		turn, version                       int
		startedAt                           time.Time
		endedAt                             *time.Time
		cancelReason, pausedReason          sql.NullString
	)

	err := m.tx.Querier(ctx).QueryRow(ctx, query, sessionID).Scan(
		&id, &draftSessionID, &activityGroupID, &pickOrder, &status, &pausedReason,
		&captainA, &captainB, &playersPerTeam, &colors,
		&turn, &version, &startedBy, &startedAt, &endedAt, &cancelReason,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrDraftNotFound
		}
		return nil, fmt.Errorf("failed to get team draft: %w", err)
	}

	picks, err := m.listPicks(ctx, id)
	if err != nil {
		return nil, err
	}

	var ppt *int
	if playersPerTeam.Valid {
		v := int(playersPerTeam.Int32)
		ppt = &v
	}

	var groupID *uuid.UUID
	if activityGroupID.Valid {
		groupID = &activityGroupID.UUID
	}

	return domain.ReconstructTeamDraft(
		id,
		draftSessionID,
		groupID,
		domain.PickOrder(pickOrder),
		domain.DraftStatus(status),
		domain.DraftPauseReason(pausedReason.String),
		domain.DraftCancelReason(cancelReason.String),
		[2]uuid.UUID{captainA.UUID, captainB.UUID}, // NULL = vacant = uuid.Nil
		domain.ReconstructTeamConfig(2, ppt, colors),
		turn,
		version,
		picks,
		startedBy,
		startedAt,
		endedAt,
	), nil
}

func (m *teamDraftManager) listPicks(ctx context.Context, draftID uuid.UUID) ([]domain.DraftPick, error) {
	rows, err := m.tx.Querier(ctx).Query(
		ctx,
		`SELECT pick_number, side, account_id, picked_at
		FROM activity.session_team_draft_pick
		WHERE draft_id = $1
		ORDER BY pick_number ASC`,
		draftID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list team draft picks: %w", err)
	}
	defer rows.Close()

	var picks []domain.DraftPick
	for rows.Next() {
		var (
			pickNumber, side int
			userID           uuid.UUID
			pickedAt         time.Time
		)
		if err := rows.Scan(&pickNumber, &side, &userID, &pickedAt); err != nil {
			return nil, fmt.Errorf("failed to scan team draft pick: %w", err)
		}
		picks = append(picks, domain.ReconstructDraftPick(pickNumber, domain.DraftSide(side), userID, pickedAt))
	}

	if rows.Err() != nil {
		return nil, fmt.Errorf("error iterating team draft picks: %w", rows.Err())
	}

	return picks, nil
}

func nullInt32(v *int) sql.NullInt32 {
	if v == nil {
		return sql.NullInt32{}
	}
	return sql.NullInt32{Int32: int32(*v), Valid: true}
}

// nullColors stores "no colors" as NULL rather than an empty array.
func nullColors(colors []string) []string {
	if len(colors) == 0 {
		return nil
	}
	return colors
}

// nullCaptain stores a vacant captain (uuid.Nil, while paused) as NULL.
func nullCaptain(id uuid.UUID) uuid.NullUUID {
	return uuid.NullUUID{UUID: id, Valid: id != uuid.Nil}
}

func nullPauseReason(reason domain.DraftPauseReason) sql.NullString {
	if reason == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: string(reason), Valid: true}
}

func nullCancelReason(reason domain.DraftCancelReason) sql.NullString {
	if reason == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: string(reason), Valid: true}
}
