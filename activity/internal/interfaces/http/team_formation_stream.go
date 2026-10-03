package http

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/http/dtos"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/http/mapper"
	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/rasparac/rekreativko-api/shared/authcontext"
)

// sseHeartbeatInterval keeps idle streams from being closed by proxies and
// lets clients notice a dead connection. A variable so tests can shorten it.
var sseHeartbeatInterval = 20 * time.Second

// StreamTeamFormation handles GET /api/v1/sessions/{id}/events
//
//	@Summary		Stream team-formation changes
//	@Description	Server-sent events stream of the session's team formation (captain draft, proposals and votes, team assignments). The first event is "snapshot"; every later event is named after the change (e.g. "draft.player_picked", "voting.vote_cast", "attendee.team_assigned"). Every event's data is {change, snapshot}: the domain event with the ids involved, and the whole state after it (same document as GET /sessions/{id}/team-formation); its id is the snapshot version. A ": ping" comment keeps idle connections alive. Before closing a stream on its own the server sends a "disconnected" event with a reason. On reconnect you get a fresh snapshot - no replay. Changes arrive within the outbox poll interval (5s by default).
//	@Tags			Team formation
//	@Produce		text/event-stream
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			id	path		string							true	"Session ID"
//	@Success		200	{object}	dtos.TeamFormationEventData		"Event stream"
//	@Failure		400	{object}	api.Response[any]				"Invalid request"
//	@Failure		401	{object}	api.Response[any]				"Unauthorized"
//	@Failure		404	{object}	api.Response[any]				"Session not found"
//	@Failure		500	{object}	api.Response[any]				"Internal server error"
//	@Router			/api/v1/sessions/{id}/events [get]
func (h *Handler) StreamTeamFormation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	sessionID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		h.logger.Error(ctx, "invalid session ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_session_id", "Invalid session ID")
		return
	}

	if _, err := h.sessionService.GetSession(ctx, sessionID, accountID); err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	rc := http.NewResponseController(w)

	// The server's WriteTimeout is meant for ordinary requests and would cut
	// the stream; this response has no write deadline.
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		h.logger.Error(ctx, "response writer cannot stream", "error", err)
		api.WriteInternalServerErrorResponse(w)
		return
	}

	client, err := h.live.Connect(ctx, sessionID, accountID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}
	defer h.live.Disconnect(client)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // tell nginx-style proxies not to buffer
	w.WriteHeader(http.StatusOK)
	if err := rc.Flush(); err != nil {
		return
	}

	h.logger.Debug(ctx, "team-formation stream opened", "session_id", sessionID)

	heartbeat := time.NewTicker(sseHeartbeatInterval)
	defer heartbeat.Stop()

	for {
		var err error

		select {
		case <-ctx.Done():
			return

		case <-client.Done():
			if reason := client.Reason(); reason != "" {
				h.logger.Debug(ctx, "team-formation stream closed by server", "session_id", sessionID, "reason", reason)
				_ = writeSSE(w, "", "disconnected", dtos.StreamDisconnectedData{Reason: reason})
				_ = rc.Flush()
			}
			return

		case update := <-client.Updates():
			err = writeSSE(
				w,
				strconv.FormatInt(update.Snapshot.Version, 10),
				update.Event,
				dtos.TeamFormationEventData{
					Change:   update.Change,
					Snapshot: mapper.TeamFormationSnapshotToResponse(update.Snapshot, accountID),
				},
			)

		case <-heartbeat.C:
			_, err = io.WriteString(w, ": ping\n\n")
		}

		if err == nil {
			err = rc.Flush()
		}
		if err != nil {
			// The client went away.
			return
		}
	}
}

// writeSSE writes one server-sent event. JSON has no raw newlines, so the
// data always fits on a single "data:" line.
func writeSSE(w io.Writer, id, event string, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("encode event data: %w", err)
	}

	if id != "" {
		if _, err := fmt.Fprintf(w, "id: %s\n", id); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, payload)
	return err
}
