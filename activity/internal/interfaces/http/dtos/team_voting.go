package dtos

import (
	"time"

	"github.com/google/uuid"
)

// ProposeTeamsRequest proposes a division of everyone going into teams
type ProposeTeamsRequest struct {
	// Teams in order: the first becomes Team A. When the session has teams the
	// count must match them; otherwise the first proposal of a round sets it.
	Teams []ProposalTeamRequest `json:"teams" validate:"required,min=2,max=8,dive"`
}

// ProposalTeamRequest is one team of a proposal
type ProposalTeamRequest struct {
	UserIDs []uuid.UUID `json:"user_ids" validate:"required" example:"123e4567-e89b-12d3-a456-426655440000"`
}

// CastVoteRequest votes (or changes a vote): give exactly one of proposal_id
// or keep_current
type CastVoteRequest struct {
	ProposalID  *uuid.UUID `json:"proposal_id,omitempty" example:"123e4567-e89b-12d3-a456-426655440000"`
	KeepCurrent bool       `json:"keep_current,omitempty" example:"false"`
}

// CloseVotingRequest closes voting. A winner is only needed on a tie: one of
// the tied proposals, or keep_current when it tied.
type CloseVotingRequest struct {
	WinnerProposalID *uuid.UUID `json:"winner_proposal_id,omitempty" example:"123e4567-e89b-12d3-a456-426655440000"`
	KeepCurrent      bool       `json:"keep_current,omitempty" example:"false"`
}

// ProposalTeamResponse is one team of a proposal
type ProposalTeamResponse struct {
	Position int         `json:"position" example:"0"`
	UserIDs  []uuid.UUID `json:"user_ids"`
}

// ProposalResponse is one proposal with its votes
type ProposalResponse struct {
	ID        uuid.UUID              `json:"id" example:"123e4567-e89b-12d3-a456-426655440000"`
	AuthorID  uuid.UUID              `json:"author_id" example:"123e4567-e89b-12d3-a456-426655440000"`
	CreatedAt time.Time              `json:"created_at" example:"2024-02-01T17:40:00Z"`
	Teams     []ProposalTeamResponse `json:"teams"`
	VoteCount int                    `json:"vote_count" example:"3"`
}

// VotingResultResponse is how a closed round ended
type VotingResultResponse struct {
	// WinnerProposalID is the proposal that became the teams; null when
	// "keep current teams" won.
	WinnerProposalID *uuid.UUID `json:"winner_proposal_id" example:"123e4567-e89b-12d3-a456-426655440000"`
	KeptCurrent      bool       `json:"kept_current" example:"false"`
}

// VotingResponse is the proposals screen: the session's latest voting round
// (or none) with proposals, tallies and the caller's vote. Proposals and votes
// only exist while voting is open - they are deleted when it ends.
type VotingResponse struct {
	RoundID      *uuid.UUID `json:"round_id" example:"123e4567-e89b-12d3-a456-426655440000"`
	VotingStatus string     `json:"voting_status" example:"open" enums:"none,open,closed,cancelled"`
	// CancelledReason is set for a cancelled round.
	CancelledReason *string `json:"cancelled_reason" example:"not_enough_players" enums:"not_enough_players,teams_replaced"`
	// TeamCount every proposal in the round must use; null before a round.
	TeamCount         *int `json:"team_count" example:"2"`
	MinPlayersPerTeam *int `json:"min_players_per_team,omitempty" example:"5"`
	PlayersGoing      int  `json:"players_going" example:"9"`
	// PlayersNeeded is how many must be going for voting; null when the
	// session has no teams and no round is open.
	PlayersNeeded *int `json:"players_needed" example:"10"`
	// MyVote is the caller's vote: a proposal id, "keep_current", or null.
	MyVote         *string `json:"my_vote" example:"keep_current"`
	VotesCast      int     `json:"votes_cast" example:"7"`
	EligibleVoters int     `json:"eligible_voters" example:"10"`
	// KeepCurrentVotes is null when "keep current teams" is not an option (the
	// session had no teams when the round opened).
	KeepCurrentVotes *int                  `json:"keep_current_votes" example:"1"`
	Items            []ProposalResponse    `json:"items"`
	Result           *VotingResultResponse `json:"result,omitempty"`
	// Version increases with every change; ignore a snapshot older than one you have.
	Version   int        `json:"version" example:"42"`
	StartedAt *time.Time `json:"started_at,omitempty" example:"2024-02-01T17:40:00Z"`
	EndedAt   *time.Time `json:"ended_at,omitempty" example:"2024-02-01T17:50:00Z"`
}
