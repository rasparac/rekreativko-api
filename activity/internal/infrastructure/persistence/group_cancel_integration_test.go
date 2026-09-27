//go:build integration

package persistence_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/activity/internal/infrastructure/persistence"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
	testutil "github.com/rasparac/rekreativko-api/shared/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (e *sessionTeamTestEnv) createGroup(t *testing.T, creatorID uuid.UUID) *domain.ActivityGroup {
	t.Helper()

	group, err := e.groupSvc.CreateActivityGroup(e.ctx, application.CreateActivityGroupParams{
		CreatorID:       creatorID,
		Title:           "Tuesday Hoops",
		ActivityType:    string(domain.ActivityTypeBasketball),
		DifficultyLevel: string(domain.DifficultyLevelBeginner),
		Visibility:      string(domain.ActivityGroupVisibilityPublic),
		LocationCity:    "Belgrade",
		LocationCountry: "RS",
		Timezone:        "UTC",
	})
	require.NoError(t, err)

	return group
}

// createSessionIn creates a session through the service; groupID nil makes a
// standalone one.
func (e *sessionTeamTestEnv) createSessionIn(t *testing.T, groupID *uuid.UUID, creatorID uuid.UUID) *domain.Session {
	t.Helper()

	session, err := e.sessionSvc.CreateSession(e.ctx, application.CreateSessionParams{
		ActivityGroupID: groupID,
		CreatedByID:     creatorID,
		Title:           "Pickup Game",
		ActivityType:    string(domain.ActivityTypeBasketball),
		DifficultyLevel: string(domain.DifficultyLevelBeginner),
		LocationCity:    "Belgrade",
		LocationCountry: "RS",
		LocationLat:     44.8,
		LocationLng:     20.4,
		StartTime:       time.Now().Add(time.Hour),
		Visibility:      string(domain.SessionVisibilityPublic),
	})
	require.NoError(t, err)

	return session
}

func (e *sessionTeamTestEnv) sessionStatus(t *testing.T, session *domain.Session) domain.SessionStatus {
	t.Helper()

	stored, err := e.sessionRepo.GetSessionByID(e.ctx, session.ID())
	require.NoError(t, err)

	return stored.Status()
}

func TestActivityGroupService_CancelCascadesToItsSessions(t *testing.T) {
	env := setupSessionTeamTest(t)
	creator := uuid.New()
	group := env.createGroup(t, creator)
	groupID := group.ID()

	scheduled := env.createSessionIn(t, &groupID, creator)

	started := env.createSessionIn(t, &groupID, creator)
	require.NoError(t, env.sessionSvc.StartSession(env.ctx, started.ID(), creator, string(domain.MemberRoleCreator)))

	completed := env.createSessionIn(t, &groupID, creator)
	require.NoError(t, env.sessionSvc.StartSession(env.ctx, completed.ID(), creator, string(domain.MemberRoleCreator)))
	require.NoError(t, env.sessionSvc.CompleteSession(env.ctx, completed.ID(), creator, string(domain.MemberRoleCreator)))

	// A running draft on one of the group's sessions.
	drafted := env.createSessionIn(t, &groupID, creator)
	players := make([]uuid.UUID, 4)
	for i := range players {
		players[i] = env.addGoingAttendee(t, drafted)
	}
	_, err := env.startDraft(drafted, players[0], players[1], nil)
	require.NoError(t, err)

	// The same creator's standalone session has nothing to do with the group.
	standalone := env.createSessionIn(t, nil, creator)

	require.NoError(t, env.groupSvc.CancelActivityGroup(env.ctx, groupID, creator, "season over"))

	assert.Equal(t, domain.SessionStatusCanceled, env.sessionStatus(t, scheduled))
	assert.Equal(t, domain.SessionStatusCanceled, env.sessionStatus(t, started))
	assert.Equal(t, domain.SessionStatusCanceled, env.sessionStatus(t, drafted))
	assert.Equal(t, domain.SessionStatusCompleted, env.sessionStatus(t, completed), "an ended session stays as it was")
	assert.Equal(t, domain.SessionStatusScheduled, env.sessionStatus(t, standalone), "sessions outside the group are untouched")

	draft, err := env.draftSvc.GetDraft(env.ctx, drafted.ID())
	require.NoError(t, err)
	assert.Equal(t, domain.DraftStatusCancelled, draft.Draft.Status())
	assert.Equal(t, domain.DraftCancelReasonSessionEnded, draft.Draft.CancelReason())
}

func TestActivityGroupService_CancelRejectedLeavesSessionsAlone(t *testing.T) {
	env := setupSessionTeamTest(t)
	creator := uuid.New()
	group := env.createGroup(t, creator)
	groupID := group.ID()
	session := env.createSessionIn(t, &groupID, creator)

	// Only the group creator may cancel it; nothing may be cancelled otherwise.
	err := env.groupSvc.CancelActivityGroup(env.ctx, groupID, uuid.New(), "nope")
	require.Error(t, err)

	assert.Equal(t, domain.SessionStatusScheduled, env.sessionStatus(t, session))
}

func TestActivityGroupService_DeleteCancelsAndDeletesItsSessions(t *testing.T) {
	env := setupSessionTeamTest(t)
	creator := uuid.New()
	group := env.createGroup(t, creator)
	groupID := group.ID()

	scheduled := env.createSessionIn(t, &groupID, creator)
	attendee := env.addGoingAttendee(t, scheduled)

	completed := env.createSessionIn(t, &groupID, creator)
	require.NoError(t, env.sessionSvc.StartSession(env.ctx, completed.ID(), creator, string(domain.MemberRoleCreator)))
	require.NoError(t, env.sessionSvc.CompleteSession(env.ctx, completed.ID(), creator, string(domain.MemberRoleCreator)))

	drafted := env.createSessionIn(t, &groupID, creator)
	players := make([]uuid.UUID, 4)
	for i := range players {
		players[i] = env.addGoingAttendee(t, drafted)
	}
	_, err := env.startDraft(drafted, players[0], players[1], nil)
	require.NoError(t, err)

	standalone := env.createSessionIn(t, nil, creator)

	require.NoError(t, env.groupSvc.DeleteActivityGroup(env.ctx, groupID, creator))

	// Every session of the group is gone - live and ended alike.
	for _, session := range []*domain.Session{scheduled, completed, drafted} {
		_, err := env.sessionSvc.GetSession(env.ctx, session.ID(), creator)
		requireAppErrorCode(t, err, "session_not_found")
	}

	listed, _, _, err := env.sessionSvc.ListSessions(env.ctx, application.ListSessionsParams{Limit: 100}, creator)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Equal(t, standalone.ID(), listed[0].ID(), "only the standalone session is left")

	joined, _, _, err := env.sessionSvc.ListSessions(env.ctx, application.ListSessionsParams{AttendeeID: &attendee, Limit: 100}, attendee)
	require.NoError(t, err)
	assert.Empty(t, joined, "a deleted session drops out of the attendee's sessions too")

	// Live sessions were cancelled on the way out, so attendees were notified.
	assert.Equal(t, 2, env.outboxCount(t, "activity.session.cancelled"))
	assert.Equal(t, 1, env.outboxCount(t, "activity.session.draft.cancelled"))
}

// outboxCount counts the activity outbox events of one type.
func (e *sessionTeamTestEnv) outboxCount(t *testing.T, eventType string) int {
	t.Helper()

	var n int
	require.NoError(t, e.txManager.Querier(e.ctx).QueryRow(e.ctx,
		`SELECT count(*) FROM activity.event_outbox WHERE event_type = $1`, eventType,
	).Scan(&n))

	return n
}

func TestSessionGenerator_SkipsCancelledAndDeletedGroups(t *testing.T) {
	env := setupSessionTeamTest(t)
	logger := testutil.CreateLogger()
	templateRepo := persistence.NewSessionTemplateManager(env.txManager, logger)
	generator := application.NewSessionGeneratorService(
		logger, env.txManager, templateRepo, env.sessionRepo,
		persistence.NewActivityGroupRepository(env.txManager, logger),
		domainevent.NewDomainEventManager(env.txManager), testMetrics(), 14*24*time.Hour,
	)

	weeklyTemplate := func(groupID, creatorID uuid.UUID) *domain.SessionTemplate {
		template := persistence.NewSessionTemplateBuilder().
			WithActivityGroup(groupID).
			WithCreator(creatorID).
			WithTitle("Tuesday Hoops").
			WithWeeklyRecurrence(time.Tuesday, 19, 0).
			Build()
		require.NoError(t, templateRepo.CreateSessionTemplate(env.ctx, template))
		return template
	}

	creator := uuid.New()

	active := env.createGroup(t, creator)
	activeTemplate := weeklyTemplate(active.ID(), creator)

	cancelled := env.createGroup(t, creator)
	cancelledTemplate := weeklyTemplate(cancelled.ID(), creator)
	require.NoError(t, env.groupSvc.CancelActivityGroup(env.ctx, cancelled.ID(), creator, "season over"))

	deleted := env.createGroup(t, creator)
	deletedTemplate := weeklyTemplate(deleted.ID(), creator)
	require.NoError(t, env.groupSvc.DeleteActivityGroup(env.ctx, deleted.ID(), creator))

	generated, err := generator.GenerateSessionsForTemplate(env.ctx, activeTemplate.ID())
	require.NoError(t, err)
	assert.Positive(t, generated, "an active group's template still generates sessions")

	generated, err = generator.GenerateSessionsForTemplate(env.ctx, cancelledTemplate.ID())
	require.NoError(t, err)
	assert.Zero(t, generated, "a cancelled group gets no new sessions")

	generated, err = generator.GenerateSessionsForTemplate(env.ctx, deletedTemplate.ID())
	require.NoError(t, err)
	assert.Zero(t, generated, "a deleted group gets no new sessions")
}
