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

// CreateRequestToParams converts CreateSessionTemplateRequest to application params
func CreateRequestToParams(
	req *dtos.CreateSessionTemplateRequest,
	activityGroupID uuid.UUID,
	createdByID uuid.UUID,
) (*application.CreateSessionTemplateParams, error) {
	params := &application.CreateSessionTemplateParams{
		ActivityGroupID:      activityGroupID,
		CreatedByID:          createdByID,
		Title:                req.Title,
		Description:          req.Description,
		RecurrenceFrequency:  req.RecurrenceFrequency,
		RecurrenceInterval:   req.RecurrenceInterval,
		RecurrenceTimeHour:   req.RecurrenceTimeHour,
		RecurrenceTimeMinute: req.RecurrenceTimeMinute,
		DefaultCapacity:      req.DefaultCapacity,
		LocationCity:         req.LocationCity,
		LocationCountry:      req.LocationCountry,
	}

	// Convert day of week from int to time.Weekday if provided
	if req.RecurrenceDayOfWeek != nil {
		dow := time.Weekday(*req.RecurrenceDayOfWeek)
		params.RecurrenceDayOfWeek = &dow
	}

	// Day of month
	if req.RecurrenceDayOfMonth != nil {
		params.RecurrenceDayOfMonth = req.RecurrenceDayOfMonth
	}

	// Parse recurrence end time if provided
	if req.RecurrenceEndsAt != nil {
		endsAt, err := time.Parse(time.RFC3339, *req.RecurrenceEndsAt)
		if err != nil {
			return nil, err
		}
		params.RecurrenceEndsAt = &endsAt
	}

	return params, nil
}

// UpdateRequestToParams converts UpdateSessionTemplateRequest to application params
func UpdateRequestToParams(req *dtos.UpdateSessionTemplateRequest) (*application.UpdateSessionTemplateParams, error) {
	params := &application.UpdateSessionTemplateParams{
		Title:                req.Title,
		Description:          req.Description,
		RecurrenceFrequency:  req.RecurrenceFrequency,
		RecurrenceInterval:   req.RecurrenceInterval,
		RecurrenceTimeHour:   req.RecurrenceTimeHour,
		RecurrenceTimeMinute: req.RecurrenceTimeMinute,
		DefaultCapacity:      req.DefaultCapacity,
		LocationCity:         req.LocationCity,
		LocationCountry:      req.LocationCountry,
	}

	// Convert day of week from int to time.Weekday if provided
	if req.RecurrenceDayOfWeek != nil {
		dow := time.Weekday(*req.RecurrenceDayOfWeek)
		params.RecurrenceDayOfWeek = &dow
	}

	// Day of month
	if req.RecurrenceDayOfMonth != nil {
		params.RecurrenceDayOfMonth = req.RecurrenceDayOfMonth
	}

	// Parse recurrence end time if provided
	if req.RecurrenceEndsAt != nil {
		endsAt, err := time.Parse(time.RFC3339, *req.RecurrenceEndsAt)
		if err != nil {
			return nil, err
		}
		params.RecurrenceEndsAt = &endsAt
	}

	return params, nil
}

// DomainToResponse converts domain SessionTemplate to SessionTemplateResponse
func DomainToResponse(template *domain.SessionTemplate) *dtos.SessionTemplateResponse {
	resp := &dtos.SessionTemplateResponse{
		ID:              template.ID(),
		ActivityGroupID: template.ActivityGroupID(),
		CreatedByID:     template.CreatedByID(),
		Title:           template.Title(),
		Description:     template.Description(),
		Status:          template.Status().String(),
		CreatedAt:       template.CreatedAt(),
		UpdatedAt:       template.UpdatedAt(),
	}

	// Default capacity
	if cap := template.DefaultCapacity(); cap != nil {
		resp.DefaultCapacity = cap
	}

	// Location
	if city := template.LocationCity(); city != "" {
		resp.LocationCity = &city
	}
	if country := template.LocationCountry(); country != "" {
		resp.LocationCountry = &country
	}

	// Generated up to
	if generatedUpTo := template.GeneratedUpTo(); generatedUpTo != nil {
		resp.GeneratedUpTo = generatedUpTo
	}

	// Recurrence details - only populate if template is recurring
	if template.IsRecurring() {
		recurrence := &dtos.RecurrenceInfo{
			IsRecurring: true,
		}

		freq := string(template.RecurrenceFrequency())
		recurrence.Frequency = &freq

		interval := template.RecurrenceInterval()
		recurrence.Interval = &interval

		// Day of week (only if >= 0, since -1 means not set)
		if dow := template.RecurrenceDayOfWeek(); dow >= 0 {
			recurrence.DayOfWeek = &dow
		}

		// Day of month (only if > 0)
		if dom := template.RecurrenceDayOfMonth(); dom > 0 {
			recurrence.DayOfMonth = &dom
		}

		// Time hour (only if >= 0, since -1 means not set)
		if hour := template.RecurrenceTimeHour(); hour >= 0 {
			recurrence.TimeHour = &hour
		}

		// Time minute (only if >= 0, since -1 means not set)
		if minute := template.RecurrenceTimeMinute(); minute >= 0 {
			recurrence.TimeMinute = &minute
		}

		// Ends at
		if endsAt := template.RecurrenceEndsAt(); endsAt != nil {
			recurrence.EndsAt = endsAt
		}

		resp.Recurrence = recurrence
	}

	// Deleted at
	if deletedAt := template.DeletedAt(); deletedAt != nil {
		resp.DeletedAt = deletedAt
	}

	return resp
}

// DomainListToResponse converts list of domain SessionTemplates to response
func DomainListToResponse(
	templates []*domain.SessionTemplate,
	limit, offset int,
) *dtos.SessionTemplateListResponse {
	resp := &dtos.SessionTemplateListResponse{
		Templates: make([]dtos.SessionTemplateResponse, 0, len(templates)),
		Total:     len(templates),
		Limit:     limit,
		Offset:    offset,
	}

	for _, template := range templates {
		resp.Templates = append(resp.Templates, *DomainToResponse(template))
	}

	return resp
}

// QueryToListParams converts URL query parameters to ListSessionTemplatesParams
func QueryToListParams(query url.Values) (*application.ListSessionTemplatesParams, error) {
	params := &application.ListSessionTemplatesParams{
		Limit:  20, // default
		Offset: 0,  // default
	}

	// Activity group ID
	if groupIDStr := query.Get("activity_group_id"); groupIDStr != "" {
		groupID, err := uuid.Parse(groupIDStr)
		if err != nil {
			return nil, err
		}
		params.ActivityGroupID = &groupID
	}

	// Created by ID
	if creatorIDStr := query.Get("created_by_id"); creatorIDStr != "" {
		creatorID, err := uuid.Parse(creatorIDStr)
		if err != nil {
			return nil, err
		}
		params.CreatedByID = &creatorID
	}

	// Status
	if status := query.Get("status"); status != "" {
		params.Status = &status
	}

	// Is recurring
	if isRecurringStr := query.Get("is_recurring"); isRecurringStr != "" {
		isRecurring, err := strconv.ParseBool(isRecurringStr)
		if err != nil {
			return nil, err
		}
		params.IsRecurring = &isRecurring
	}

	// Limit
	if limitStr := query.Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			return nil, err
		}
		params.Limit = limit
	}

	// Offset
	if offsetStr := query.Get("offset"); offsetStr != "" {
		offset, err := strconv.Atoi(offsetStr)
		if err != nil {
			return nil, err
		}
		params.Offset = offset
	}

	return params, nil
}
