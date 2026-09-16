package dtos

import (
	"time"

	"github.com/google/uuid"
)

// NotificationResponse is a response containing a single notification
type NotificationResponse struct {
	ID        uuid.UUID      `json:"id" example:"123e4567-e89b-12d3-a456-426655440000"`
	Type      string         `json:"type" example:"join_request_created"`
	Data      map[string]any `json:"data"`
	ReadAt    *time.Time     `json:"read_at,omitempty" example:"2024-01-01T00:00:00Z"`
	CreatedAt time.Time      `json:"created_at" example:"2024-01-01T00:00:00Z"`
}

// NotificationListResponse is a response containing a page of notifications.
// Not the shared generic api.Page[T] envelope, since this endpoint needs one
// extra field (UnreadCount) that no other list endpoint carries.
type NotificationListResponse struct {
	Items         []NotificationResponse `json:"items"`
	Limit         int                    `json:"limit"`
	NextPageToken string                 `json:"next_page_token,omitempty"`
	UnreadCount   int                    `json:"unread_count"`
}
