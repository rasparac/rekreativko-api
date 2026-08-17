package mapper

import (
	"net/url"
	"strconv"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/http/dtos"
)

// CreateRSVPRequestToParams converts CreateRSVPRequest to application params
func CreateRSVPRequestToParams(
	req *dtos.CreateRSVPRequest,
	sessionID uuid.UUID,
	activityGroupID uuid.UUID,
	userID uuid.UUID,
) *application.CreateRSVPParams {
	return &application.CreateRSVPParams{
		SessionID:       sessionID,
		ActivityGroupID: activityGroupID,
		UserID:          userID,
		Status:          req.Status,
	}
}

// UpdateRSVPRequestToParams converts UpdateRSVPRequest to application params
func UpdateRSVPRequestToParams(
	req *dtos.UpdateRSVPRequest,
	sessionID uuid.UUID,
	activityGroupID uuid.UUID,
	userID uuid.UUID,
) *application.UpdateRSVPParams {
	return &application.UpdateRSVPParams{
		SessionID:       sessionID,
		ActivityGroupID: activityGroupID,
		UserID:          userID,
		NewStatus:       req.Status,
	}
}

// AttendeeToResponse converts domain Attendee to AttendeeResponse
func AttendeeToResponse(attendee *domain.Attendee) *dtos.AttendeeResponse {
	return &dtos.AttendeeResponse{
		ID:              attendee.ID(),
		SessionID:       attendee.SessionID(),
		ActivityGroupID: attendee.ActivityID(),
		UserID:          attendee.UserID(),
		Status:          string(attendee.Status()),
		Source:          string(attendee.Source()),
		CreatedAt:       attendee.CreatedAt(),
		UpdatedAt:       attendee.UpdatedAt(),
	}
}

// AttendeeListToResponse converts list of attendees to list response
func AttendeeListToResponse(
	attendees []*domain.Attendee,
	limit int,
	offset int,
) *dtos.AttendeeListResponse {
	responses := make([]dtos.AttendeeResponse, len(attendees))
	for i, attendee := range attendees {
		responses[i] = *AttendeeToResponse(attendee)
	}

	return &dtos.AttendeeListResponse{
		Attendees: responses,
		Total:     len(responses),
		Limit:     limit,
		Offset:    offset,
	}
}

// QueryToListRSVPsParams parses query parameters for listing RSVPs
func QueryToListRSVPsParams(query url.Values) (*application.ListRSVPsParams, error) {
	params := &application.ListRSVPsParams{
		Limit:  20, // default
		Offset: 0,  // default
	}

	// Parse session_id filter
	if sessionIDStr := query.Get("session_id"); sessionIDStr != "" {
		sessionID, err := uuid.Parse(sessionIDStr)
		if err != nil {
			return nil, err
		}
		params.SessionID = &sessionID
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

	// Parse limit
	if limitStr := query.Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			return nil, err
		}
		params.Limit = limit
	}

	// Parse offset
	if offsetStr := query.Get("offset"); offsetStr != "" {
		offset, err := strconv.Atoi(offsetStr)
		if err != nil {
			return nil, err
		}
		params.Offset = offset
	}

	return params, nil
}
