//go:build integration

package persistence_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/activity/internal/infrastructure/persistence"
	"github.com/rasparac/rekreativko-api/shared/domainerror"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
	testutil "github.com/rasparac/rekreativko-api/shared/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type sessionInviteTestEnv struct {
	ctx          context.Context
	svc          *application.SessionInviteService
	inviteRepo   persistence.SessionInviteRepository
	sessionRepo  application.SessionRepository
	attendeeRepo application.AttendeeRepository
}

func setupSessionInviteTest(t *testing.T) *sessionInviteTestEnv {
	t.Helper()

	db := setupCapacityRaceTestDB(t)
	txManager := db.CreateTransactionManager()
	logger := testutil.CreateLogger()

	inviteRepo := persistence.NewSessionInviteRepository(txManager, logger)
	sessionRepo := persistence.NewSessionManager(txManager, logger)
	attendeeRepo := persistence.NewAttendeeRepository(txManager, logger)
	eventWriter := domainevent.NewDomainEventManager(txManager)

	return &sessionInviteTestEnv{
		ctx:          context.Background(),
		svc:          application.NewSessionInviteService(logger, txManager, inviteRepo, sessionRepo, attendeeRepo, eventWriter, testMetrics()),
		inviteRepo:   inviteRepo,
		sessionRepo:  sessionRepo,
		attendeeRepo: attendeeRepo,
	}
}

// createPrivateStandaloneSession creates the exact kind of session this
// feature exists for: no group, private, so nobody but the creator could
// ever attend without an invite.
func (e *sessionInviteTestEnv) createPrivateStandaloneSession(t *testing.T, capacity int) *domain.Session {
	t.Helper()

	title, err := domain.NewTitle("Private Standalone Session")
	require.NoError(t, err)

	location, err := domain.NewSessionLocation("Belgrade", "RS", "", 44.8, 20.4)
	require.NoError(t, err)

	schedule, err := domain.NewSessionSchedule(time.Now().Add(time.Hour), nil)
	require.NoError(t, err)

	// Mirror SessionService.CreateSession: the creator auto-attends their own
	// session, so they hold a slot from the start.
	creatorID := uuid.New()
	visibility := domain.SessionVisibilityPrivate
	session, attendees, err := domain.NewSession(domain.SessionInput{
		CreatedByID:     creatorID,
		AutoAttendeeIDs: []uuid.UUID{creatorID},
		Title:           title,
		ActivityType:    domain.ActivityTypeRunning,
		DifficultyLevel: domain.DifficultyLevelBeginner,
		Location:        location,
		Schedule:        schedule,
		Capacity:        testutil.Ptr(capacity),
		Visibility:      &visibility,
	})
	require.NoError(t, err)

	require.NoError(t, e.sessionRepo.CreateSession(e.ctx, session))
	for _, attendee := range attendees {
		require.NoError(t, e.attendeeRepo.CreateAttendee(e.ctx, attendee))
	}

	return session
}

func (e *sessionInviteTestEnv) invite(t *testing.T, session *domain.Session, invitedUserID uuid.UUID) *domain.SessionInvite {
	t.Helper()

	invite, err := e.svc.SendInvite(e.ctx, application.SendSessionInviteParams{
		SessionID:     session.ID(),
		RequesterID:   session.CreatedByID(),
		InvitedUserID: invitedUserID,
	})
	require.NoError(t, err)

	return invite
}

func requireAppErrorCode(t *testing.T, err error, code string) {
	t.Helper()

	var appErr *domainerror.AppError
	require.True(t, errors.As(err, &appErr), "expected an AppError, got %v", err)
	assert.Equal(t, code, appErr.Code)
}

func TestSessionInviteService_InviteAcceptFlow_MakesInviteeAnAttendee(t *testing.T) {
	env := setupSessionInviteTest(t)
	session := env.createPrivateStandaloneSession(t, 5)
	invitee := uuid.New()

	invite := env.invite(t, session, invitee)

	// Before accepting, the invitee is not an attendee - consent comes first.
	_, err := env.attendeeRepo.GetAttendeeBySessionAndUser(env.ctx, session.ID(), invitee)
	require.ErrorIs(t, err, domain.ErrAttendeeNotFound)

	listed, _, err := env.svc.ListMyInvites(env.ctx, invitee, application.ListMyInvitesParams{Limit: 20})
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Equal(t, invite.ID(), listed[0].ID())

	attendee, err := env.svc.AcceptInvite(env.ctx, invite.ID(), invitee)
	require.NoError(t, err)
	assert.Equal(t, domain.AttendeeStatusGoing, attendee.Status())
	assert.Equal(t, domain.AttendeeSourceInvited, attendee.Source())

	stored, err := env.attendeeRepo.GetAttendeeBySessionAndUser(env.ctx, session.ID(), invitee)
	require.NoError(t, err)
	assert.Equal(t, domain.AttendeeStatusGoing, stored.Status())
	assert.Nil(t, stored.ActivityID())

	storedInvite, err := env.inviteRepo.GetInviteByID(env.ctx, invite.ID())
	require.NoError(t, err)
	assert.Equal(t, domain.InviteStatusAccepted, storedInvite.Status())

	// No longer pending, so no longer listed.
	listed, _, err = env.svc.ListMyInvites(env.ctx, invitee, application.ListMyInvitesParams{Limit: 20})
	require.NoError(t, err)
	assert.Empty(t, listed)

	// Accepting twice fails.
	_, err = env.svc.AcceptInvite(env.ctx, invite.ID(), invitee)
	requireAppErrorCode(t, err, "invite_already_processed")
}

func TestSessionInviteService_Decline_CreatesNoAttendee(t *testing.T) {
	env := setupSessionInviteTest(t)
	session := env.createPrivateStandaloneSession(t, 5)
	invitee := uuid.New()

	invite := env.invite(t, session, invitee)
	require.NoError(t, env.svc.DeclineInvite(env.ctx, invite.ID(), invitee))

	_, err := env.attendeeRepo.GetAttendeeBySessionAndUser(env.ctx, session.ID(), invitee)
	require.ErrorIs(t, err, domain.ErrAttendeeNotFound)

	// A declined invite no longer blocks a fresh one.
	env.invite(t, session, invitee)
}

func TestSessionInviteService_OnlyInviteeCanRespond(t *testing.T) {
	env := setupSessionInviteTest(t)
	session := env.createPrivateStandaloneSession(t, 5)

	invite := env.invite(t, session, uuid.New())

	_, err := env.svc.AcceptInvite(env.ctx, invite.ID(), uuid.New())
	requireAppErrorCode(t, err, "unauthorized")

	err = env.svc.DeclineInvite(env.ctx, invite.ID(), uuid.New())
	requireAppErrorCode(t, err, "unauthorized")
}

func TestSessionInviteService_SendInvite_Rejections(t *testing.T) {
	env := setupSessionInviteTest(t)
	session := env.createPrivateStandaloneSession(t, 5)
	invitee := uuid.New()

	// Only the creator can invite.
	_, err := env.svc.SendInvite(env.ctx, application.SendSessionInviteParams{
		SessionID:     session.ID(),
		RequesterID:   uuid.New(),
		InvitedUserID: invitee,
	})
	requireAppErrorCode(t, err, "unauthorized")

	// Inviting the creator (already attending) is rejected.
	_, err = env.svc.SendInvite(env.ctx, application.SendSessionInviteParams{
		SessionID:     session.ID(),
		RequesterID:   session.CreatedByID(),
		InvitedUserID: session.CreatedByID(),
	})
	requireAppErrorCode(t, err, "cannot_invite_self")

	// A second pending invite for the same user is rejected.
	env.invite(t, session, invitee)
	_, err = env.svc.SendInvite(env.ctx, application.SendSessionInviteParams{
		SessionID:     session.ID(),
		RequesterID:   session.CreatedByID(),
		InvitedUserID: invitee,
	})
	requireAppErrorCode(t, err, "already_invited")

	// Someone who is already attending can't be invited.
	attending := uuid.New()
	attendee, err := domain.NewRSVPManualAttendee(session, nil, attending, domain.AttendeeStatusGoing, 0)
	require.NoError(t, err)
	require.NoError(t, env.attendeeRepo.CreateAttendee(env.ctx, attendee))

	_, err = env.svc.SendInvite(env.ctx, application.SendSessionInviteParams{
		SessionID:     session.ID(),
		RequesterID:   session.CreatedByID(),
		InvitedUserID: attending,
	})
	requireAppErrorCode(t, err, "attendee_already_attending")
}

func TestSessionInviteService_SendInvite_UnknownSession_NotFound(t *testing.T) {
	env := setupSessionInviteTest(t)

	_, err := env.svc.SendInvite(env.ctx, application.SendSessionInviteParams{
		SessionID:     uuid.New(),
		RequesterID:   uuid.New(),
		InvitedUserID: uuid.New(),
	})
	requireAppErrorCode(t, err, "session_not_found")
}

func TestSessionInviteService_AcceptOnFullSession_GoesToWaitlist(t *testing.T) {
	env := setupSessionInviteTest(t)
	// Capacity 1 is taken by the creator (auto-confirmed on creation).
	session := env.createPrivateStandaloneSession(t, 1)
	invitee := uuid.New()

	invite := env.invite(t, session, invitee)
	attendee, err := env.svc.AcceptInvite(env.ctx, invite.ID(), invitee)
	require.NoError(t, err)

	assert.Equal(t, domain.AttendeeStatusPending, attendee.Status())
}

// Two invitees accepting concurrently for the last remaining slot must never
// both end up confirmed - accepting takes the same session capacity lock as
// RSVPs and approvals.
func TestSessionInviteService_ConcurrentAccept_DoesNotOverbook(t *testing.T) {
	env := setupSessionInviteTest(t)
	// Capacity 2: the creator holds one slot, leaving exactly one.
	session := env.createPrivateStandaloneSession(t, 2)

	userA, userB := uuid.New(), uuid.New()
	inviteA := env.invite(t, session, userA)
	inviteB := env.invite(t, session, userB)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, errs[0] = env.svc.AcceptInvite(env.ctx, inviteA.ID(), userA)
	}()
	go func() {
		defer wg.Done()
		_, errs[1] = env.svc.AcceptInvite(env.ctx, inviteB.ID(), userB)
	}()
	wg.Wait()

	require.NoError(t, errs[0])
	require.NoError(t, errs[1])

	confirmed, err := env.attendeeRepo.CountConfirmedAttendees(env.ctx, session.ID())
	require.NoError(t, err)
	assert.Equal(t, 2, confirmed, "creator + exactly one accepted invitee")
}

func TestSessionInviteService_ExpireStaleInvites(t *testing.T) {
	env := setupSessionInviteTest(t)
	session := env.createPrivateStandaloneSession(t, 5)
	invitee := uuid.New()

	// Persist an invite that is already past its expiry.
	stale := domain.ReconstructSessionInvite(
		uuid.New(), session.ID(), invitee, session.CreatedByID(),
		domain.InviteStatusPending,
		time.Now().Add(-8*24*time.Hour), time.Now().Add(-time.Hour), nil,
	)
	require.NoError(t, env.inviteRepo.CreateInvite(env.ctx, stale))
	env.invite(t, session, uuid.New()) // a fresh one that must survive the sweep

	// Expired-but-still-pending invites aren't offered to the invitee...
	listed, _, err := env.svc.ListMyInvites(env.ctx, invitee, application.ListMyInvitesParams{Limit: 20})
	require.NoError(t, err)
	assert.Empty(t, listed)

	// ...and accepting one fails.
	_, err = env.svc.AcceptInvite(env.ctx, stale.ID(), invitee)
	requireAppErrorCode(t, err, "invite_expired")

	count, err := env.svc.ExpireStaleInvites(env.ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	stored, err := env.inviteRepo.GetInviteByID(env.ctx, stale.ID())
	require.NoError(t, err)
	assert.Equal(t, domain.InviteStatusExpired, stored.Status())
}
