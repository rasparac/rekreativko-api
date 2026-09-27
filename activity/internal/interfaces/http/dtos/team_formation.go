package dtos

import "github.com/google/uuid"

// TeamFormationResponse is a session's whole team-formation state: its teams
// with members, the latest captain draft and the latest voting round. GET
// /sessions/{id}/team-formation returns it and every event on the session's
// stream (GET /sessions/{id}/events) carries it, so a screen renders from one
// document either way.
type TeamFormationResponse struct {
	SessionID uuid.UUID `json:"session_id" example:"123e4567-e89b-12d3-a456-426655440000"`
	// Version orders snapshots of the same session: a higher version is newer
	// state. Ignore a snapshot with a lower version than one you already have.
	Version int64 `json:"version" example:"1790000000000000"`
	// TeamConfig is null until the session is split into teams.
	TeamConfig *TeamConfigResponse `json:"team_config"`
	// Teams with their current members, in position order; empty without teams.
	Teams []TeamResponse `json:"teams"`
	// Draft is the latest captain draft (any status), null if there never was one.
	Draft *DraftResponse `json:"draft"`
	// Voting is the latest voting round; voting_status "none" when there never was one.
	Voting *VotingResponse `json:"voting"`
}
