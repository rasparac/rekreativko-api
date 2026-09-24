package mapper

import (
	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/http/dtos"
)

// SendSessionInviteRequestToParams converts SendSessionInviteRequest to application params
func SendSessionInviteRequestToParams(
	req *dtos.SendSessionInviteRequest,
	sessionID uuid.UUID,
	requesterID uuid.UUID,
) *application.SendSessionInviteParams {
	return &application.SendSessionInviteParams{
		SessionID:     sessionID,
		RequesterID:   requesterID,
		InvitedUserID: req.UserID,
	}
}

// SessionInviteToResponse converts a domain SessionInvite to SessionInviteResponse
func SessionInviteToResponse(invite *domain.SessionInvite) *dtos.SessionInviteResponse {
	return &dtos.SessionInviteResponse{
		ID:            invite.ID(),
		SessionID:     invite.SessionID(),
		InvitedUserID: invite.InvitedUserID(),
		InvitedByID:   invite.InvitedByID(),
		Status:        invite.Status().String(),
		CreatedAt:     invite.CreatedAt(),
		ExpiresAt:     invite.ExpiresAt(),
		RespondedAt:   invite.RespondedAt(),
	}
}
