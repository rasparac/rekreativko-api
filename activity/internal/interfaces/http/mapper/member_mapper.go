package mapper

import (
	"net/url"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/http/dtos"
	"github.com/rasparac/rekreativko-api/shared/api"
)

// InviteMemberRequestToParams converts InviteMemberRequest to application params
func InviteMemberRequestToParams(
	req *dtos.InviteMemberRequest,
	activityGroupID uuid.UUID,
	requesterID uuid.UUID,
	requesterRole string,
) *application.InviteMemberParams {
	return &application.InviteMemberParams{
		ActivityGroupID: activityGroupID,
		RequesterID:     requesterID,
		RequesterRole:   requesterRole,
		UserID:          req.UserID,
	}
}

// RemoveMemberRequestToParams converts request to RemoveMemberParams
func RemoveMemberRequestToParams(
	activityGroupID uuid.UUID,
	userID uuid.UUID,
	requesterID uuid.UUID,
	requesterRole string,
) *application.RemoveMemberParams {
	return &application.RemoveMemberParams{
		ActivityGroupID: activityGroupID,
		RequesterID:     requesterID,
		RequesterRole:   requesterRole,
		UserID:          userID,
	}
}

// UpdateMemberRoleRequestToParams converts UpdateMemberRoleRequest to application params
func UpdateMemberRoleRequestToParams(
	req *dtos.UpdateMemberRoleRequest,
	activityGroupID uuid.UUID,
	userID uuid.UUID,
	requesterID uuid.UUID,
	requesterRole string,
) *application.UpdateMemberRoleParams {
	return &application.UpdateMemberRoleParams{
		ActivityGroupID: activityGroupID,
		RequesterID:     requesterID,
		RequesterRole:   requesterRole,
		UserID:          userID,
		NewRole:         req.Role,
	}
}

// ApproveMemberRequestToParams converts request to ApproveMemberParams
func ApproveMemberRequestToParams(
	activityGroupID uuid.UUID,
	userID uuid.UUID,
	requesterID uuid.UUID,
	requesterRole string,
) *application.ApproveMemberParams {
	return &application.ApproveMemberParams{
		ActivityGroupID: activityGroupID,
		RequesterID:     requesterID,
		RequesterRole:   requesterRole,
		UserID:          userID,
	}
}

// RejectMemberRequestToParams converts request to RejectMemberParams
func RejectMemberRequestToParams(
	activityGroupID uuid.UUID,
	userID uuid.UUID,
	requesterID uuid.UUID,
	requesterRole string,
) *application.RejectMemberParams {
	return &application.RejectMemberParams{
		ActivityGroupID: activityGroupID,
		RequesterID:     requesterID,
		RequesterRole:   requesterRole,
		UserID:          userID,
	}
}

// MemberToResponse converts domain Member to MemberResponse
func MemberToResponse(member *domain.Member) *dtos.MemberResponse {
	return &dtos.MemberResponse{
		ID:              member.ID(),
		ActivityGroupID: member.ActivityGroupID(),
		UserID:          member.UserID(),
		Role:            member.Role().String(),
		Status:          member.Status().String(),
		JoinedAt:        member.JoinedAt(),
		DecidedAt:       member.DecidedAt(),
	}
}

// QueryToListMembersParams parses query parameters for listing members
func QueryToListMembersParams(query url.Values) (*application.ListMembersParams, error) {
	params := &application.ListMembersParams{
		Limit: 20, // default
	}

	// Parse activity_group_id filter
	if groupIDStr := query.Get("activity_group_id"); groupIDStr != "" {
		groupID, err := uuid.Parse(groupIDStr)
		if err != nil {
			return nil, err
		}
		params.ActivityGroupID = &groupID
	}

	// Parse user_id filter
	if userIDStr := query.Get("user_id"); userIDStr != "" {
		userID, err := uuid.Parse(userIDStr)
		if err != nil {
			return nil, err
		}
		params.UserID = &userID
	}

	// Parse status filter
	if status := query.Get("status"); status != "" {
		params.Status = &status
	}

	// Parse role filter
	if role := query.Get("role"); role != "" {
		params.Role = &role
	}

	limit, pageToken, err := api.ParsePageParams(query, params.Limit)
	if err != nil {
		return nil, err
	}
	params.Limit = limit
	params.PageToken = pageToken

	return params, nil
}
