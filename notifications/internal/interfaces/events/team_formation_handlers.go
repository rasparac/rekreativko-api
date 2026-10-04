package events

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/notifications/internal/application"
	"github.com/rasparac/rekreativko-api/notifications/internal/domain"
	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/rasparac/rekreativko-api/shared/events"
	"github.com/rasparac/rekreativko-api/shared/logger"
)

// Draft/voting cancel reasons that are not notified by the cancel handlers:
// session_ended is covered by session_cancelled, which reaches everyone going;
// roster_changed and teams_reset are covered by the teams_reset event, which
// tells everyone going once instead of once per thing that was cancelled.
const (
	sessionEndedReason  = "session_ended"
	rosterChangedReason = "roster_changed"
	teamsResetReason    = "teams_reset"
)

func cancelNotifiedElsewhere(reason string) bool {
	return reason == sessionEndedReason || reason == rosterChangedReason || reason == teamsResetReason
}

// notifyEach creates one notification of type t per recipient, skipping
// uuid.Nil, duplicates and actor (the person who triggered the event already
// knows). data is built per recipient so it can carry recipient-specific
// fields.
func notifyEach(
	ctx context.Context,
	notifications notificationCreator,
	log *logger.Logger,
	eventID uuid.UUID,
	recipients []uuid.UUID,
	actor uuid.UUID,
	t domain.NotificationType,
	data func(recipient uuid.UUID) map[string]any,
) error {
	seen := make(map[uuid.UUID]struct{}, len(recipients))
	for _, recipient := range recipients {
		if recipient == uuid.Nil || recipient == actor {
			continue
		}
		if _, dup := seen[recipient]; dup {
			continue
		}
		seen[recipient] = struct{}{}

		_, err := notifications.CreateNotification(ctx, application.CreateNotificationParams{
			EventID:            eventID,
			RecipientAccountID: recipient,
			Type:               string(t),
			Data:               data(recipient),
		})
		if err != nil {
			log.Error(ctx, "failed to create "+string(t)+" notification",
				"error", err, "recipient_account_id", recipient)
			return err
		}
	}

	return nil
}

func decodeEvent(payload []byte, event any) error {
	if err := json.Unmarshal(payload, event); err != nil {
		return events.Permanent(fmt.Errorf("decode payload: %w", err))
	}
	return nil
}

// draftStartedHandler tells both captains they were chosen.
type draftStartedHandler struct {
	notifications notificationCreator
	logger        *logger.Logger
}

func (h *draftStartedHandler) Handle(ctx context.Context, payload []byte) error {
	var event draftStartedEvent
	if err := decodeEvent(payload, &event); err != nil {
		return err
	}
	ctx = api.WithEventID(ctx, event.EventID.String())

	return notifyEach(ctx, h.notifications, h.logger, event.EventID, event.CaptainIDs[:], event.StartedBy,
		domain.NotificationTypeTeamDraftCaptainSelected,
		func(recipient uuid.UUID) map[string]any {
			position := 0
			if recipient == event.CaptainIDs[1] {
				position = 1
			}
			return map[string]any{
				"session_id":    event.SessionID,
				"draft_id":      event.DraftID,
				"team_position": position,
				"started_by":    event.StartedBy,
				"screen":        domain.ScreenTeamDraft,
			}
		})
}

// draftTurnChangedHandler tells a captain it is their turn to pick.
type draftTurnChangedHandler struct {
	notifications notificationCreator
	logger        *logger.Logger
}

func (h *draftTurnChangedHandler) Handle(ctx context.Context, payload []byte) error {
	var event draftTurnChangedEvent
	if err := decodeEvent(payload, &event); err != nil {
		return err
	}
	ctx = api.WithEventID(ctx, event.EventID.String())

	return notifyEach(ctx, h.notifications, h.logger, event.EventID, []uuid.UUID{event.CaptainID}, uuid.Nil,
		domain.NotificationTypeTeamDraftYourTurn,
		func(uuid.UUID) map[string]any {
			return map[string]any{
				"session_id":    event.SessionID,
				"draft_id":      event.DraftID,
				"team_position": event.TeamPosition,
				"pick_number":   event.PickNumber,
				"screen":        domain.ScreenTeamDraft,
			}
		})
}

// draftPausedHandler tells the organizer who started the draft that a captain
// left and a replacement is needed.
type draftPausedHandler struct {
	notifications notificationCreator
	logger        *logger.Logger
}

func (h *draftPausedHandler) Handle(ctx context.Context, payload []byte) error {
	var event draftPausedEvent
	if err := decodeEvent(payload, &event); err != nil {
		return err
	}
	ctx = api.WithEventID(ctx, event.EventID.String())

	return notifyEach(ctx, h.notifications, h.logger, event.EventID, []uuid.UUID{event.StartedBy}, uuid.Nil,
		domain.NotificationTypeTeamDraftPaused,
		func(uuid.UUID) map[string]any {
			return map[string]any{
				"session_id":    event.SessionID,
				"draft_id":      event.DraftID,
				"reason":        event.Reason,
				"team_position": event.TeamPosition,
				"captain_id":    event.CaptainID,
				"screen":        domain.ScreenTeamDraft,
			}
		})
}

// draftCompletedHandler tells every drafted player (captains included) which
// team they ended up on.
type draftCompletedHandler struct {
	notifications notificationCreator
	logger        *logger.Logger
}

func (h *draftCompletedHandler) Handle(ctx context.Context, payload []byte) error {
	var event draftCompletedEvent
	if err := decodeEvent(payload, &event); err != nil {
		return err
	}
	ctx = api.WithEventID(ctx, event.EventID.String())

	positions := make(map[uuid.UUID]int, len(event.Teams[0])+len(event.Teams[1]))
	var recipients []uuid.UUID
	for position, roster := range event.Teams {
		for _, userID := range roster {
			positions[userID] = position
			recipients = append(recipients, userID)
		}
	}

	return notifyEach(ctx, h.notifications, h.logger, event.EventID, recipients, uuid.Nil,
		domain.NotificationTypeTeamDraftCompleted,
		func(recipient uuid.UUID) map[string]any {
			return map[string]any{
				"session_id":    event.SessionID,
				"draft_id":      event.DraftID,
				"team_position": positions[recipient],
				"screen":        domain.ScreenSessionTeams,
			}
		})
}

// draftCancelledHandler tells everyone going that the draft stopped without
// result (teams unchanged).
type draftCancelledHandler struct {
	notifications notificationCreator
	logger        *logger.Logger
}

func (h *draftCancelledHandler) Handle(ctx context.Context, payload []byte) error {
	var event draftCancelledEvent
	if err := decodeEvent(payload, &event); err != nil {
		return err
	}
	ctx = api.WithEventID(ctx, event.EventID.String())

	if cancelNotifiedElsewhere(event.Reason) {
		return nil
	}

	return notifyEach(ctx, h.notifications, h.logger, event.EventID, event.ParticipantUserIDs, event.CancelledBy,
		domain.NotificationTypeTeamDraftCancelled,
		func(uuid.UUID) map[string]any {
			return map[string]any{
				"session_id":   event.SessionID,
				"draft_id":     event.DraftID,
				"reason":       event.Reason,
				"cancelled_by": event.CancelledBy,
				"screen":       domain.ScreenSessionTeams,
			}
		})
}

// votingOpenedHandler tells everyone going (but the author) that the first
// proposal of a round is up for a vote.
type votingOpenedHandler struct {
	notifications notificationCreator
	logger        *logger.Logger
}

func (h *votingOpenedHandler) Handle(ctx context.Context, payload []byte) error {
	var event votingOpenedEvent
	if err := decodeEvent(payload, &event); err != nil {
		return err
	}
	ctx = api.WithEventID(ctx, event.EventID.String())

	return notifyEach(ctx, h.notifications, h.logger, event.EventID, event.ParticipantUserIDs, event.AuthorID,
		domain.NotificationTypeTeamVotingOpened,
		func(uuid.UUID) map[string]any {
			return map[string]any{
				"session_id":  event.SessionID,
				"round_id":    event.RoundID,
				"proposal_id": event.ProposalID,
				"author_id":   event.AuthorID,
				"screen":      domain.ScreenTeamVoting,
			}
		})
}

// votingClosedHandler tells everyone going how the vote ended: the winning
// proposal's teams are set, or the current teams were kept.
type votingClosedHandler struct {
	notifications notificationCreator
	logger        *logger.Logger
}

func (h *votingClosedHandler) Handle(ctx context.Context, payload []byte) error {
	var event votingClosedEvent
	if err := decodeEvent(payload, &event); err != nil {
		return err
	}
	ctx = api.WithEventID(ctx, event.EventID.String())

	return notifyEach(ctx, h.notifications, h.logger, event.EventID, event.ParticipantUserIDs, event.ClosedBy,
		domain.NotificationTypeTeamVotingClosed,
		func(uuid.UUID) map[string]any {
			return map[string]any{
				"session_id":         event.SessionID,
				"round_id":           event.RoundID,
				"winner_proposal_id": event.WinnerProposalID,
				"kept_current":       event.KeptCurrent,
				"closed_by":          event.ClosedBy,
				"screen":             domain.ScreenSessionTeams,
			}
		})
}

// votingCancelledHandler tells everyone going that voting was cancelled
// because too few people are going. Other reasons carry no participants
// (teams_replaced) or are covered by session_cancelled (session_ended).
type votingCancelledHandler struct {
	notifications notificationCreator
	logger        *logger.Logger
}

func (h *votingCancelledHandler) Handle(ctx context.Context, payload []byte) error {
	var event votingCancelledEvent
	if err := decodeEvent(payload, &event); err != nil {
		return err
	}
	ctx = api.WithEventID(ctx, event.EventID.String())

	if cancelNotifiedElsewhere(event.Reason) {
		return nil
	}

	return notifyEach(ctx, h.notifications, h.logger, event.EventID, event.ParticipantUserIDs, uuid.Nil,
		domain.NotificationTypeTeamVotingCancelled,
		func(uuid.UUID) map[string]any {
			return map[string]any{
				"session_id": event.SessionID,
				"round_id":   event.RoundID,
				"reason":     event.Reason,
				"screen":     domain.ScreenSessionTeams,
			}
		})
}

// votingTiedHandler tells the session's managers (except the one who pressed
// Close and saw the 409) that the vote is tied and one of them has to pick the
// winner.
type votingTiedHandler struct {
	notifications notificationCreator
	logger        *logger.Logger
}

func (h *votingTiedHandler) Handle(ctx context.Context, payload []byte) error {
	var event votingTiedEvent
	if err := decodeEvent(payload, &event); err != nil {
		return err
	}
	ctx = api.WithEventID(ctx, event.EventID.String())

	return notifyEach(ctx, h.notifications, h.logger, event.EventID, event.ManagerUserIDs, event.ClosedBy,
		domain.NotificationTypeTeamVotingTied,
		func(uuid.UUID) map[string]any {
			return map[string]any{
				"session_id":        event.SessionID,
				"round_id":          event.RoundID,
				"tied_proposal_ids": event.TiedProposalIDs,
				"keep_current_tied": event.KeepCurrentTied,
				"closed_by":         event.ClosedBy,
				"screen":            domain.ScreenTeamVoting,
			}
		})
}

// teamsResetHandler tells everyone still going that team formation was reset:
// the teams are gone (everyone unassigned) and any draft or vote was
// cancelled, so they can start again. Whoever caused it is skipped.
type teamsResetHandler struct {
	notifications notificationCreator
	logger        *logger.Logger
}

func (h *teamsResetHandler) Handle(ctx context.Context, payload []byte) error {
	var event teamsResetEvent
	if err := decodeEvent(payload, &event); err != nil {
		return err
	}
	ctx = api.WithEventID(ctx, event.EventID.String())

	return notifyEach(ctx, h.notifications, h.logger, event.EventID, event.ParticipantUserIDs, event.ResetBy,
		domain.NotificationTypeTeamFormationReset,
		func(uuid.UUID) map[string]any {
			return map[string]any{
				"session_id": event.SessionID,
				"reason":     event.Reason,
				"reset_by":   event.ResetBy,
				"screen":     domain.ScreenSessionTeams,
			}
		})
}

// attendeeTeamChangedHandler tells an attendee an organizer moved them to
// another team. Teams replaced by a draft or vote emit team_assigned instead,
// so this only fires for manual moves.
type attendeeTeamChangedHandler struct {
	notifications notificationCreator
	logger        *logger.Logger
}

func (h *attendeeTeamChangedHandler) Handle(ctx context.Context, payload []byte) error {
	var event attendeeTeamChangedEvent
	if err := decodeEvent(payload, &event); err != nil {
		return err
	}
	ctx = api.WithEventID(ctx, event.EventID.String())

	return notifyEach(ctx, h.notifications, h.logger, event.EventID, []uuid.UUID{event.UserID}, event.ChangedBy,
		domain.NotificationTypeTeamChanged,
		func(uuid.UUID) map[string]any {
			return map[string]any{
				"session_id":       event.SessionID,
				"team_id":          event.TeamID,
				"previous_team_id": event.PreviousTeamID,
				"changed_by":       event.ChangedBy,
				"screen":           domain.ScreenSessionTeams,
			}
		})
}
