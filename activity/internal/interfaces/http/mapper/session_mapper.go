package mapper

import (
	"net/url"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/http/dtos"
)

// CreateSessionRequestToParams converts CreateSessionRequest to application params
func CreateSessionRequestToParams(
	req *dtos.CreateSessionRequest,
	createdByID uuid.UUID,
) *application.CreateSessionParams {
	return &application.CreateSessionParams{
		ActivityGroupID: req.ActivityGroupID,
		CreatedByID:     createdByID,
		LocationCity:    req.LocationCity,
		LocationCountry: req.LocationCountry,
		LocationLat:     req.LocationLat,
		LocationLng:     req.LocationLng,
		StartTime:       req.StartTime,
		EndTime:         req.EndTime,
		Capacity:        req.Capacity,
		Note:            req.Note,
		IsRecurring:     req.IsRecurring,
	}
}

// UpdateSessionRequestToParams converts UpdateSessionRequest to application params
func UpdateSessionRequestToParams(
	req *dtos.UpdateSessionRequest,
	requesterID uuid.UUID,
	requesterRole string,
) *application.UpdateSessionParams {
	return &application.UpdateSessionParams{
		RequesterID:     requesterID,
		RequesterRole:   requesterRole,
		LocationCity:    req.LocationCity,
		LocationCountry: req.LocationCountry,
		LocationLat:     req.LocationLat,
		LocationLng:     req.LocationLng,
		StartTime:       req.StartTime,
		EndTime:         req.EndTime,
		Capacity:        req.Capacity,
		Note:            req.Note,
	}
}

// SessionToResponse converts domain Session to SessionResponse
func SessionToResponse(session *domain.Session) *dtos.SessionResponse {
	resp := &dtos.SessionResponse{
		ID:              session.ID(),
		ActivityGroupID: session.ActivityGroupID(),
		CreatedByID:     session.CreatedByID(),
		TemplateID:      session.TemplateID(),
		LocationCity:    session.Location().City(),
		LocationCountry: session.Location().Country(),
		LocationLat:     session.Location().Latitude(),
		LocationLng:     session.Location().Longitude(),
		StartTime:       session.Schedule().StartTime(),
		EndTime:         session.Schedule().EndTime(),
		Capacity:        session.Capacity(),
		Status:          session.Status().String(),
		IsRecurring:     session.IsRecurring(),
		Note:            session.Note(),
		OpenAt:          session.OpenAt(),
		CreatedAt:       session.CreatedAt(),
		UpdatedAt:       session.UpdatedAt(),
		CancelledAt:     session.CancelledAt(),
		StartedAt:       session.StartedAt(),
		CompletedAt:     session.CompletedAt(),
	}

	return resp
}

// SessionListToResponse converts list of sessions to list response
func SessionListToResponse(
	sessions []*domain.Session,
	limit int,
	offset int,
) *dtos.SessionListResponse {
	responses := make([]dtos.SessionResponse, len(sessions))
	for i, session := range sessions {
		responses[i] = *SessionToResponse(session)
	}

	return &dtos.SessionListResponse{
		Sessions: responses,
		Total:    len(responses),
		Limit:    limit,
		Offset:   offset,
	}
}

// QueryToListSessionsParams parses query parameters for listing sessions
func QueryToListSessionsParams(query url.Values) (*application.ListSessionsParams, error) {
	params := &application.ListSessionsParams{
		Limit:  20, // default
		Offset: 0,  // default
	}

	// Parse activity_group_id filter
	if groupIDStr := query.Get("activity_group_id"); groupIDStr != "" {
		groupID, err := uuid.Parse(groupIDStr)
		if err != nil {
			return nil, err
		}
		params.ActivityGroupID = &groupID
	}

	// Parse session_template_id filter
	if templateIDStr := query.Get("session_template_id"); templateIDStr != "" {
		templateID, err := uuid.Parse(templateIDStr)
		if err != nil {
			return nil, err
		}
		params.SessionTemplateID = &templateID
	}

	// Parse status filter
	if status := query.Get("status"); status != "" {
		params.Status = &status
	}

	// Parse is_recurring filter
	if isRecurringStr := query.Get("is_recurring"); isRecurringStr != "" {
		isRecurring, err := strconv.ParseBool(isRecurringStr)
		if err != nil {
			return nil, err
		}
		params.IsRecurring = &isRecurring
	}

	// Parse start_time_from filter
	if startTimeFromStr := query.Get("start_time_from"); startTimeFromStr != "" {
		startTimeFrom, err := time.Parse(time.RFC3339, startTimeFromStr)
		if err != nil {
			return nil, err
		}
		params.StartTimeFrom = &startTimeFrom
	}

	// Parse start_time_to filter
	if startTimeToStr := query.Get("start_time_to"); startTimeToStr != "" {
		startTimeTo, err := time.Parse(time.RFC3339, startTimeToStr)
		if err != nil {
			return nil, err
		}
		params.StartTimeTo = &startTimeTo
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
