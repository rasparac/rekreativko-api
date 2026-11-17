//go:build integration

package persistence_test

import (
	"context"
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

type moderationTestEnv struct {
	*sessionTeamTestEnv
	moderation     *application.ModerationService
	sessionInvites *application.SessionInviteService
}

func setupModerationTest(t *testing.T) *moderationTestEnv {
	t.Helper()

	env := setupSessionTeamTest(t)
	logger := testutil.CreateLogger()
	repo := persistence.NewModerationRepository(env.txManager, logger)
	groupRepo := persistence.NewActivityGroupRepository(env.txManager, logger)

	return &moderationTestEnv{
		sessionTeamTestEnv: env,
		moderation:         application.NewModerationService(logger, env.txManager, repo, groupRepo, env.memberRepo, env.sessionRepo, testMetrics()),
		sessionInvites: application.NewSessionInviteService(logger, env.txManager,
			persistence.NewSessionInviteRepository(env.txManager, logger), env.sessionRepo, env.attendeeRepo,
			persistence.NewAccountStateManager(env.txManager), repo,
			domainevent.MustNewEventWriter(env.txManager, logger, "activity"), testMetrics()),
	}
}

func (e *moderationTestEnv) report(reporter uuid.UUID, targetType string, target uuid.UUID) error {
	return e.moderation.Report(e.ctx, application.ReportParams{
		ReporterID: reporter, TargetType: targetType, TargetID: target, Reason: "spam", Note: "buy my app",
	})
}

func (e *moderationTestEnv) reports(t *testing.T, target uuid.UUID) int {
	t.Helper()

	var n int
	require.NoError(t, e.txManager.Querier(e.ctx).QueryRow(e.ctx,
		`SELECT count(*) FROM activity.report WHERE target_id = $1`, target).Scan(&n))
	return n
}

func (e *moderationTestEnv) block(t *testing.T, blocker, blocked uuid.UUID) {
	t.Helper()
	require.NoError(t, e.moderation.Block(e.ctx, blocker, blocked))
}

func TestReport(t *testing.T) {
	env := setupModerationTest(t)
	reporter, creator := uuid.New(), uuid.New()
	public := env.createActiveGroup(t, creator, domain.ActivityGroupVisibilityPublic, "Belgrade")
	private := env.createActiveGroup(t, creator, domain.ActivityGroupVisibilityPrivate, "Belgrade")
	session := env.createSessionWith(t, creator, sessionOptions{})

	t.Run("a user, a group and a session can be reported", func(t *testing.T) {
		require.NoError(t, env.report(reporter, "user", creator))
		require.NoError(t, env.report(reporter, "group", public.ID()))
		require.NoError(t, env.report(reporter, "session", session.ID()))

		assert.Equal(t, 1, env.reports(t, creator))
		assert.Equal(t, 1, env.reports(t, public.ID()))
		assert.Equal(t, 1, env.reports(t, session.ID()))
	})

	t.Run("reporting it again while open changes nothing", func(t *testing.T) {
		require.NoError(t, env.report(reporter, "group", public.ID()))
		assert.Equal(t, 1, env.reports(t, public.ID()))
	})

	t.Run("a group the reporter can't see is not found, like one that doesn't exist", func(t *testing.T) {
		requireAppErrorCode(t, env.report(reporter, "group", private.ID()), "activity_group_not_found")
		requireAppErrorCode(t, env.report(reporter, "group", uuid.New()), "activity_group_not_found")
		requireAppErrorCode(t, env.report(reporter, "session", uuid.New()), "session_not_found")
		assert.Zero(t, env.reports(t, private.ID()))
	})

	t.Run("bad input", func(t *testing.T) {
		requireAppErrorCode(t, env.report(reporter, "user", reporter), "cannot_report_self")
		requireAppErrorCode(t, env.moderation.Report(env.ctx, application.ReportParams{
			ReporterID: reporter, TargetType: "user", TargetID: creator, Reason: "rude",
		}), "invalid_report_reason")
	})
}

func TestBlocks(t *testing.T) {
	env := setupModerationTest(t)
	me, them, other := uuid.New(), uuid.New(), uuid.New()

	requireAppErrorCode(t, env.moderation.Block(env.ctx, me, me), "cannot_block_self")

	env.block(t, me, them)
	env.block(t, me, them) // again: no-op
	time.Sleep(time.Millisecond)
	env.block(t, me, other)

	blocks, err := env.moderation.ListBlocks(env.ctx, me)
	require.NoError(t, err)
	require.Len(t, blocks, 2)
	assert.Equal(t, other, blocks[0].BlockedID(), "newest first")
	assert.Equal(t, them, blocks[1].BlockedID())

	require.NoError(t, env.moderation.Unblock(env.ctx, me, them))
	require.NoError(t, env.moderation.Unblock(env.ctx, me, them)) // again: no-op

	blocks, err = env.moderation.ListBlocks(env.ctx, me)
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	assert.Equal(t, other, blocks[0].BlockedID())

	theirs, err := env.moderation.ListBlocks(env.ctx, them)
	require.NoError(t, err)
	assert.Empty(t, theirs, "a block is the blocker's, not the blocked account's")
}

func TestBlock_StopsInvitesBothWays(t *testing.T) {
	env := setupModerationTest(t)
	blocker, blocked := uuid.New(), uuid.New()
	env.block(t, blocker, blocked)

	t.Run("group invites", func(t *testing.T) {
		blockersGroup := env.createGroup(t, blocker)
		blockedsGroup := env.createGroup(t, blocked)

		_, err := env.sendInvite(blockersGroup, blocked)
		requireAppErrorCode(t, err, "account_blocked")
		_, err = env.sendInvite(blockedsGroup, blocker)
		requireAppErrorCode(t, err, "account_blocked")

		_, err = env.sendInvite(blockedsGroup, uuid.New())
		assert.NoError(t, err, "anyone else can still be invited")
	})

	t.Run("session invites", func(t *testing.T) {
		session := env.createSessionWith(t, blocked, sessionOptions{visibility: domain.SessionVisibilityPrivate})

		_, err := env.sessionInvites.SendInvite(env.ctx, application.SendSessionInviteParams{
			SessionID: session.ID(), RequesterID: blocked, InvitedUserID: blocker,
		})
		requireAppErrorCode(t, err, "account_blocked")
	})
}

func TestBlock_HidesInvitesAlreadySentByTheBlockedAccount(t *testing.T) {
	env := setupModerationTest(t)
	me, them := uuid.New(), uuid.New()
	invite, err := env.sendInvite(env.createGroup(t, them), me)
	require.NoError(t, err)

	listed, err := env.inviteSvc.ListMyInvites(env.ctx, me)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, invite.ID(), listed[0].ID())

	env.block(t, me, them)

	listed, err = env.inviteSvc.ListMyInvites(env.ctx, me)
	require.NoError(t, err)
	assert.Empty(t, listed)
}

func TestBlock_HidesTheBlockedAccountsGroupsAndSessions(t *testing.T) {
	env := setupModerationTest(t)
	me, them := uuid.New(), uuid.New()
	theirGroup := env.createActiveGroup(t, them, domain.ActivityGroupVisibilityPublic, "Belgrade")
	sharedGroup := env.createActiveGroup(t, them, domain.ActivityGroupVisibilityPublic, "Belgrade")
	require.NoError(t, env.memberRepo.CreateMember(env.ctx, domain.NewMemberFromInvite(sharedGroup.ID(), me)))
	theirSession := env.createSessionWith(t, them, sessionOptions{})
	joinedSession := env.createSessionWith(t, them, sessionOptions{})
	_, err := env.rsvp(joinedSession, me, domain.AttendeeStatusGoing)
	require.NoError(t, err)

	discoverGroups := func(viewer uuid.UUID) []uuid.UUID {
		groups, _, err := env.groupSvc.DiscoverActivityGroups(env.ctx, application.DiscoverActivityGroupsParams{RequesterID: viewer, Limit: 50})
		require.NoError(t, err)
		return groupIDs(groups)
	}
	browseGroups := func(viewer uuid.UUID) []uuid.UUID {
		groups, _, err := env.groupSvc.ListActivityGroups(env.ctx, application.ListActivityGroupsParams{RequesterID: viewer, Limit: 50})
		require.NoError(t, err)
		return groupIDs(groups)
	}
	discoverSessions := func(viewer uuid.UUID) []uuid.UUID {
		results, _, _, err := env.sessionSvc.DiscoverSessions(env.ctx, application.DiscoverSessionsParams{
			Lat: 44.8, Lng: 20.4, RadiusKM: 10, Limit: 50,
		}, viewer)
		require.NoError(t, err)
		ids := make([]uuid.UUID, len(results))
		for i, r := range results {
			ids[i] = r.Session.ID()
		}
		return ids
	}
	browseSessions := func(viewer uuid.UUID) []uuid.UUID {
		sessions, _, _, err := env.sessionSvc.ListSessions(env.ctx, application.ListSessionsParams{Limit: 50}, viewer)
		require.NoError(t, err)
		ids := make([]uuid.UUID, len(sessions))
		for i, s := range sessions {
			ids[i] = s.ID()
		}
		return ids
	}

	require.Contains(t, discoverGroups(me), theirGroup.ID())
	require.Contains(t, discoverSessions(me), theirSession.ID())

	env.block(t, me, them)

	t.Run("new content of theirs is hidden from me", func(t *testing.T) {
		assert.NotContains(t, discoverGroups(me), theirGroup.ID())
		assert.NotContains(t, browseGroups(me), theirGroup.ID())
		assert.NotContains(t, discoverSessions(me), theirSession.ID())
		assert.NotContains(t, browseSessions(me), theirSession.ID())
	})

	t.Run("what we already share stays", func(t *testing.T) {
		assert.Contains(t, browseGroups(me), sharedGroup.ID())
		assert.Contains(t, discoverSessions(me), joinedSession.ID())
		assert.Contains(t, browseSessions(me), joinedSession.ID())
	})

	t.Run("everyone else still sees it", func(t *testing.T) {
		assert.Contains(t, discoverGroups(uuid.New()), theirGroup.ID())
		assert.Contains(t, discoverSessions(uuid.New()), theirSession.ID())
	})

	t.Run("unblocking shows it again", func(t *testing.T) {
		require.NoError(t, env.moderation.Unblock(env.ctx, me, them))
		assert.Contains(t, discoverGroups(me), theirGroup.ID())
	})
}

func TestAccountErasure_RemovesReportsAndBlocks(t *testing.T) {
	env := setupModerationTest(t)
	erased, other := uuid.New(), uuid.New()
	group := env.createActiveGroup(t, erased, domain.ActivityGroupVisibilityPublic, "Belgrade")
	othersGroup := env.createActiveGroup(t, other, domain.ActivityGroupVisibilityPublic, "Belgrade")

	require.NoError(t, env.report(other, "group", group.ID()))        // about what it created
	require.NoError(t, env.report(other, "user", erased))             // about it
	require.NoError(t, env.report(erased, "group", othersGroup.ID())) // filed by it
	require.NoError(t, env.report(other, "user", uuid.New()))         // unrelated: kept
	env.block(t, erased, other)
	env.block(t, other, erased)

	err := env.txManager.WithTransaction(env.ctx, func(tCtx context.Context) error {
		_, err := persistence.NewAccountStateManager(env.txManager).Erase(tCtx, erased, time.Now().UTC())
		return err
	})
	require.NoError(t, err)

	var reports, blocks int
	q := env.txManager.Querier(env.ctx)
	require.NoError(t, q.QueryRow(env.ctx, `SELECT count(*) FROM activity.report`).Scan(&reports))
	require.NoError(t, q.QueryRow(env.ctx, `SELECT count(*) FROM activity.account_block`).Scan(&blocks))
	assert.Equal(t, 1, reports, "only the unrelated report is left")
	assert.Zero(t, blocks)
}
