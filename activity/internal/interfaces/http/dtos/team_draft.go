package dtos

import (
	"time"

	"github.com/google/uuid"
)

// StartDraftRequest starts a captain draft
type StartDraftRequest struct {
	// CaptainIDs are exactly two people going: Team A's captain (picks first),
	// then Team B's.
	CaptainIDs []uuid.UUID `json:"captain_ids" validate:"required,len=2" example:"123e4567-e89b-12d3-a456-426655440000,223e4567-e89b-12d3-a456-426655440000"`
	// PickOrder is "snake" (A,B,B,A - default) or "alternate" (A,B,A,B).
	PickOrder string `json:"pick_order,omitempty" validate:"omitempty,oneof=snake alternate" example:"snake" enums:"snake,alternate"`
	// MinPlayersPerTeam is the optional minimum per team (captain included).
	// Starting needs at least 2 x this many people going (2 when omitted).
	MinPlayersPerTeam *int `json:"min_players_per_team,omitempty" validate:"omitempty,min=1,max=100" example:"5"`
	// Colors are optional "#RRGGBB" colors for Team A and Team B - omit, or give both.
	Colors []string `json:"colors,omitempty" validate:"omitempty,max=2,dive,hexcolor" example:"#FFFFFF,#000000"`
}

// DraftPickRequest is a captain's pick
type DraftPickRequest struct {
	UserID uuid.UUID `json:"user_id" validate:"required" example:"123e4567-e89b-12d3-a456-426655440000"`
}

// ReplaceDraftCaptainRequest names a new captain for a team whose captain
// left (while the draft is paused)
type ReplaceDraftCaptainRequest struct {
	// TeamPosition is the team without a captain: 0 = Team A, 1 = Team B.
	TeamPosition *int `json:"team_position" validate:"required,min=0,max=1" example:"1"`
	// UserID is the new captain: someone already on that team, or still unpicked.
	UserID uuid.UUID `json:"user_id" validate:"required" example:"123e4567-e89b-12d3-a456-426655440000"`
}

// DraftCaptainResponse is one team's captain
type DraftCaptainResponse struct {
	// UserID is null while the captain left and the draft is paused.
	UserID       *uuid.UUID `json:"user_id" example:"123e4567-e89b-12d3-a456-426655440000"`
	TeamPosition int        `json:"team_position" example:"0"`
}

// DraftTurnResponse is whose turn it is
type DraftTurnResponse struct {
	// PickNumber is the 0-based index of this turn in turn_order.
	PickNumber int `json:"pick_number" example:"5"`
	// TotalPicks is how many picks the draft will have in all; it changes when
	// people join or leave.
	TotalPicks   int `json:"total_picks" example:"8"`
	TeamPosition int `json:"team_position" example:"0"`
	// CaptainUserID is null while that team's captain left (draft paused).
	CaptainUserID *uuid.UUID `json:"captain_user_id" example:"123e4567-e89b-12d3-a456-426655440000"`
}

// DraftTeamResponse is one side of a draft
type DraftTeamResponse struct {
	Position int `json:"position" example:"0"`
	// UserIDs are the captain first (when present), then picks in order.
	UserIDs []uuid.UUID `json:"user_ids"`
}

// DraftPickResponse is one pick made in a draft
type DraftPickResponse struct {
	PickNumber   int       `json:"pick_number" example:"0"`
	TeamPosition int       `json:"team_position" example:"0"`
	UserID       uuid.UUID `json:"user_id" example:"123e4567-e89b-12d3-a456-426655440000"`
	PickedAt     time.Time `json:"picked_at" example:"2024-02-01T17:45:00Z"`
}

// DraftResponse is the full state of a session's captain draft - enough to
// render (or resync) the draft screen. Teams are positions (0 = Team A,
// 1 = Team B): the real teams only exist once the draft completes.
type DraftResponse struct {
	ID        uuid.UUID `json:"id" example:"123e4567-e89b-12d3-a456-426655440000"`
	SessionID uuid.UUID `json:"session_id" example:"123e4567-e89b-12d3-a456-426655440000"`
	Status    string    `json:"status" example:"active" enums:"active,paused,completed,cancelled"`
	// PausedReason is set while paused: "captain_left".
	PausedReason *string `json:"paused_reason" example:"captain_left" enums:"captain_left"`
	// CancelledReason is set for a cancelled draft.
	CancelledReason   *string                `json:"cancelled_reason" example:"organizer" enums:"organizer,not_enough_players"`
	PickOrder         string                 `json:"pick_order" example:"snake" enums:"snake,alternate"`
	MinPlayersPerTeam *int                   `json:"min_players_per_team,omitempty" example:"5"`
	Colors            []string               `json:"colors,omitempty" example:"#FFFFFF,#000000"`
	Captains          []DraftCaptainResponse `json:"captains"`
	// Turn is whose turn it is; null once the draft ended.
	Turn *DraftTurnResponse `json:"turn"`
	// TurnOrder is the team position picking on every turn from the first to
	// the last (total_picks long): done, current (turn.pick_number) and upcoming.
	TurnOrder []int               `json:"turn_order" example:"0,1,1,0"`
	Teams     []DraftTeamResponse `json:"teams"`
	Picks     []DraftPickResponse `json:"picks"`
	// AvailableUserIDs are people going nobody has picked yet; empty once the draft ended.
	AvailableUserIDs []uuid.UUID `json:"available_user_ids"`
	// Version increases with every change; ignore a snapshot older than one you have.
	Version   int        `json:"version" example:"12"`
	StartedBy uuid.UUID  `json:"started_by" example:"123e4567-e89b-12d3-a456-426655440000"`
	StartedAt time.Time  `json:"started_at" example:"2024-02-01T17:40:00Z"`
	EndedAt   *time.Time `json:"ended_at,omitempty" example:"2024-02-01T17:50:00Z"`
}
