package mapper

import (
	"net/url"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/http/dtos"
	"github.com/rasparac/rekreativko-api/shared/api"
)

// CreateActivityGroupRequestToParams converts CreateActivityGroupRequest to application params
func CreateActivityGroupRequestToParams(
	req *dtos.CreateActivityGroupRequest,
	creatorID uuid.UUID,
) *application.CreateActivityGroupParams {
	return &application.CreateActivityGroupParams{
		CreatorID:       creatorID,
		Title:           req.Title,
		Description:     req.Description,
		ActivityType:    req.ActivityType,
		DifficultyLevel: req.DifficultyLevel,
		Visibility:      req.Visibility,
		LocationCity:    req.LocationCity,
		LocationCountry: req.LocationCountry,
		Timezone:        req.Timezone,
		DefaultCapacity: req.DefaultCapacity,
	}
}

// UpdateActivityGroupRequestToParams converts UpdateActivityGroupRequest to application params
func UpdateActivityGroupRequestToParams(
	req *dtos.UpdateActivityGroupRequest,
	requesterID uuid.UUID,
) *application.UpdateActivityGroupParams {
	return &application.UpdateActivityGroupParams{
		RequesterID:     requesterID,
		Title:           req.Title,
		Description:     req.Description,
		ActivityType:    req.ActivityType,
		DifficultyLevel: req.DifficultyLevel,
		LocationCity:    req.LocationCity,
		LocationCountry: req.LocationCountry,
		Timezone:        req.Timezone,
	}
}

// ActivityGroupToResponse converts domain ActivityGroup to ActivityGroupResponse
func ActivityGroupToResponse(group *domain.ActivityGroup) *dtos.ActivityGroupResponse {
	resp := &dtos.ActivityGroupResponse{
		ID:              group.ID(),
		CreatorID:       group.CreatorID(),
		Title:           group.Title().Value(),
		Description:     group.Description(),
		ActivityType:    group.ActivityType().String(),
		DifficultyLevel: string(group.DifficultyLevel()),
		Visibility:      group.Visibility().String(),
		Status:          string(group.Status()),
		LocationCity:    group.Location().City(),
		LocationCountry: group.Location().Country(),
		Timezone:        group.Timezone(),
		CreatedAt:       group.CreatedAt(),
		UpdatedAt:       group.UpdatedAt(),
	}

	// Default capacity (optional)
	if cap := group.DefaultCapacity(); cap != nil {
		capacity := cap.Capacity()
		resp.DefaultCapacity = &capacity
	}

	// Cancelled at (optional)
	if cancelledAt := group.CancelledAt(); cancelledAt != nil {
		resp.CancelledAt = cancelledAt
	}

	// Deleted at (optional)
	if deletedAt := group.DeletedAt(); deletedAt != nil {
		resp.DeletedAt = deletedAt
	}

	return resp
}

// QueryToListActivityGroupsParams parses query parameters for listing activity groups
func QueryToListActivityGroupsParams(query url.Values) (*application.ListActivityGroupsParams, error) {
	params := &application.ListActivityGroupsParams{
		Limit: 20, // default
	}

	// Parse creator_id filter
	if creatorIDStr := query.Get("creator_id"); creatorIDStr != "" {
		creatorID, err := uuid.Parse(creatorIDStr)
		if err != nil {
			return nil, err
		}
		params.CreatorID = &creatorID
	}

	// Parse member_id filter
	if memberIDStr := query.Get("member_id"); memberIDStr != "" {
		memberID, err := uuid.Parse(memberIDStr)
		if err != nil {
			return nil, err
		}
		params.MemberID = &memberID
	}

	// Parse member_status filter - repeatable, e.g.
	// ?member_status=pending&member_status=confirmed. Only meaningful
	// alongside member_id.
	if statuses := query["member_status"]; len(statuses) > 0 {
		params.MemberStatus = statuses
	}

	// Parse status filter
	if status := query.Get("status"); status != "" {
		params.Status = &status
	}

	// Parse title filter
	if title := query.Get("title"); title != "" {
		params.Title = &title
	}

	// Parse activity_type filter
	if activityType := query.Get("activity_type"); activityType != "" {
		params.ActivityType = &activityType
	}

	// Parse difficulty_level filter
	if difficultyLevel := query.Get("difficulty_level"); difficultyLevel != "" {
		params.DifficultyLevel = &difficultyLevel
	}

	limit, pageToken, err := api.ParsePageParams(query, params.Limit)
	if err != nil {
		return nil, err
	}
	params.Limit = limit
	params.PageToken = pageToken

	return params, nil
}

// QueryToDiscoverActivityGroupsParams parses query parameters for discovering activity groups
func QueryToDiscoverActivityGroupsParams(query url.Values) (*application.DiscoverActivityGroupsParams, error) {
	params := &application.DiscoverActivityGroupsParams{
		Limit: 20, // default
	}

	// Parse city filter
	if city := query.Get("city"); city != "" {
		params.City = &city
	}

	// Parse country filter
	if country := query.Get("country"); country != "" {
		params.Country = &country
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
