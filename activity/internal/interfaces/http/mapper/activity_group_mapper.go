package mapper

import (
	"net/url"
	"strconv"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/http/dtos"
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

// ActivityGroupListToResponse converts list of activity groups to list response
func ActivityGroupListToResponse(
	groups []*domain.ActivityGroup,
	limit int,
	offset int,
) *dtos.ActivityGroupListResponse {
	responses := make([]dtos.ActivityGroupResponse, len(groups))
	for i, group := range groups {
		responses[i] = *ActivityGroupToResponse(group)
	}

	return &dtos.ActivityGroupListResponse{
		Groups: responses,
		Total:  len(responses),
		Limit:  limit,
		Offset: offset,
	}
}

// QueryToListActivityGroupsParams parses query parameters for listing activity groups
func QueryToListActivityGroupsParams(query url.Values) (*application.ListActivityGroupsParams, error) {
	params := &application.ListActivityGroupsParams{
		Limit:  20, // default
		Offset: 0,  // default
	}

	// Parse creator_id filter
	if creatorIDStr := query.Get("creator_id"); creatorIDStr != "" {
		creatorID, err := uuid.Parse(creatorIDStr)
		if err != nil {
			return nil, err
		}
		params.CreatorID = &creatorID
	}

	// Parse status filter
	if status := query.Get("status"); status != "" {
		params.Status = &status
	}

	// Parse title filter
	if title := query.Get("title"); title != "" {
		params.Title = &title
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

// QueryToDiscoverActivityGroupsParams parses query parameters for discovering activity groups
func QueryToDiscoverActivityGroupsParams(query url.Values) (*application.DiscoverActivityGroupsParams, error) {
	params := &application.DiscoverActivityGroupsParams{
		Limit:  20, // default
		Offset: 0,  // default
	}

	// Parse city filter
	if city := query.Get("city"); city != "" {
		params.City = &city
	}

	// Parse country filter
	if country := query.Get("country"); country != "" {
		params.Country = &country
	}

	// Parse activity_type filter
	if activityType := query.Get("activity_type"); activityType != "" {
		params.ActivityType = &activityType
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
