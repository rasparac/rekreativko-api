package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/notifications/internal/application"
	"github.com/rasparac/rekreativko-api/notifications/internal/domain"
	"github.com/rasparac/rekreativko-api/notifications/internal/interfaces/http/dtos"
	"github.com/rasparac/rekreativko-api/notifications/internal/interfaces/http/mapper"
	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/rasparac/rekreativko-api/shared/authcontext"
	"github.com/rasparac/rekreativko-api/shared/domainerror"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/middleware"
)

type (
	notificationService interface {
		ListNotifications(ctx context.Context, params application.ListNotificationsParams) ([]*domain.Notification, int, string, error)
		MarkAsRead(ctx context.Context, params application.MarkAsReadParams) error
	}

	Handler struct {
		notificationService notificationService
		logger              *logger.Logger
	}
)

// NewHandler creates a new notifications HTTP handler
func NewHandler(
	notificationService notificationService,
	log *logger.Logger,
) *Handler {
	return &Handler{
		notificationService: notificationService,
		logger:              log.WithName("notifications.http.handler"),
	}
}

func (h *Handler) RegisterRoutes(
	mux *http.ServeMux,
	middlewares *middleware.Chain,
) {
	mux.Handle(
		"GET /api/v1/notifications",
		middlewares.ThenFunc(h.GetNotifications),
	)
	mux.Handle(
		"POST /api/v1/notifications/{id}/read",
		middlewares.ThenFunc(h.MarkNotificationAsRead),
	)
}

// GetNotifications handles GET /api/v1/notifications
//
//	@Summary		List my notifications
//	@Description	Returns the authenticated user's notification feed (newest first) alongside their current unread count - everything a home-screen notification badge needs in one call.
//	@Tags			Notifications
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			unread_only	query		boolean											false	"Only return unread notifications"
//	@Param			limit		query		int												false	"Limit number of results (default 20)"
//	@Param			page_token	query		string											false	"Token from the previous response's next_page_token, to fetch the next page"
//	@Success		200			{object}	api.Response[dtos.NotificationListResponse]	"Notifications retrieved successfully"
//	@Failure		400			{object}	api.Response[any]								"Invalid request"
//	@Failure		401			{object}	api.Response[any]								"Unauthorized"
//	@Failure		500			{object}	api.Response[any]								"Internal server error"
//	@Router			/api/v1/notifications [get]
func (h *Handler) GetNotifications(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	params, err := mapper.QueryToListNotificationsParams(r.URL.Query())
	if err != nil {
		h.logger.Error(ctx, "failed to parse query parameters", "error", err)
		api.WriteBadRequestResponse(w, "invalid_params", "Invalid query parameters")
		return
	}
	params.RecipientAccountID = accountID

	notifications, unreadCount, nextPageToken, err := h.notificationService.ListNotifications(ctx, *params)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	var resp *dtos.NotificationListResponse = mapper.NotificationListToResponse(notifications, params.Limit, nextPageToken, unreadCount)

	api.WriteOkResponse(w, resp, "")
}

// MarkNotificationAsRead handles POST /api/v1/notifications/{id}/read
//
//	@Summary		Mark a notification as read
//	@Description	Marks a single notification as read. Only the notification's own recipient may do this.
//	@Tags			Notifications
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			id	path		string							true	"Notification ID"
//	@Success		200	{object}	api.Response[any]				"Notification marked read"
//	@Failure		400	{object}	api.Response[any]				"Invalid request"
//	@Failure		401	{object}	api.Response[any]				"Unauthorized"
//	@Failure		403	{object}	api.Response[any]				"Not your notification"
//	@Failure		404	{object}	api.Response[any]				"Notification not found"
//	@Failure		500	{object}	api.Response[any]				"Internal server error"
//	@Router			/api/v1/notifications/{id}/read [post]
func (h *Handler) MarkNotificationAsRead(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		h.logger.Error(ctx, "invalid notification ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_notification_id", "Invalid notification ID")
		return
	}

	err = h.notificationService.MarkAsRead(ctx, application.MarkAsReadParams{
		NotificationID: id,
		RequesterID:    accountID,
	})
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(w, struct{}{}, "Notification marked read")
}

func (h *Handler) handleServiceError(ctx context.Context, w http.ResponseWriter, err error) {
	appErr := domainerror.GetAppError(err)

	api.WriteError(
		w,
		appErr.StatusCode,
		appErr.Code,
		appErr.Message,
		nil,
	)
}
