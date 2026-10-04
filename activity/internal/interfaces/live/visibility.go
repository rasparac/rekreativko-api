package live

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/application"
)

// managerOnlyEvents are attendee events about people who are not (yet) going:
// join requests and the waitlist. Only the session's managers - and the person
// the event is about - may see them; everyone else's snapshot has no such
// people in it, so they lose nothing by not getting the event.
var managerOnlyEvents = map[string]struct{}{
	"attendee.join_requested":    {},
	"attendee.join_rejected":     {},
	"attendee.auto_pending":      {},
	"attendee.rsvp_auto_pending": {},
}

// managerIDsField lists who the managers are (it is how notifications know
// whom to tell). Viewers other than managers don't get it.
const managerIDsField = "manager_user_ids"

var managerIDsKey = []byte(`"` + managerIDsField + `"`)

// deliver offers the event to the session's clients, holding back what a
// viewer may not see: manager-only events, and the managers' ids. Anything
// that needs a manager check fails closed - if the managers can't be
// resolved, nobody is treated as one.
func (h *Hub) deliver(
	ctx context.Context,
	clients []*Client,
	sessionID uuid.UUID,
	event string,
	subjectID uuid.UUID,
	payload []byte,
	snapshot *application.TeamFormationSnapshot,
) {
	_, managerOnly := managerOnlyEvents[event]

	full := Update{Event: event, Change: json.RawMessage(payload), Snapshot: snapshot}

	if !managerOnly && !bytes.Contains(payload, managerIDsKey) {
		for _, c := range clients {
			c.offer(full)
		}
		return
	}

	managers := h.managerSet(ctx, sessionID)
	redacted := Update{Event: event, Change: h.withoutManagerIDs(ctx, payload), Snapshot: snapshot}

	for _, c := range clients {
		_, isManager := managers[c.ViewerID]

		switch {
		case isManager:
			c.offer(full)
		case managerOnly && c.ViewerID != subjectID:
			// not theirs to know
		default:
			c.offer(redacted)
		}
	}
}

func (h *Hub) managerSet(ctx context.Context, sessionID uuid.UUID) map[uuid.UUID]struct{} {
	ids, err := h.managers.SessionManagers(ctx, sessionID)
	if err != nil {
		h.logger.Error(ctx, "failed to resolve session managers; treating nobody as one", "session_id", sessionID, "error", err)
		return nil
	}

	set := make(map[uuid.UUID]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set
}

// withoutManagerIDs is payload minus the managers' ids. If it can't be
// rewritten the change is dropped (nil) rather than sent as it is.
func (h *Hub) withoutManagerIDs(ctx context.Context, payload []byte) json.RawMessage {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		h.logger.Warn(ctx, "cannot redact session event; sending it without its change", "error", err)
		return nil
	}

	delete(fields, managerIDsField)

	redacted, err := json.Marshal(fields)
	if err != nil {
		return nil
	}
	return redacted
}
