//go:build integration

package persistence_test

import (
	"context"
	"encoding/json"
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

type accountLifecycleTestEnv struct {
	ctx            context.Context
	db             *testutil.TestDatabase
	accounts       *application.AccountLifecycleService
	members        *application.MemberService
	invites        *application.InviteService
	sessionInvites *application.SessionInviteService
	attendees      *application.AttendeeService
}

func setupAccountLifecycleTest(t *testing.T) *accountLifecycleTestEnv {
	t.Helper()

	db := setupCapacityRaceTestDB(t)
	txManager := db.CreateTransactionManager()
	logger := testutil.CreateLogger()
	eventWriter := domainevent.NewDomainEventManager(txManager, logger)
	accountStates := persistence.NewAccountStateManager(txManager)

	sessionRepo := persistence.NewSessionManager(txManager, logger)
	attendeeRepo := persistence.NewAttendeeRepository(txManager, logger)
	memberRepo := persistence.NewMemberRepository(txManager, logger)
	groupRepo := persistence.NewActivityGroupRepository(txManager, logger)
	draftRepo := persistence.NewTeamDraftRepository(txManager, logger)
	votingRepo := persistence.NewTeamVotingRepository(txManager, logger)
	sessionInviteRepo := persistence.NewSessionInviteRepository(txManager, logger)

	env := &accountLifecycleTestEnv{
		ctx:            context.Background(),
		db:             db,
		members:        application.NewMemberService(logger, txManager, memberRepo, groupRepo, accountStates, eventWriter, testMetrics()),
		invites:        application.NewInviteService(logger, txManager, persistence.NewGroupInviteRepository(txManager, logger), memberRepo, accountStates, eventWriter, testMetrics()),
		sessionInvites: application.NewSessionInviteService(logger, txManager, sessionInviteRepo, sessionRepo, attendeeRepo, accountStates, eventWriter, testMetrics()),
		attendees:      application.NewAttendeeService(logger, txManager, attendeeRepo, memberRepo, sessionRepo, draftRepo, votingRepo, accountStates, eventWriter, testMetrics()),
	}
	env.accounts = application.NewAccountLifecycleService(
		accountStates, env.attendees, env.invites, env.sessionInvites, env.members,
		txManager, eventWriter, logger, testMetrics(),
	)

	return env
}

// The fixture is written with SQL so that every table gets rows, whatever
// the services allow.

func (e *accountLifecycleTestEnv) group(creator uuid.UUID) uuid.UUID {
	id := uuid.New()
	e.db.RunSQL(`INSERT INTO activity.activity_group (id, creator_id, title, description, activity_type, status)
		VALUES ($1, $2, 'Tuesday Hoops', 'free text by the creator', 'basketball', 'active')`, id, creator)
	e.db.RunSQL(`INSERT INTO activity.activity_group_statistics (activity_group_id) VALUES ($1)`, id)
	e.member(id, creator, "confirmed", "creator")
	return id
}

func (e *accountLifecycleTestEnv) member(groupID, accountID uuid.UUID, status, role string) {
	e.db.RunSQL(`INSERT INTO activity.member (activity_group_id, account_id, status, role)
		VALUES ($1, $2, $3, $4)`, groupID, accountID, status, role)
}

func (e *accountLifecycleTestEnv) template(groupID, creator uuid.UUID) uuid.UUID {
	id := uuid.New()
	e.db.RunSQL(`INSERT INTO activity.session_template (id, activity_group_id, created_by_id, title,
		location_city, location_country, location_lat, location_lng, location_street)
		VALUES ($1, $2, $3, 'Monday', 'Belgrade', 'RS', 44.8, 20.4, 'Ada Ciganlija bb')`, id, groupID, creator)
	return id
}

// session creates a session; groupID and templateID may be nil.
func (e *accountLifecycleTestEnv) session(groupID, templateID *uuid.UUID, creator uuid.UUID, status string, start time.Time) uuid.UUID {
	id := uuid.New()
	e.db.RunSQL(`INSERT INTO activity.session (id, activity_group_id, session_template_id, created_by_id, title,
		activity_type, location_lat, location_lng, location_city, location_country, location_street, start_time,
		status, capacity, note)
		VALUES ($1, $2, $3, $4, 'Pickup', 'basketball', 44.8, 20.4, 'Belgrade', 'RS', 'Court 3', $5, $6, 1, 'a note')`,
		id, groupID, templateID, creator, start, status)
	e.db.RunSQL(`INSERT INTO activity.session_statistics (session_id, activity_group_id) VALUES ($1, $2)`, id, groupID)
	return id
}

func (e *accountLifecycleTestEnv) attendee(sessionID uuid.UUID, groupID *uuid.UUID, accountID uuid.UUID, status string) {
	e.db.RunSQL(`INSERT INTO activity.session_attendee (session_id, activity_group_id, account_id, status)
		VALUES ($1, $2, $3, $4)`, sessionID, groupID, accountID, status)
}

// teamFormation gives a session a team, a draft with captain and pick
// a and b, and a voting round in which a proposes and both vote.
func (e *accountLifecycleTestEnv) teamFormation(sessionID, a, b uuid.UUID) {
	team, draft, round, proposal := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	e.db.RunSQL(`INSERT INTO activity.session_team (id, session_id, name, position) VALUES ($1, $2, 'Team A', 0)`, team, sessionID)
	e.db.RunSQL(`INSERT INTO activity.session_team_draft (id, session_id, pick_order, captain_a_id, captain_b_id, started_by, status)
		VALUES ($1, $2, 'snake', $3, $4, $3, 'completed')`, draft, sessionID, a, b)
	e.db.RunSQL(`INSERT INTO activity.session_team_draft_pick (draft_id, pick_number, side, account_id)
		VALUES ($1, 0, 0, $2), ($1, 1, 1, $3)`, draft, a, b)
	e.db.RunSQL(`INSERT INTO activity.session_team_voting_round (id, session_id, team_count) VALUES ($1, $2, 2)`, round, sessionID)
	e.db.RunSQL(`INSERT INTO activity.session_team_proposal (id, round_id, author_id) VALUES ($1, $2, $3)`, proposal, round, a)
	e.db.RunSQL(`INSERT INTO activity.session_team_proposal_member (proposal_id, team_position, account_id, sort_order)
		VALUES ($1, 0, $2, 0), ($1, 1, $3, 0)`, proposal, a, b)
	e.db.RunSQL(`INSERT INTO activity.session_team_vote (round_id, voter_id, proposal_id) VALUES ($1, $2, $4), ($1, $3, $4)`,
		round, a, b, proposal)
}

func (e *accountLifecycleTestEnv) groupInvite(groupID, to, by uuid.UUID) uuid.UUID {
	id := uuid.New()
	e.db.RunSQL(`INSERT INTO activity.group_invites (id, activity_group_id, invited_user_id, invited_by_user_id, expires_at)
		VALUES ($1, $2, $3, $4, NOW() + interval '7 days')`, id, groupID, to, by)
	return id
}

func (e *accountLifecycleTestEnv) sessionInvite(sessionID, to, by uuid.UUID) uuid.UUID {
	id := uuid.New()
	e.db.RunSQL(`INSERT INTO activity.session_invites (id, session_id, invited_user_id, invited_by_user_id, expires_at)
		VALUES ($1, $2, $3, $4, NOW() + interval '7 days')`, id, sessionID, to, by)
	return id
}

// inviteLink creates a link to groupID by creator, used by user.
func (e *accountLifecycleTestEnv) inviteLink(groupID, creator, user uuid.UUID) uuid.UUID {
	id := uuid.New()
	e.db.RunSQL(`INSERT INTO activity.group_invite_links (id, activity_group_id, created_by_user_id, token, expires_at)
		VALUES ($1, $2, $3, $4, NOW() + interval '7 days')`, id, groupID, creator, uuid.NewString())
	e.db.RunSQL(`INSERT INTO activity.group_invite_link_usage (group_invite_link_id, account_id) VALUES ($1, $2)`, id, user)
	return id
}

func (e *accountLifecycleTestEnv) count(t *testing.T, query string, args ...any) int {
	t.Helper()

	var n int
	require.NoError(t, e.db.Pool.QueryRow(e.ctx, query, args...).Scan(&n))
	return n
}

func (e *accountLifecycleTestEnv) exists(t *testing.T, table string, id uuid.UUID) bool {
	t.Helper()
	return e.count(t, `SELECT count(*) FROM activity.`+table+` WHERE id = $1`, id) == 1
}

func (e *accountLifecycleTestEnv) erasedEvents(t *testing.T, accountID uuid.UUID) []json.RawMessage {
	t.Helper()

	rows, err := e.db.Pool.Query(e.ctx, `SELECT payload FROM activity.event_outbox
		WHERE event_type = $1 AND aggregate_id = $2`, domain.EventAccountErased, accountID)
	require.NoError(t, err)
	defer rows.Close()

	var payloads []json.RawMessage
	for rows.Next() {
		var p json.RawMessage
		require.NoError(t, rows.Scan(&p))
		payloads = append(payloads, p)
	}
	require.NoError(t, rows.Err())
	return payloads
}

func TestAccountLifecycle_DeletionGivesUpWhatIsComingButKeepsWhatItCreated(t *testing.T) {
	env := setupAccountLifecycleTest(t)
	leaving, owner, waiting := uuid.New(), uuid.New(), uuid.New()

	// someone else's group, which leaving is a member of
	othersGroup := env.group(owner)
	env.member(othersGroup, leaving, "confirmed", "admin")
	upcoming := env.session(&othersGroup, nil, owner, "scheduled", time.Now().Add(24*time.Hour))
	env.attendee(upcoming, &othersGroup, leaving, "going")
	env.attendee(upcoming, &othersGroup, waiting, "pending") // capacity 1: on the waitlist
	completed := env.session(&othersGroup, nil, owner, "completed", time.Now().Add(-24*time.Hour))
	env.attendee(completed, &othersGroup, leaving, "going")
	groupInvite := env.groupInvite(env.group(owner), leaving, owner)
	standalone := env.session(nil, nil, owner, "scheduled", time.Now().Add(24*time.Hour))
	sessionInvite := env.sessionInvite(standalone, leaving, owner)

	// its own group and session
	ownGroup := env.group(leaving)
	ownSession := env.session(&ownGroup, nil, leaving, "scheduled", time.Now().Add(24*time.Hour))

	require.NoError(t, env.accounts.DeactivateAccount(env.ctx, leaving))
	require.NoError(t, env.accounts.DeactivateAccount(env.ctx, leaving), "a redelivered deletion is a no-op")

	inactive, err := env.accounts.IsInactive(env.ctx, leaving)
	require.NoError(t, err)
	assert.True(t, inactive)

	assert.Zero(t, env.count(t, `SELECT count(*) FROM activity.session_attendee
		WHERE session_id = $1 AND account_id = $2 AND deleted_at IS NULL`, upcoming, leaving), "RSVP to the upcoming session cancelled")
	assert.Equal(t, 1, env.count(t, `SELECT count(*) FROM activity.session_attendee
		WHERE session_id = $1 AND account_id = $2 AND status = 'promoted'`, upcoming, waiting), "the waitlist moved up")
	assert.Equal(t, 1, env.count(t, `SELECT count(*) FROM activity.session_attendee
		WHERE session_id = $1 AND account_id = $2 AND deleted_at IS NULL`, completed, leaving), "past attendance stays")

	assert.Equal(t, 1, env.count(t, `SELECT count(*) FROM activity.member
		WHERE activity_group_id = $1 AND account_id = $2 AND status = 'left'`, othersGroup, leaving), "left the group")
	assert.Equal(t, 1, env.count(t, `SELECT count(*) FROM activity.group_invites WHERE id = $1 AND status = 'declined'`, groupInvite))
	assert.Equal(t, 1, env.count(t, `SELECT count(*) FROM activity.session_invites WHERE id = $1 AND status = 'declined'`, sessionInvite))

	assert.Equal(t, 1, env.count(t, `SELECT count(*) FROM activity.member
		WHERE activity_group_id = $1 AND account_id = $2 AND status = 'confirmed'`, ownGroup, leaving), "still the creator of its group")
	assert.True(t, env.exists(t, "session", ownSession), "its session keeps working until the erasure")
}

func TestAccountLifecycle_ErasureRemovesWhatItCreatedAndWhatIsAboutIt(t *testing.T) {
	env := setupAccountLifecycleTest(t)
	erased, other, third := uuid.New(), uuid.New(), uuid.New()

	// Everything the erased account created, each with the other account in it.
	ownGroup := env.group(erased)
	env.member(ownGroup, other, "confirmed", "member")
	ownTemplate := env.template(ownGroup, erased)
	ownGroupSession := env.session(&ownGroup, &ownTemplate, erased, "completed", time.Now().Add(-time.Hour))
	env.attendee(ownGroupSession, &ownGroup, other, "going")
	env.teamFormation(ownGroupSession, erased, other)
	env.groupInvite(ownGroup, third, erased)
	env.inviteLink(ownGroup, erased, other)
	ownStandalone := env.session(nil, nil, erased, "scheduled", time.Now().Add(time.Hour))
	env.attendee(ownStandalone, nil, other, "going")
	env.sessionInvite(ownStandalone, other, erased)

	// Someone else's group, with the erased account in it.
	othersGroup := env.group(other)
	env.member(othersGroup, erased, "left", "member")
	templateInOthersGroup := env.template(othersGroup, erased)
	sessionInOthersGroup := env.session(&othersGroup, nil, erased, "scheduled", time.Now().Add(time.Hour))
	othersTemplate := env.template(othersGroup, other)
	played := env.session(&othersGroup, &othersTemplate, other, "completed", time.Now().Add(-time.Hour))
	env.attendee(played, &othersGroup, erased, "going")
	env.attendee(played, &othersGroup, other, "going")
	env.teamFormation(played, other, erased)
	withdrawn := env.session(&othersGroup, nil, other, "scheduled", time.Now().Add(time.Hour))
	env.db.RunSQL(`INSERT INTO activity.session_attendee (session_id, activity_group_id, account_id, status, deleted_at)
		VALUES ($1, $2, $3, 'going', NOW())`, withdrawn, othersGroup, erased)
	inviteToErased := env.groupInvite(othersGroup, erased, other)
	inviteByErased := env.groupInvite(othersGroup, third, erased)
	othersStandalone := env.session(nil, nil, other, "scheduled", time.Now().Add(time.Hour))
	sessionInviteToErased := env.sessionInvite(othersStandalone, erased, other)
	linkUsedByErased := env.inviteLink(othersGroup, other, erased)
	linkByErased := env.inviteLink(othersGroup, erased, third)

	require.NoError(t, env.accounts.EraseAccount(env.ctx, erased))

	t.Run("what it created is gone, with everything in it", func(t *testing.T) {
		assert.False(t, env.exists(t, "activity_group", ownGroup))
		for _, id := range []uuid.UUID{ownGroupSession, ownStandalone, sessionInOthersGroup} {
			assert.False(t, env.exists(t, "session", id))
		}
		for _, id := range []uuid.UUID{ownTemplate, templateInOthersGroup} {
			assert.False(t, env.exists(t, "session_template", id))
		}
		for table, query := range map[string]string{
			"members":            `SELECT count(*) FROM activity.member WHERE activity_group_id = $1`,
			"group invites":      `SELECT count(*) FROM activity.group_invites WHERE activity_group_id = $1`,
			"group invite links": `SELECT count(*) FROM activity.group_invite_links WHERE activity_group_id = $1`,
			"group statistics":   `SELECT count(*) FROM activity.activity_group_statistics WHERE activity_group_id = $1`,
		} {
			assert.Zero(t, env.count(t, query, ownGroup), table)
		}
		for table, query := range map[string]string{
			"attendees":  `SELECT count(*) FROM activity.session_attendee WHERE session_id = ANY($1)`,
			"teams":      `SELECT count(*) FROM activity.session_team WHERE session_id = ANY($1)`,
			"drafts":     `SELECT count(*) FROM activity.session_team_draft WHERE session_id = ANY($1)`,
			"rounds":     `SELECT count(*) FROM activity.session_team_voting_round WHERE session_id = ANY($1)`,
			"invites":    `SELECT count(*) FROM activity.session_invites WHERE session_id = ANY($1)`,
			"statistics": `SELECT count(*) FROM activity.session_statistics WHERE session_id = ANY($1)`,
		} {
			assert.Zero(t, env.count(t, query, []uuid.UUID{ownGroupSession, ownStandalone, sessionInOthersGroup}), table)
		}
	})

	t.Run("what is about it in others' records is gone", func(t *testing.T) {
		assert.Zero(t, env.count(t, `SELECT count(*) FROM activity.member WHERE account_id = $1`, erased), "memberships")
		assert.False(t, env.exists(t, "group_invites", inviteToErased))
		assert.False(t, env.exists(t, "session_invites", sessionInviteToErased))
		assert.Zero(t, env.count(t, `SELECT count(*) FROM activity.group_invite_link_usage WHERE account_id = $1`, erased))
		assert.True(t, env.exists(t, "group_invite_links", linkUsedByErased), "the link itself is the other's")
		assert.Zero(t, env.count(t, `SELECT count(*) FROM activity.session_attendee WHERE session_id = $1`, withdrawn), "withdrawn RSVP")
	})

	t.Run("others' records keep it only as an actor", func(t *testing.T) {
		assert.True(t, env.exists(t, "activity_group", othersGroup))
		assert.True(t, env.exists(t, "session", played))
		assert.True(t, env.exists(t, "session_template", othersTemplate))
		assert.Equal(t, 1, env.count(t, `SELECT count(*) FROM activity.member WHERE activity_group_id = $1 AND account_id = $2`, othersGroup, other))
		assert.Equal(t, 2, env.count(t, `SELECT count(*) FROM activity.session_attendee WHERE session_id = $1`, played), "past attendance")
		assert.Equal(t, 1, env.count(t, `SELECT count(*) FROM activity.session_team_draft WHERE session_id = $1 AND captain_b_id = $2`, played, erased))
		assert.Equal(t, 1, env.count(t, `SELECT count(*) FROM activity.session_team_vote WHERE voter_id = $1`, erased))
		assert.True(t, env.exists(t, "group_invites", inviteByErased))
		assert.True(t, env.exists(t, "group_invite_links", linkByErased))
		assert.Equal(t, 1, env.count(t, `SELECT count(*) FROM activity.group_invite_link_usage WHERE group_invite_link_id = $1`, linkByErased))
	})

	t.Run("confirmed once", func(t *testing.T) {
		payloads := env.erasedEvents(t, erased)
		require.Len(t, payloads, 1, "activity.account.erased in the outbox")
		var payload map[string]any
		require.NoError(t, json.Unmarshal(payloads[0], &payload))
		assert.Equal(t, erased.String(), payload["account_id"])

		require.NoError(t, env.accounts.EraseAccount(env.ctx, erased), "a redelivered erasure is a no-op")
		assert.Len(t, env.erasedEvents(t, erased), 1)

		inactive, err := env.accounts.IsInactive(env.ctx, erased)
		require.NoError(t, err)
		assert.True(t, inactive)
	})
}

func TestAccountLifecycle_ErasureOfAnAccountWithNoActivityIsStillConfirmed(t *testing.T) {
	env := setupAccountLifecycleTest(t)
	nobody := uuid.New()

	require.NoError(t, env.accounts.EraseAccount(env.ctx, nobody))

	assert.Len(t, env.erasedEvents(t, nobody), 1)
}

// Others cannot create new data about a deleted account: no invite to it, and
// a request it made before the deletion cannot be approved.
func TestAccountLifecycle_ADeletedAccountCannotBeInvitedOrApproved(t *testing.T) {
	env := setupAccountLifecycleTest(t)
	deleted, owner := uuid.New(), uuid.New()

	group := env.group(owner)
	env.member(group, deleted, "pending", "member") // asked to join before deleting
	session := env.session(&group, nil, owner, "scheduled", time.Now().Add(24*time.Hour))
	env.attendee(session, &group, deleted, "pending") // asked to attend before deleting
	standalone := env.session(nil, nil, owner, "scheduled", time.Now().Add(24*time.Hour))

	require.NoError(t, env.accounts.DeactivateAccount(env.ctx, deleted))

	creator := string(domain.MemberRoleCreator)
	for name, call := range map[string]func() error{
		"invite as member": func() error {
			_, err := env.members.InviteMember(env.ctx, application.InviteMemberParams{
				ActivityGroupID: group, RequesterID: owner, RequesterRole: creator, UserID: deleted,
			})
			return err
		},
		"group invite": func() error {
			_, err := env.invites.SendInvite(env.ctx, application.SendInviteParams{
				ActivityGroupID: group, RequesterID: owner, RequesterRole: creator, InvitedUserID: deleted,
			})
			return err
		},
		"session invite": func() error {
			_, err := env.sessionInvites.SendInvite(env.ctx, application.SendSessionInviteParams{
				SessionID: standalone, RequesterID: owner, InvitedUserID: deleted,
			})
			return err
		},
		"approve join request": func() error {
			return env.members.ApproveMember(env.ctx, application.ApproveMemberParams{
				ActivityGroupID: group, RequesterID: owner, RequesterRole: creator, UserID: deleted,
			})
		},
		"approve attendee": func() error {
			return env.attendees.ApproveAttendee(env.ctx, application.ApproveAttendeeParams{
				SessionID: session, UserID: deleted, RequesterID: owner, RequesterRole: creator,
			})
		},
	} {
		assert.ErrorIs(t, call(), domain.ErrAccountDeleted, name)
	}

	assert.Zero(t, env.count(t, `SELECT count(*) FROM activity.group_invites WHERE invited_user_id = $1`, deleted))
	assert.Zero(t, env.count(t, `SELECT count(*) FROM activity.session_invites WHERE invited_user_id = $1`, deleted))
	assert.Equal(t, 1, env.count(t, `SELECT count(*) FROM activity.member
		WHERE account_id = $1 AND status = 'pending'`, deleted), "the request stays pending until the erasure")
}
