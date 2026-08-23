package mapper

import (
	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/http/dtos"
)

// SendInviteRequestToParams converts SendInviteRequest to application params
func SendInviteRequestToParams(
	req *dtos.SendInviteRequest,
	activityGroupID uuid.UUID,
	requesterID uuid.UUID,
	requesterRole string,
) *application.SendInviteParams {
	return &application.SendInviteParams{
		ActivityGroupID: activityGroupID,
		RequesterID:     requesterID,
		RequesterRole:   requesterRole,
		InvitedUserID:   req.UserID,
	}
}

// InviteToResponse converts a domain GroupInvite to InviteResponse
func InviteToResponse(invite *domain.GroupInvite) *dtos.InviteResponse {
	resp := &dtos.InviteResponse{
		ID:              invite.ID(),
		ActivityGroupID: invite.ActivityGroupID(),
		InvitedUserID:   invite.InvitedUserID(),
		InvitedByID:     invite.InvitedByID(),
		Status:          invite.Status().String(),
		CreatedAt:       invite.CreatedAt(),
		ExpiresAt:       invite.ExpiresAt(),
	}

	if respondedAt := invite.RespondedAt(); respondedAt != nil {
		resp.RespondedAt = respondedAt
	}

	return resp
}

// InviteListToResponse converts a list of domain GroupInvites to InviteListResponse
func InviteListToResponse(invites []*domain.GroupInvite) *dtos.InviteListResponse {
	responses := make([]dtos.InviteResponse, len(invites))
	for i, invite := range invites {
		responses[i] = *InviteToResponse(invite)
	}

	return &dtos.InviteListResponse{
		Invites: responses,
	}
}
