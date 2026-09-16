package mapper

import (
	"net/url"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/http/dtos"
	"github.com/rasparac/rekreativko-api/shared/api"
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

// QueryToListMyInvitesParams parses query parameters for listing the caller's invites
func QueryToListMyInvitesParams(query url.Values) (*application.ListMyInvitesParams, error) {
	params := &application.ListMyInvitesParams{
		Limit: 20, // default
	}

	limit, pageToken, err := api.ParsePageParams(query, params.Limit)
	if err != nil {
		return nil, err
	}
	params.Limit = limit
	params.PageToken = pageToken

	return params, nil
}
