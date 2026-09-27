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

// TeamVotingRepository defines the interface for team voting persistence
type TeamVotingRepository interface {
	CreateRound(ctx context.Context, round *domain.TeamVotingRound) error
	UpdateRound(ctx context.Context, round *domain.TeamVotingRound) error
	GetOpenRound(ctx context.Context, sessionID uuid.UUID) (*domain.TeamVotingRound, error)
	GetLatestRound(ctx context.Context, sessionID uuid.UUID) (*domain.TeamVotingRound, error)
}

type teamVotingManager struct {
	tx     *postgres.TransactionManager
	logger *logger.Logger
}

// NewTeamVotingRepository creates a new team voting repository
func NewTeamVotingRepository(
	tx *postgres.TransactionManager,
	logger *logger.Logger,
) TeamVotingRepository {
	return &teamVotingManager{
		tx:     tx,
		logger: logger,
	}
}

func (m *teamVotingManager) CreateRound(ctx context.Context, round *domain.TeamVotingRound) error {
	_, err := m.tx.Querier(ctx).Exec(
		ctx,
		`INSERT INTO activity.session_team_voting_round (
			id, session_id, status, cancel_reason, team_count, min_players_per_team,
			keep_current_allowed, winner_proposal_id, kept_current, version, started_at, ended_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		round.ID(),
		round.SessionID(),
		string(round.Status()),
		nullString(string(round.CancelReason())),
		round.TeamCount(),
		nullInt32(round.MinPlayersPerTeam()),
		round.KeepCurrentAllowed(),
		round.WinnerProposalID(),
		round.KeptCurrent(),
		round.Version(),
		round.StartedAt(),
		round.EndedAt(),
	)
	if err != nil {
		return fmt.Errorf("failed to create team voting round: %w", err)
	}

	return m.insertContent(ctx, round)
}

// UpdateRound saves the round's state and rewrites its proposals and votes
// (a round has at most a few dozen). An ended round has none, so this is also
// what deletes them (D16).
func (m *teamVotingManager) UpdateRound(ctx context.Context, round *domain.TeamVotingRound) error {
	q := m.tx.Querier(ctx)

	result, err := q.Exec(
		ctx,
		`UPDATE activity.session_team_voting_round
		SET status = $2, cancel_reason = $3, winner_proposal_id = $4, kept_current = $5,
			version = $6, ended_at = $7
		WHERE id = $1`,
		round.ID(),
		string(round.Status()),
		nullString(string(round.CancelReason())),
		round.WinnerProposalID(),
		round.KeptCurrent(),
		round.Version(),
		round.EndedAt(),
	)
	if err != nil {
		return fmt.Errorf("failed to update team voting round: %w", err)
	}

	if result.RowsAffected() == 0 {
		return domain.ErrVotingNotOpen
	}

	for _, query := range []string{
		`DELETE FROM activity.session_team_vote WHERE round_id = $1`,
		`DELETE FROM activity.session_team_proposal_member
		WHERE proposal_id IN (SELECT id FROM activity.session_team_proposal WHERE round_id = $1)`,
		`DELETE FROM activity.session_team_proposal WHERE round_id = $1`,
	} {
		if _, err := q.Exec(ctx, query, round.ID()); err != nil {
			return fmt.Errorf("failed to clear team voting round content: %w", err)
		}
	}

	return m.insertContent(ctx, round)
}

func (m *teamVotingManager) insertContent(ctx context.Context, round *domain.TeamVotingRound) error {
	q := m.tx.Querier(ctx)

	for _, p := range round.Proposals() {
		_, err := q.Exec(
			ctx,
			`INSERT INTO activity.session_team_proposal (id, round_id, author_id, created_at)
			VALUES ($1, $2, $3, $4)`,
			p.ID(), round.ID(), p.AuthorID(), p.CreatedAt(),
		)
		if err != nil {
			return fmt.Errorf("failed to create team proposal: %w", err)
		}

		for position, team := range p.Teams() {
			for order, userID := range team {
				_, err := q.Exec(
					ctx,
					`INSERT INTO activity.session_team_proposal_member (proposal_id, team_position, account_id, sort_order)
					VALUES ($1, $2, $3, $4)`,
					p.ID(), position, userID, order,
				)
				if err != nil {
					return fmt.Errorf("failed to create team proposal member: %w", err)
				}
			}
		}
	}

	for voterID, choice := range round.Votes() {
		var proposalID uuid.NullUUID
		if choice != domain.KeepCurrentTeams {
			proposalID = uuid.NullUUID{UUID: choice, Valid: true}
		}

		_, err := q.Exec(
			ctx,
			`INSERT INTO activity.session_team_vote (round_id, voter_id, proposal_id) VALUES ($1, $2, $3)`,
			round.ID(), voterID, proposalID,
		)
		if err != nil {
			return fmt.Errorf("failed to create team vote: %w", err)
		}
	}

	return nil
}

// GetOpenRound returns the session's open round, or domain.ErrVotingNotOpen
// when there is none.
func (m *teamVotingManager) GetOpenRound(ctx context.Context, sessionID uuid.UUID) (*domain.TeamVotingRound, error) {
	return m.getRound(ctx, `WHERE r.session_id = $1 AND r.status = 'open'`, sessionID)
}

// GetLatestRound returns the session's most recent round in any status, or
// domain.ErrVotingNotOpen when the session never had one.
func (m *teamVotingManager) GetLatestRound(ctx context.Context, sessionID uuid.UUID) (*domain.TeamVotingRound, error) {
	return m.getRound(ctx, `WHERE r.session_id = $1 ORDER BY r.started_at DESC LIMIT 1`, sessionID)
}

func (m *teamVotingManager) getRound(ctx context.Context, where string, sessionID uuid.UUID) (*domain.TeamVotingRound, error) {
	query := `
		SELECT
			r.id, r.session_id, s.activity_group_id, r.status, r.cancel_reason, r.team_count,
			r.min_players_per_team, r.keep_current_allowed, r.winner_proposal_id, r.kept_current,
			r.version, r.started_at, r.ended_at
		FROM activity.session_team_voting_round r
		JOIN activity.session s ON s.id = r.session_id
	` + where

	var (
		id, roundSessionID              uuid.UUID
		activityGroupID, winnerProposal uuid.NullUUID
		status                          string
		cancelReason                    sql.NullString
		teamCount, version              int
		minPlayersPerTeam               sql.NullInt32
		keepCurrentAllowed, keptCurrent bool
		startedAt                       time.Time
		endedAt                         *time.Time
	)

	err := m.tx.Querier(ctx).QueryRow(ctx, query, sessionID).Scan(
		&id, &roundSessionID, &activityGroupID, &status, &cancelReason, &teamCount,
		&minPlayersPerTeam, &keepCurrentAllowed, &winnerProposal, &keptCurrent,
		&version, &startedAt, &endedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrVotingNotOpen
		}
		return nil, fmt.Errorf("failed to get team voting round: %w", err)
	}

	proposals, err := m.listProposals(ctx, id, teamCount)
	if err != nil {
		return nil, err
	}

	votes, err := m.listVotes(ctx, id)
	if err != nil {
		return nil, err
	}

	var groupID, winnerID *uuid.UUID
	if activityGroupID.Valid {
		groupID = &activityGroupID.UUID
	}
	if winnerProposal.Valid {
		winnerID = &winnerProposal.UUID
	}

	var minPlayers *int
	if minPlayersPerTeam.Valid {
		v := int(minPlayersPerTeam.Int32)
		minPlayers = &v
	}

	return domain.ReconstructTeamVotingRound(
		id,
		roundSessionID,
		groupID,
		domain.VotingStatus(status),
		domain.VotingCancelReason(cancelReason.String),
		teamCount,
		minPlayers,
		keepCurrentAllowed,
		proposals,
		votes,
		winnerID,
		keptCurrent,
		version,
		startedAt,
		endedAt,
	), nil
}

func (m *teamVotingManager) listProposals(ctx context.Context, roundID uuid.UUID, teamCount int) ([]*domain.TeamProposal, error) {
	rows, err := m.tx.Querier(ctx).Query(
		ctx,
		`SELECT p.id, p.author_id, p.created_at, pm.team_position, pm.account_id
		FROM activity.session_team_proposal p
		LEFT JOIN activity.session_team_proposal_member pm ON pm.proposal_id = p.id
		WHERE p.round_id = $1
		ORDER BY p.created_at ASC, p.id ASC, pm.team_position ASC, pm.sort_order ASC`,
		roundID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list team proposals: %w", err)
	}
	defer rows.Close()

	type proposalRow struct {
		authorID  uuid.UUID
		createdAt time.Time
		teams     map[int][]uuid.UUID
	}

	var (
		order []uuid.UUID
		byID  = map[uuid.UUID]*proposalRow{}
	)
	for rows.Next() {
		var (
			proposalID, authorID uuid.UUID
			createdAt            time.Time
			position             sql.NullInt32
			userID               uuid.NullUUID
		)
		if err := rows.Scan(&proposalID, &authorID, &createdAt, &position, &userID); err != nil {
			return nil, fmt.Errorf("failed to scan team proposal: %w", err)
		}

		p, ok := byID[proposalID]
		if !ok {
			p = &proposalRow{authorID: authorID, createdAt: createdAt, teams: map[int][]uuid.UUID{}}
			byID[proposalID] = p
			order = append(order, proposalID)
		}

		if position.Valid && userID.Valid {
			pos := int(position.Int32)
			p.teams[pos] = append(p.teams[pos], userID.UUID)
		}
	}

	if rows.Err() != nil {
		return nil, fmt.Errorf("error iterating team proposals: %w", rows.Err())
	}

	proposals := make([]*domain.TeamProposal, 0, len(order))
	for _, proposalID := range order {
		p := byID[proposalID]
		// A team emptied by leavers has no member rows; the round's team
		// count keeps it in the proposal.
		teams := make([][]uuid.UUID, teamCount)
		for pos := range teams {
			teams[pos] = p.teams[pos]
			if teams[pos] == nil {
				teams[pos] = []uuid.UUID{}
			}
		}
		proposals = append(proposals, domain.ReconstructTeamProposal(proposalID, p.authorID, p.createdAt, teams))
	}

	return proposals, nil
}

func (m *teamVotingManager) listVotes(ctx context.Context, roundID uuid.UUID) (map[uuid.UUID]uuid.UUID, error) {
	rows, err := m.tx.Querier(ctx).Query(
		ctx,
		`SELECT voter_id, proposal_id FROM activity.session_team_vote WHERE round_id = $1`,
		roundID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list team votes: %w", err)
	}
	defer rows.Close()

	votes := map[uuid.UUID]uuid.UUID{}
	for rows.Next() {
		var (
			voterID    uuid.UUID
			proposalID uuid.NullUUID
		)
		if err := rows.Scan(&voterID, &proposalID); err != nil {
			return nil, fmt.Errorf("failed to scan team vote: %w", err)
		}
		// NULL proposal = keep current teams (uuid.Nil)
		votes[voterID] = proposalID.UUID
	}

	if rows.Err() != nil {
		return nil, fmt.Errorf("error iterating team votes: %w", rows.Err())
	}

	return votes, nil
}

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
