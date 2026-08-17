package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/rasparac/rekreativko-api/shared/domainerror"
)

// getUserRole fetches the user's role in the activity group from the database
// Returns the role string or an error if the user is not a member
func (h *Handler) getUserRole(ctx context.Context, activityGroupID, accountID uuid.UUID) (string, error) {
	member, err := h.memberService.GetMember(ctx, activityGroupID, accountID)
	if err != nil {
		h.logger.Error(ctx, "failed to get user membership",
			"activity_group_id", activityGroupID,
			"account_id", accountID,
			"error", err)
		return "", domainerror.NotFound(domainerror.ErrCodeNotFound, "user is not a member of this activity group", err)
	}

	// Check if member is active
	if !member.Status().IsActive() {
		return "", domainerror.Forbidden(domainerror.ErrCodeForbidden, "user membership is not active", nil)
	}

	return member.Role().String(), nil
}

// handleServiceError converts service errors to HTTP responses
// Logging is handled by middleware based on HTTP status code
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
