package mapper

import (
	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/http/dtos"
)

// StartDraftRequestToParams converts StartDraftRequest to application params.
// The request validator guarantees exactly two captain IDs.
func StartDraftRequestToParams(
	req *dtos.StartDraftRequest,
	sessionID uuid.UUID,
	requesterID uuid.UUID,
	requesterRole string,
) *application.StartDraftParams {
	return &application.StartDraftParams{
		SessionID:         sessionID,
		RequesterID:       requesterID,
		RequesterRole:     requesterRole,
		CaptainIDs:        [2]uuid.UUID{req.CaptainIDs[0], req.CaptainIDs[1]},
		PickOrder:         req.PickOrder,
		MinPlayersPerTeam: req.MinPlayersPerTeam,
		Colors:            req.Colors,
	}
}

// ReplaceDraftCaptainRequestToParams converts ReplaceDraftCaptainRequest to
// application params. The request validator guarantees TeamPosition is set.
func ReplaceDraftCaptainRequestToParams(
	req *dtos.ReplaceDraftCaptainRequest,
	sessionID uuid.UUID,
	requesterID uuid.UUID,
	requesterRole string,
) *application.ReplaceDraftCaptainParams {
	return &application.ReplaceDraftCaptainParams{
		SessionID:     sessionID,
		RequesterID:   requesterID,
		RequesterRole: requesterRole,
		TeamPosition:  *req.TeamPosition,
		UserID:        req.UserID,
	}
}

// TeamDraftStateToResponse converts a draft and its available players to a DraftResponse
func TeamDraftStateToResponse(state *application.TeamDraftState) *dtos.DraftResponse {
	draft := state.Draft
	sides := []domain.DraftSide{domain.DraftSideA, domain.DraftSideB}

	resp := &dtos.DraftResponse{
		ID:                draft.ID(),
		SessionID:         draft.SessionID(),
		Status:            string(draft.Status()),
		PausedReason:      optionalString(string(draft.PausedReason())),
		CancelledReason:   optionalString(string(draft.CancelReason())),
		PickOrder:         draft.PickOrder().String(),
		MinPlayersPerTeam: draft.TeamConfig().MinPlayersPerTeam(),
		Colors:            draft.TeamConfig().Colors(),
		Captains:          make([]dtos.DraftCaptainResponse, 0, len(sides)),
		TurnOrder:         []int{},
		Teams:             make([]dtos.DraftTeamResponse, 0, len(sides)),
		Picks:             make([]dtos.DraftPickResponse, 0, len(draft.Picks())),
		AvailableUserIDs:  []uuid.UUID{},
		Version:           draft.Version(),
		StartedBy:         draft.StartedBy(),
		StartedAt:         draft.StartedAt(),
		EndedAt:           draft.EndedAt(),
	}

	if len(state.Available) > 0 {
		resp.AvailableUserIDs = state.Available
	}

	captains := draft.Captains()
	for _, side := range sides {
		resp.Captains = append(resp.Captains, dtos.DraftCaptainResponse{
			UserID:       optionalUUID(captains[side]),
			TeamPosition: int(side),
		})
		resp.Teams = append(resp.Teams, dtos.DraftTeamResponse{
			Position: int(side),
			UserIDs:  draft.Roster(side),
		})
	}

	for _, pick := range draft.Picks() {
		resp.Picks = append(resp.Picks, dtos.DraftPickResponse{
			PickNumber:   pick.PickNumber(),
			TeamPosition: int(pick.Side()),
			UserID:       pick.UserID(),
			PickedAt:     pick.PickedAt(),
		})
	}

	for _, side := range draft.TurnOrder(len(state.Available)) {
		resp.TurnOrder = append(resp.TurnOrder, int(side))
	}

	if draft.IsRunning() {
		side := draft.CurrentSide()
		resp.Turn = &dtos.DraftTurnResponse{
			PickNumber:    draft.Turn(),
			TotalPicks:    draft.TotalPicks(len(state.Available)),
			TeamPosition:  int(side),
			CaptainUserID: draft.CurrentCaptainID(),
		}
	}

	return resp
}

func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func optionalUUID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}
