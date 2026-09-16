package mapper

import (
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/activity/internal/infrastructure/persistence"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/http/dtos"
	"github.com/rasparac/rekreativko-api/shared/api"
)

// CreateSessionRequestToParams converts CreateSessionRequest to application params
func CreateSessionRequestToParams(
	req *dtos.CreateSessionRequest,
	createdByID uuid.UUID,
) *application.CreateSessionParams {
	return &application.CreateSessionParams{
		ActivityGroupID:  req.ActivityGroupID,
		CreatedByID:      createdByID,
		Title:            req.Title,
		ActivityType:     req.ActivityType,
		DifficultyLevel:  req.DifficultyLevel,
		LocationCity:     req.LocationCity,
		LocationCountry:  req.LocationCountry,
		LocationStreet:   req.LocationStreet,
		LocationLat:      req.LocationLat,
		LocationLng:      req.LocationLng,
		StartTime:        req.StartTime,
		EndTime:          req.EndTime,
		Capacity:         req.Capacity,
		Note:             req.Note,
		IsRecurring:      req.IsRecurring,
		Visibility:       req.Visibility,
		RequiresApproval: req.RequiresApproval,
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
		LocationStreet:  req.LocationStreet,
		LocationLat:     req.LocationLat,
		LocationLng:     req.LocationLng,
		StartTime:       req.StartTime,
		EndTime:         req.EndTime,
		Capacity:        req.Capacity,
		Note:            req.Note,
		Visibility:      req.Visibility,
	}
}

// SessionToResponse converts domain Session to SessionResponse. attendeeStatuses
// is looked up by session ID for the requesting user's own RSVP status - pass
// nil when that isn't available/relevant (e.g. a single-session fetch where the
// caller hasn't computed it).
func SessionToResponse(session *domain.Session, attendeeStatuses map[uuid.UUID]domain.AttendeeStatus) *dtos.SessionResponse {
	resp := &dtos.SessionResponse{
		ID:               session.ID(),
		ActivityGroupID:  session.ActivityGroupID(),
		CreatedByID:      session.CreatedByID(),
		TemplateID:       session.TemplateID(),
		Title:            session.Title().Value(),
		ActivityType:     session.ActivityType().String(),
		DifficultyLevel:  session.DifficultyLevel().String(),
		LocationCity:     session.Location().City(),
		LocationCountry:  session.Location().Country(),
		LocationStreet:   session.Location().Street(),
		LocationLat:      session.Location().Latitude(),
		LocationLng:      session.Location().Longitude(),
		StartTime:        session.Schedule().StartTime(),
		EndTime:          session.Schedule().EndTime(),
		Capacity:         session.Capacity(),
		Status:           session.Status().String(),
		Visibility:       session.Visibility().String(),
		RequiresApproval: session.RequiresApproval(),
		IsRecurring:      session.IsRecurring(),
		Note:             session.Note(),
		OpenAt:           session.OpenAt(),
		CreatedAt:        session.CreatedAt(),
		UpdatedAt:        session.UpdatedAt(),
		CancelledAt:      session.CancelledAt(),
		StartedAt:        session.StartedAt(),
		CompletedAt:      session.CompletedAt(),
	}

	if status, ok := attendeeStatuses[session.ID()]; ok {
		statusStr := string(status)
		resp.AttendeeStatus = &statusStr
	}

	return resp
}

// SessionWithDistanceToResponse converts a SessionWithDistance to a NearbySessionResponse
func SessionWithDistanceToResponse(result persistence.SessionWithDistance, attendeeStatuses map[uuid.UUID]domain.AttendeeStatus) dtos.NearbySessionResponse {
	return dtos.NearbySessionResponse{
		SessionResponse: *SessionToResponse(result.Session, attendeeStatuses),
		DistanceKm:      result.DistanceKM,
	}
}

// QueryToDiscoverSessionsParams parses query parameters for discovering nearby sessions
func QueryToDiscoverSessionsParams(query url.Values) (*application.DiscoverSessionsParams, error) {
	params := &application.DiscoverSessionsParams{
		RadiusKM: 10, // default 10km
		Limit:    20, // default
	}

	latStr := query.Get("lat")
	if latStr == "" {
		return nil, fmt.Errorf("lat is required")
	}
	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid lat: %w", err)
	}
	params.Lat = lat

	lngStr := query.Get("lng")
	if lngStr == "" {
		return nil, fmt.Errorf("lng is required")
	}
	lng, err := strconv.ParseFloat(lngStr, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid lng: %w", err)
	}
	params.Lng = lng

	if radiusStr := query.Get("radius_km"); radiusStr != "" {
		radius, err := strconv.ParseFloat(radiusStr, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid radius_km: %w", err)
		}
		params.RadiusKM = radius
	}

	params.Interests = parseInterestsFromQuery(query)

	limit, pageToken, err := api.ParsePageParams(query, params.Limit)
	if err != nil {
		return nil, err
	}
	params.Limit = limit
	params.PageToken = pageToken

	return params, nil
}

// QueryToListSessionsParams parses query parameters for listing sessions
func QueryToListSessionsParams(query url.Values) (*application.ListSessionsParams, error) {
	params := &application.ListSessionsParams{
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

	// Parse session_template_id filter
	if templateIDStr := query.Get("session_template_id"); templateIDStr != "" {
		templateID, err := uuid.Parse(templateIDStr)
		if err != nil {
			return nil, err
		}
		params.SessionTemplateID = &templateID
	}

	// Parse created_by_id filter
	if createdByIDStr := query.Get("created_by_id"); createdByIDStr != "" {
		createdByID, err := uuid.Parse(createdByIDStr)
		if err != nil {
			return nil, err
		}
		params.CreatedByID = &createdByID
	}

	// Parse attendee_id filter
	if attendeeIDStr := query.Get("attendee_id"); attendeeIDStr != "" {
		attendeeID, err := uuid.Parse(attendeeIDStr)
		if err != nil {
			return nil, err
		}
		params.AttendeeID = &attendeeID
	}

	// Parse attendee_status filter - repeatable, e.g.
	// ?attendee_status=going&attendee_status=pending. Only meaningful
	// alongside attendee_id.
	if statuses := query["attendee_status"]; len(statuses) > 0 {
		params.AttendeeStatus = statuses
	}

	// Parse status filter
	if status := query.Get("status"); status != "" {
		params.Status = &status
	}

	// Parse activity_type filter
	if activityType := query.Get("activity_type"); activityType != "" {
		params.ActivityType = &activityType
	}

	// Parse difficulty_level filter
	if difficultyLevel := query.Get("difficulty_level"); difficultyLevel != "" {
		params.DifficultyLevel = &difficultyLevel
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

	limit, pageToken, err := api.ParsePageParams(query, params.Limit)
	if err != nil {
		return nil, err
	}
	params.Limit = limit
	params.PageToken = pageToken

	return params, nil
}
