package mapper

import (
	"net/url"
	"strconv"

	"github.com/rasparac/rekreativko-api/notifications/internal/application"
	"github.com/rasparac/rekreativko-api/notifications/internal/domain"
	"github.com/rasparac/rekreativko-api/notifications/internal/interfaces/http/dtos"
	"github.com/rasparac/rekreativko-api/shared/api"
)

// NotificationToResponse converts a domain Notification to NotificationResponse
func NotificationToResponse(n *domain.Notification) dtos.NotificationResponse {
	return dtos.NotificationResponse{
		ID:        n.ID(),
		Type:      string(n.Type()),
		Data:      n.Data(),
		ReadAt:    n.ReadAt(),
		CreatedAt: n.CreatedAt(),
	}
}

// NotificationListToResponse converts a list of domain Notifications to NotificationListResponse
func NotificationListToResponse(
	notifications []*domain.Notification,
	limit int,
	nextPageToken string,
	unreadCount int,
) *dtos.NotificationListResponse {
	items := make([]dtos.NotificationResponse, len(notifications))
	for i, n := range notifications {
		items[i] = NotificationToResponse(n)
	}

	return &dtos.NotificationListResponse{
		Items:         items,
		Limit:         limit,
		NextPageToken: nextPageToken,
		UnreadCount:   unreadCount,
	}
}

// QueryToListNotificationsParams parses query parameters for listing notifications
func QueryToListNotificationsParams(query url.Values) (*application.ListNotificationsParams, error) {
	params := &application.ListNotificationsParams{
		Limit: 20, // default
	}

	if unreadOnlyStr := query.Get("unread_only"); unreadOnlyStr != "" {
		unreadOnly, err := strconv.ParseBool(unreadOnlyStr)
		if err != nil {
			return nil, err
		}
		params.UnreadOnly = unreadOnly
	}

	limit, pageToken, err := api.ParsePageParams(query, params.Limit)
	if err != nil {
		return nil, err
	}
	params.Limit = limit
	params.PageToken = pageToken

	return params, nil
}
