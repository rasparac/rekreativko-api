package mapper

import (
	"net/url"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/http/dtos"
)

// CreateRSVPRequestToParams converts CreateRSVPRequest to application params
func CreateRSVPRequestToParams(
	req *dtos.CreateRSVPRequest,
	sessionID uuid.UUID,
	userID uuid.UUID,
) *application.CreateRSVPParams {
	return &application.CreateRSVPParams{
		SessionID: sessionID,
		UserID:    userID,
		Status:    req.Status,
	}
}

// UpdateRSVPRequestToParams converts UpdateRSVPRequest to application params
func UpdateRSVPRequestToParams(
	req *dtos.UpdateRSVPRequest,
	sessionID uuid.UUID,
	userID uuid.UUID,
) *application.UpdateRSVPParams {
	return &application.UpdateRSVPParams{
		SessionID: sessionID,
		UserID:    userID,
		NewStatus: req.Status,
	}
}

// ApproveAttendeeRequestToParams builds params for approving a pending join request
func ApproveAttendeeRequestToParams(
	sessionID uuid.UUID,
	userID uuid.UUID,
	requesterID uuid.UUID,
	requesterRole string,
) *application.ApproveAttendeeParams {
	return &application.ApproveAttendeeParams{
		SessionID:     sessionID,
		UserID:        userID,
		RequesterID:   requesterID,
		RequesterRole: requesterRole,
	}
}

// AssignTeamRequestToParams builds params for assigning an attendee to a team
func AssignTeamRequestToParams(
	req *dtos.AssignTeamRequest,
	sessionID uuid.UUID,
	userID uuid.UUID,
	requesterID uuid.UUID,
	requesterRole string,
) *application.AssignAttendeeTeamParams {
	return &application.AssignAttendeeTeamParams{
		SessionID:     sessionID,
		UserID:        userID,
		TeamID:        req.TeamID,
		RequesterID:   requesterID,
		RequesterRole: requesterRole,
	}
}

// UnassignTeamRequestToParams builds params for taking an attendee off their team
func UnassignTeamRequestToParams(
	sessionID uuid.UUID,
	userID uuid.UUID,
	requesterID uuid.UUID,
	requesterRole string,
) *application.UnassignAttendeeTeamParams {
	return &application.UnassignAttendeeTeamParams{
		SessionID:     sessionID,
		UserID:        userID,
		RequesterID:   requesterID,
		RequesterRole: requesterRole,
	}
}

// RejectAttendeeRequestToParams builds params for rejecting a pending join request
func RejectAttendeeRequestToParams(
	sessionID uuid.UUID,
	userID uuid.UUID,
	requesterID uuid.UUID,
	requesterRole string,
) *application.RejectAttendeeParams {
	return &application.RejectAttendeeParams{
		SessionID:     sessionID,
		UserID:        userID,
		RequesterID:   requesterID,
		RequesterRole: requesterRole,
	}
}

// RemoveAttendeeRequestToParams builds params for removing an already-confirmed attendee
func RemoveAttendeeRequestToParams(
	sessionID uuid.UUID,
	userID uuid.UUID,
	requesterID uuid.UUID,
	requesterRole string,
) *application.RemoveAttendeeParams {
	return &application.RemoveAttendeeParams{
		SessionID:     sessionID,
		UserID:        userID,
		RequesterID:   requesterID,
		RequesterRole: requesterRole,
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
		TeamID:          attendee.TeamID(),
		CreatedAt:       attendee.CreatedAt(),
		UpdatedAt:       attendee.UpdatedAt(),
	}
}

// QueryToListRSVPsParams parses query parameters for listing RSVPs
func QueryToListRSVPsParams(query url.Values) (*application.ListRSVPsParams, error) {
	params := &application.ListRSVPsParams{}

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

	// Parse status filter - repeatable, e.g. ?status=going&status=pending;
	// the service validates the values
	for _, status := range query["status"] {
		if status != "" {
			params.Statuses = append(params.Statuses, status)
		}
	}

	return params, nil
}
