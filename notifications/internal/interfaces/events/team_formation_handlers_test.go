package events

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/notifications/internal/application"
	"github.com/rasparac/rekreativko-api/notifications/internal/domain"
	"github.com/rasparac/rekreativko-api/shared/events"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type eventHandler interface {
	Handle(ctx context.Context, payload []byte) error
}

// teamPayload builds the wire shape activity publishes - BaseEvent fields
// plus the event's own fields - so the tests catch a drift between the
// upstream JSON tags and our local copies.
func teamPayload(t *testing.T, eventID uuid.UUID, eventType string, fields map[string]any) []byte {
	t.Helper()

	body := map[string]any{
		"event_id":     eventID,
		"event_type":   eventType,
		"occurred_at":  time.Now().UTC(),
		"aggregate_id": uuid.New(),
		"version":      3,
	}
	for k, v := range fields {
		body[k] = v
	}

	payload, err := json.Marshal(body)
	require.NoError(t, err)

	return payload
}

func handle(t *testing.T, h eventHandler, payload []byte) {
	t.Helper()
	require.NoError(t, h.Handle(context.Background(), payload))
}

func recipients(calls []application.CreateNotificationParams) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(calls))
	for _, c := range calls {
		out = append(out, c.RecipientAccountID)
	}
	return out
}

func requireTeamNotification(t *testing.T, call application.CreateNotificationParams, eventID, sessionID uuid.UUID, typ domain.NotificationType, screen domain.Screen) {
	t.Helper()
	assert.Equal(t, eventID, call.EventID)
	assert.Equal(t, string(typ), call.Type)
	assert.Equal(t, sessionID, call.Data["session_id"])
	assert.Equal(t, screen, call.Data["screen"])
}

func TestDraftStartedHandler_NotifiesBothCaptains(t *testing.T) {
	creator := &recordingCreator{}
	h := &draftStartedHandler{notifications: creator, logger: logger.New("error", "json")}
	eventID, sessionID, draftID := uuid.New(), uuid.New(), uuid.New()
	captainA, captainB, organizer := uuid.New(), uuid.New(), uuid.New()

	handle(t, h, teamPayload(t, eventID, "activity.session.draft.started", map[string]any{
		"session_id":  sessionID,
		"draft_id":    draftID,
		"captain_ids": []uuid.UUID{captainA, captainB},
		"pick_order":  "snake",
		"started_by":  organizer,
	}))

	require.Equal(t, []uuid.UUID{captainA, captainB}, recipients(creator.calls))
	for i, call := range creator.calls {
		requireTeamNotification(t, call, eventID, sessionID, domain.NotificationTypeTeamDraftCaptainSelected, domain.ScreenTeamDraft)
		assert.Equal(t, i, call.Data["team_position"])
		assert.Equal(t, draftID, call.Data["draft_id"])
	}
}

func TestDraftStartedHandler_SkipsOrganizerWhoIsCaptain(t *testing.T) {
	creator := &recordingCreator{}
	h := &draftStartedHandler{notifications: creator, logger: logger.New("error", "json")}
	organizer, other := uuid.New(), uuid.New()

	handle(t, h, teamPayload(t, uuid.New(), "activity.session.draft.started", map[string]any{
		"session_id":  uuid.New(),
		"captain_ids": []uuid.UUID{organizer, other},
		"started_by":  organizer,
	}))

	require.Equal(t, []uuid.UUID{other}, recipients(creator.calls))
	assert.Equal(t, 1, creator.calls[0].Data["team_position"])
}

func TestDraftTurnChangedHandler_NotifiesCaptain(t *testing.T) {
	creator := &recordingCreator{}
	h := &draftTurnChangedHandler{notifications: creator, logger: logger.New("error", "json")}
	eventID, sessionID, captain := uuid.New(), uuid.New(), uuid.New()

	handle(t, h, teamPayload(t, eventID, "activity.session.draft.turn_changed", map[string]any{
		"session_id":    sessionID,
		"draft_id":      uuid.New(),
		"captain_id":    captain,
		"team_position": 1,
		"pick_number":   4,
	}))

	require.Equal(t, []uuid.UUID{captain}, recipients(creator.calls))
	requireTeamNotification(t, creator.calls[0], eventID, sessionID, domain.NotificationTypeTeamDraftYourTurn, domain.ScreenTeamDraft)
	assert.Equal(t, 1, creator.calls[0].Data["team_position"])
	assert.Equal(t, 4, creator.calls[0].Data["pick_number"])
}

func TestDraftPausedHandler_NotifiesOrganizerOnly(t *testing.T) {
	creator := &recordingCreator{}
	h := &draftPausedHandler{notifications: creator, logger: logger.New("error", "json")}
	eventID, sessionID, organizer, leaver := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	handle(t, h, teamPayload(t, eventID, "activity.session.draft.paused", map[string]any{
		"session_id":    sessionID,
		"draft_id":      uuid.New(),
		"reason":        "captain_left",
		"team_position": 0,
		"captain_id":    leaver,
		"started_by":    organizer,
	}))

	require.Equal(t, []uuid.UUID{organizer}, recipients(creator.calls))
	requireTeamNotification(t, creator.calls[0], eventID, sessionID, domain.NotificationTypeTeamDraftPaused, domain.ScreenTeamDraft)
	assert.Equal(t, leaver, creator.calls[0].Data["captain_id"])
	assert.Equal(t, "captain_left", creator.calls[0].Data["reason"])
}

func TestDraftCompletedHandler_NotifiesEveryPlayerWithTheirTeam(t *testing.T) {
	creator := &recordingCreator{}
	h := &draftCompletedHandler{notifications: creator, logger: logger.New("error", "json")}
	eventID, sessionID := uuid.New(), uuid.New()
	teamA := []uuid.UUID{uuid.New(), uuid.New()}
	teamB := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}

	handle(t, h, teamPayload(t, eventID, "activity.session.draft.completed", map[string]any{
		"session_id": sessionID,
		"draft_id":   uuid.New(),
		"teams":      [][]uuid.UUID{teamA, teamB},
	}))

	require.Equal(t, append(append([]uuid.UUID{}, teamA...), teamB...), recipients(creator.calls))
	for _, call := range creator.calls {
		requireTeamNotification(t, call, eventID, sessionID, domain.NotificationTypeTeamDraftCompleted, domain.ScreenSessionTeams)
		want := 0
		if call.RecipientAccountID != teamA[0] && call.RecipientAccountID != teamA[1] {
			want = 1
		}
		assert.Equal(t, want, call.Data["team_position"])
	}
}

func TestDraftCancelledHandler(t *testing.T) {
	t.Run("notifies everyone going but the organizer who cancelled", func(t *testing.T) {
		creator := &recordingCreator{}
		h := &draftCancelledHandler{notifications: creator, logger: logger.New("error", "json")}
		eventID, sessionID, organizer := uuid.New(), uuid.New(), uuid.New()
		others := []uuid.UUID{uuid.New(), uuid.New()}

		handle(t, h, teamPayload(t, eventID, "activity.session.draft.cancelled", map[string]any{
			"session_id":           sessionID,
			"draft_id":             uuid.New(),
			"reason":               "organizer",
			"cancelled_by":         organizer,
			"participant_user_ids": []uuid.UUID{others[0], organizer, others[1]},
		}))

		require.Equal(t, others, recipients(creator.calls))
		requireTeamNotification(t, creator.calls[0], eventID, sessionID, domain.NotificationTypeTeamDraftCancelled, domain.ScreenSessionTeams)
		assert.Equal(t, "organizer", creator.calls[0].Data["reason"])
	})

	t.Run("not enough players notifies everyone still going", func(t *testing.T) {
		creator := &recordingCreator{}
		h := &draftCancelledHandler{notifications: creator, logger: logger.New("error", "json")}
		going := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}

		handle(t, h, teamPayload(t, uuid.New(), "activity.session.draft.cancelled", map[string]any{
			"session_id":           uuid.New(),
			"reason":               "not_enough_players",
			"cancelled_by":         uuid.Nil,
			"participant_user_ids": going,
		}))

		assert.Equal(t, going, recipients(creator.calls))
	})

	t.Run("session ended is left to session_cancelled", func(t *testing.T) {
		creator := &recordingCreator{}
		h := &draftCancelledHandler{notifications: creator, logger: logger.New("error", "json")}

		handle(t, h, teamPayload(t, uuid.New(), "activity.session.draft.cancelled", map[string]any{
			"session_id":           uuid.New(),
			"reason":               "session_ended",
			"participant_user_ids": []uuid.UUID{uuid.New()},
		}))

		assert.Empty(t, creator.calls)
	})
}

func TestVotingOpenedHandler_NotifiesEveryoneButTheAuthor(t *testing.T) {
	creator := &recordingCreator{}
	h := &votingOpenedHandler{notifications: creator, logger: logger.New("error", "json")}
	eventID, sessionID, proposalID, author := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	voters := []uuid.UUID{uuid.New(), uuid.New()}

	handle(t, h, teamPayload(t, eventID, "activity.session.voting.opened", map[string]any{
		"session_id":           sessionID,
		"round_id":             uuid.New(),
		"proposal_id":          proposalID,
		"author_id":            author,
		"team_count":           2,
		"participant_user_ids": []uuid.UUID{author, voters[0], voters[1]},
	}))

	require.Equal(t, voters, recipients(creator.calls))
	requireTeamNotification(t, creator.calls[0], eventID, sessionID, domain.NotificationTypeTeamVotingOpened, domain.ScreenTeamVoting)
	assert.Equal(t, proposalID, creator.calls[0].Data["proposal_id"])
}

func TestVotingClosedHandler_NotifiesEveryoneButTheCloser(t *testing.T) {
	creator := &recordingCreator{}
	h := &votingClosedHandler{notifications: creator, logger: logger.New("error", "json")}
	eventID, sessionID, winner, organizer := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	going := []uuid.UUID{uuid.New(), uuid.New()}

	handle(t, h, teamPayload(t, eventID, "activity.session.voting.closed", map[string]any{
		"session_id":           sessionID,
		"round_id":             uuid.New(),
		"winner_proposal_id":   winner,
		"kept_current":         false,
		"closed_by":            organizer,
		"participant_user_ids": append([]uuid.UUID{organizer}, going...),
	}))

	require.Equal(t, going, recipients(creator.calls))
	requireTeamNotification(t, creator.calls[0], eventID, sessionID, domain.NotificationTypeTeamVotingClosed, domain.ScreenSessionTeams)
	assert.Equal(t, &winner, creator.calls[0].Data["winner_proposal_id"])
	assert.Equal(t, false, creator.calls[0].Data["kept_current"])
}

func TestVotingCancelledHandler(t *testing.T) {
	t.Run("not enough players notifies everyone still going", func(t *testing.T) {
		creator := &recordingCreator{}
		h := &votingCancelledHandler{notifications: creator, logger: logger.New("error", "json")}
		eventID, sessionID := uuid.New(), uuid.New()
		going := []uuid.UUID{uuid.New(), uuid.New()}

		handle(t, h, teamPayload(t, eventID, "activity.session.voting.cancelled", map[string]any{
			"session_id":           sessionID,
			"round_id":             uuid.New(),
			"reason":               "not_enough_players",
			"participant_user_ids": going,
		}))

		require.Equal(t, going, recipients(creator.calls))
		requireTeamNotification(t, creator.calls[0], eventID, sessionID, domain.NotificationTypeTeamVotingCancelled, domain.ScreenSessionTeams)
		assert.Equal(t, "not_enough_players", creator.calls[0].Data["reason"])
	})

	t.Run("session ended is left to session_cancelled", func(t *testing.T) {
		creator := &recordingCreator{}
		h := &votingCancelledHandler{notifications: creator, logger: logger.New("error", "json")}

		handle(t, h, teamPayload(t, uuid.New(), "activity.session.voting.cancelled", map[string]any{
			"session_id":           uuid.New(),
			"reason":               "session_ended",
			"participant_user_ids": []uuid.UUID{uuid.New()},
		}))

		assert.Empty(t, creator.calls)
	})
}

func TestVotingTiedHandler_NotifiesManagersButNotTheCloser(t *testing.T) {
	creator := &recordingCreator{}
	h := &votingTiedHandler{notifications: creator, logger: logger.New("error", "json")}
	eventID, sessionID, closer := uuid.New(), uuid.New(), uuid.New()
	managers := []uuid.UUID{uuid.New(), uuid.New()}
	tied := []uuid.UUID{uuid.New(), uuid.New()}

	handle(t, h, teamPayload(t, eventID, "activity.session.voting.tied", map[string]any{
		"session_id":        sessionID,
		"round_id":          uuid.New(),
		"tied_proposal_ids": tied,
		"keep_current_tied": true,
		"closed_by":         closer,
		"manager_user_ids":  append([]uuid.UUID{closer}, managers...),
	}))

	require.Equal(t, managers, recipients(creator.calls))
	requireTeamNotification(t, creator.calls[0], eventID, sessionID, domain.NotificationTypeTeamVotingTied, domain.ScreenTeamVoting)
	assert.Equal(t, tied, creator.calls[0].Data["tied_proposal_ids"])
	assert.Equal(t, true, creator.calls[0].Data["keep_current_tied"])
}

func TestTeamsResetHandler_NotifiesEveryoneGoingExceptWhoCausedIt(t *testing.T) {
	creator := &recordingCreator{}
	h := &teamsResetHandler{notifications: creator, logger: logger.New("error", "json")}
	eventID, sessionID, organizer := uuid.New(), uuid.New(), uuid.New()
	going := []uuid.UUID{uuid.New(), uuid.New()}

	handle(t, h, teamPayload(t, eventID, "activity.session.teams_reset", map[string]any{
		"session_id":           sessionID,
		"reason":               "organizer",
		"reset_by":             organizer,
		"participant_user_ids": append([]uuid.UUID{organizer}, going...),
	}))

	require.Equal(t, going, recipients(creator.calls))
	requireTeamNotification(t, creator.calls[0], eventID, sessionID, domain.NotificationTypeTeamFormationReset, domain.ScreenSessionTeams)
	assert.Equal(t, "organizer", creator.calls[0].Data["reason"])
}

// The reset tells everyone going once; the draft and vote it cancelled must not
// each notify them again.
func TestCancelHandlers_LeaveResetCancellationsToTheResetNotification(t *testing.T) {
	for _, reason := range []string{"roster_changed", "teams_reset"} {
		t.Run(reason, func(t *testing.T) {
			creator := &recordingCreator{}
			log := logger.New("error", "json")
			going := []uuid.UUID{uuid.New(), uuid.New()}
			fields := map[string]any{
				"session_id":           uuid.New(),
				"reason":               reason,
				"participant_user_ids": going,
			}

			handle(t, &draftCancelledHandler{notifications: creator, logger: log},
				teamPayload(t, uuid.New(), "activity.session.draft.cancelled", fields))
			handle(t, &votingCancelledHandler{notifications: creator, logger: log},
				teamPayload(t, uuid.New(), "activity.session.voting.cancelled", fields))

			assert.Empty(t, creator.calls)
		})
	}
}

func TestAttendeeTeamChangedHandler_NotifiesMovedAttendee(t *testing.T) {
	creator := &recordingCreator{}
	h := &attendeeTeamChangedHandler{notifications: creator, logger: logger.New("error", "json")}
	eventID, sessionID, user, organizer, teamID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()

	handle(t, h, teamPayload(t, eventID, "activity.session.attendee.team_changed", map[string]any{
		"session_id":       sessionID,
		"user_id":          user,
		"team_id":          teamID,
		"previous_team_id": uuid.New(),
		"changed_by":       organizer,
	}))

	require.Equal(t, []uuid.UUID{user}, recipients(creator.calls))
	requireTeamNotification(t, creator.calls[0], eventID, sessionID, domain.NotificationTypeTeamChanged, domain.ScreenSessionTeams)
	assert.Equal(t, teamID, creator.calls[0].Data["team_id"])
}

func TestNotifyEach_SkipsNilAndDuplicateRecipients(t *testing.T) {
	creator := &recordingCreator{}
	user := uuid.New()

	err := notifyEach(context.Background(), creator, logger.New("error", "json"), uuid.New(),
		[]uuid.UUID{uuid.Nil, user, user}, uuid.Nil, domain.NotificationTypeTeamDraftCancelled,
		func(uuid.UUID) map[string]any { return nil })

	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{user}, recipients(creator.calls))
}

func TestTeamFormationHandlers_MalformedPayloadIsPermanent(t *testing.T) {
	log := logger.New("error", "json")
	handlers := map[string]eventHandler{
		"draft_started":      &draftStartedHandler{notifications: &recordingCreator{}, logger: log},
		"draft_turn_changed": &draftTurnChangedHandler{notifications: &recordingCreator{}, logger: log},
		"draft_paused":       &draftPausedHandler{notifications: &recordingCreator{}, logger: log},
		"draft_completed":    &draftCompletedHandler{notifications: &recordingCreator{}, logger: log},
		"draft_cancelled":    &draftCancelledHandler{notifications: &recordingCreator{}, logger: log},
		"voting_opened":      &votingOpenedHandler{notifications: &recordingCreator{}, logger: log},
		"voting_closed":      &votingClosedHandler{notifications: &recordingCreator{}, logger: log},
		"voting_cancelled":   &votingCancelledHandler{notifications: &recordingCreator{}, logger: log},
		"voting_tied":        &votingTiedHandler{notifications: &recordingCreator{}, logger: log},
		"teams_reset":        &teamsResetHandler{notifications: &recordingCreator{}, logger: log},
		"team_changed":       &attendeeTeamChangedHandler{notifications: &recordingCreator{}, logger: log},
	}

	for name, h := range handlers {
		t.Run(name, func(t *testing.T) {
			err := h.Handle(context.Background(), []byte("{not json"))
			require.Error(t, err)
			assert.ErrorIs(t, err, events.ErrPermanent, "a malformed payload must not be retried")
		})
	}
}

type failingCreator struct{ err error }

func (f failingCreator) CreateNotification(context.Context, application.CreateNotificationParams) (*domain.Notification, error) {
	return nil, f.err
}

func TestNotifyEach_ReturnsCreateErrorForRetry(t *testing.T) {
	boom := errors.New("db down")

	err := notifyEach(context.Background(), failingCreator{err: boom}, logger.New("error", "json"), uuid.New(),
		[]uuid.UUID{uuid.New()}, uuid.Nil, domain.NotificationTypeTeamDraftCancelled,
		func(uuid.UUID) map[string]any { return nil })

	assert.ErrorIs(t, err, boom)
}
