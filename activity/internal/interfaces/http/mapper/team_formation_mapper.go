package mapper

import (
	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/http/dtos"
)

// TeamFormationSnapshotToResponse converts a snapshot for one viewer - only
// the voting round's my_vote depends on who is looking.
func TeamFormationSnapshotToResponse(snapshot *application.TeamFormationSnapshot, viewerID uuid.UUID) *dtos.TeamFormationResponse {
	session := SessionWithTeamsToResponse(snapshot.Session, snapshot.TeamMembers)

	resp := &dtos.TeamFormationResponse{
		SessionID:    snapshot.Session.ID(),
		Version:      snapshot.Version,
		TeamConfig:   session.TeamConfig,
		Teams:        session.Teams,
		GoingUserIDs: snapshot.GoingUserIDs,
		Voting:       TeamVotingStateToResponse(snapshot.Voting, viewerID),
	}
	if resp.Teams == nil {
		resp.Teams = []dtos.TeamResponse{}
	}
	if resp.GoingUserIDs == nil {
		resp.GoingUserIDs = []uuid.UUID{}
	}

	if snapshot.Draft != nil {
		resp.Draft = TeamDraftStateToResponse(snapshot.Draft)
	}

	return resp
}
