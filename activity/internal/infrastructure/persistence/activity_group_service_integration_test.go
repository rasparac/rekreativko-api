//go:build integration

package persistence_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	testutil "github.com/rasparac/rekreativko-api/shared/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createActiveGroup creates a group through the service and activates it.
func (e *sessionTeamTestEnv) createActiveGroup(t *testing.T, creatorID uuid.UUID, visibility domain.ActivityGroupVisibility, city string) *domain.ActivityGroup {
	t.Helper()

	group, err := e.groupSvc.CreateActivityGroup(e.ctx, application.CreateActivityGroupParams{
		CreatorID:       creatorID,
		Title:           "Hoops in " + city,
		ActivityType:    string(domain.ActivityTypeBasketball),
		DifficultyLevel: string(domain.DifficultyLevelBeginner),
		Visibility:      string(visibility),
		LocationCity:    city,
		LocationCountry: "RS",
		Timezone:        "UTC",
	})
	require.NoError(t, err)
	require.NoError(t, e.groupSvc.ActivateActivityGroup(e.ctx, group.ID(), creatorID))

	return group
}

func groupIDs(groups []*domain.ActivityGroup) []uuid.UUID {
	ids := make([]uuid.UUID, len(groups))
	for i, g := range groups {
		ids[i] = g.ID()
	}
	return ids
}

func TestActivityGroupService_ActivateActivityGroup(t *testing.T) {
	env := setupSessionTeamTest(t)
	creator := uuid.New()
	group := env.createGroup(t, creator)
	require.Equal(t, domain.ActivityGroupStatusDraft, group.Status())

	t.Run("only the creator may", func(t *testing.T) {
		admin := env.addAdmin(t, group)
		assert.ErrorIs(t, env.groupSvc.ActivateActivityGroup(env.ctx, group.ID(), admin), domain.ErrUnauthorized)
	})

	t.Run("the creator publishes the draft", func(t *testing.T) {
		require.NoError(t, env.groupSvc.ActivateActivityGroup(env.ctx, group.ID(), creator))

		loaded, err := env.groupSvc.GetActivityGroup(env.ctx, group.ID(), creator)
		require.NoError(t, err)
		assert.Equal(t, domain.ActivityGroupStatusActive, loaded.Status())
		assert.Equal(t, 1, env.outboxCount(t, domain.EventActivityGroupPublished))
	})

	t.Run("a group is published only once", func(t *testing.T) {
		requireAppErrorCode(t, env.groupSvc.ActivateActivityGroup(env.ctx, group.ID(), creator), "activity_group_not_draft")
	})

	t.Run("an unknown group is not found", func(t *testing.T) {
		assert.ErrorIs(t, env.groupSvc.ActivateActivityGroup(env.ctx, uuid.New(), creator), domain.ErrActivityGroupNotFound)
	})
}

func TestActivityGroupService_GetActivityGroup_APrivateGroupIsHiddenFromStrangers(t *testing.T) {
	env := setupSessionTeamTest(t)
	creator := uuid.New()
	group := env.createActiveGroup(t, creator, domain.ActivityGroupVisibilityPrivate, "Belgrade")
	member := env.addMember(t, group.ID())

	_, err := env.groupSvc.GetActivityGroup(env.ctx, group.ID(), uuid.New())
	assert.ErrorIs(t, err, domain.ErrActivityGroupNotFound, "not forbidden: a stranger can't tell it exists")

	for _, viewer := range []uuid.UUID{creator, member} {
		_, err := env.groupSvc.GetActivityGroup(env.ctx, group.ID(), viewer)
		assert.NoError(t, err)
	}
}

func TestActivityGroupService_ListActivityGroups(t *testing.T) {
	env := setupSessionTeamTest(t)
	me, other := uuid.New(), uuid.New()

	myPublic := env.createActiveGroup(t, me, domain.ActivityGroupVisibilityPublic, "Belgrade")
	myPrivate := env.createActiveGroup(t, me, domain.ActivityGroupVisibilityPrivate, "Belgrade")
	theirPublic := env.createActiveGroup(t, other, domain.ActivityGroupVisibilityPublic, "Belgrade")
	theirPrivate := env.createActiveGroup(t, other, domain.ActivityGroupVisibilityPrivate, "Belgrade")
	theirPrivateImIn := env.createActiveGroup(t, other, domain.ActivityGroupVisibilityPrivate, "Belgrade")
	require.NoError(t, env.memberRepo.CreateMember(env.ctx, domain.NewMemberFromInvite(theirPrivateImIn.ID(), me)))

	list := func(t *testing.T, params application.ListActivityGroupsParams) []uuid.UUID {
		t.Helper()
		params.RequesterID = me
		groups, _, err := env.groupSvc.ListActivityGroups(env.ctx, params)
		require.NoError(t, err)
		return groupIDs(groups)
	}

	t.Run("everything I may see, never a stranger's private group", func(t *testing.T) {
		got := list(t, application.ListActivityGroupsParams{})
		assert.ElementsMatch(t, []uuid.UUID{myPublic.ID(), myPrivate.ID(), theirPublic.ID(), theirPrivateImIn.ID()}, got)
		assert.NotContains(t, got, theirPrivate.ID())
	})

	t.Run("my own creations, private included", func(t *testing.T) {
		assert.ElementsMatch(t, []uuid.UUID{myPublic.ID(), myPrivate.ID()}, list(t, application.ListActivityGroupsParams{CreatorID: &me}))
	})

	t.Run("someone else's creations: public only, even the private one I'm in", func(t *testing.T) {
		assert.Equal(t, []uuid.UUID{theirPublic.ID()}, list(t, application.ListActivityGroupsParams{CreatorID: &other}))
	})

	t.Run("someone else's memberships: confirmed and public only", func(t *testing.T) {
		pending := env.createActiveGroup(t, uuid.New(), domain.ActivityGroupVisibilityPublic, "Belgrade")
		require.NoError(t, env.memberRepo.CreateMember(env.ctx, domain.NewJoinRequest(pending.ID(), other, nil)))

		got := list(t, application.ListActivityGroupsParams{MemberID: &other, MemberStatus: []string{"pending"}})
		assert.ElementsMatch(t, []uuid.UUID{theirPublic.ID()}, got, "their pending request stays private")
	})

	t.Run("my own pending requests", func(t *testing.T) {
		pending := env.createActiveGroup(t, uuid.New(), domain.ActivityGroupVisibilityPublic, "Belgrade")
		require.NoError(t, env.memberRepo.CreateMember(env.ctx, domain.NewJoinRequest(pending.ID(), me, nil)))

		assert.Equal(t, []uuid.UUID{pending.ID()}, list(t, application.ListActivityGroupsParams{MemberID: &me, MemberStatus: []string{"pending"}}))
	})
}

func TestActivityGroupService_DiscoverActivityGroups(t *testing.T) {
	env := setupSessionTeamTest(t)
	creator := uuid.New()

	belgrade := env.createActiveGroup(t, creator, domain.ActivityGroupVisibilityPublic, "Belgrade")
	noviSad := env.createActiveGroup(t, creator, domain.ActivityGroupVisibilityPublic, "Novi Sad")
	env.createActiveGroup(t, creator, domain.ActivityGroupVisibilityPrivate, "Belgrade")
	env.createGroup(t, creator) // a draft in Belgrade

	discover := func(t *testing.T, params application.DiscoverActivityGroupsParams) []uuid.UUID {
		t.Helper()
		groups, _, err := env.groupSvc.DiscoverActivityGroups(env.ctx, params)
		require.NoError(t, err)
		return groupIDs(groups)
	}

	t.Run("only public, active groups", func(t *testing.T) {
		assert.ElementsMatch(t, []uuid.UUID{belgrade.ID(), noviSad.ID()}, discover(t, application.DiscoverActivityGroupsParams{}))
	})

	t.Run("in a city", func(t *testing.T) {
		assert.Equal(t, []uuid.UUID{noviSad.ID()}, discover(t, application.DiscoverActivityGroupsParams{City: testutil.Ptr("novi sad")}))
	})

	t.Run("matching an interest", func(t *testing.T) {
		basketball := application.ActivityInterestFilter{ActivityType: string(domain.ActivityTypeBasketball)}
		advancedBasketball := application.ActivityInterestFilter{ActivityType: string(domain.ActivityTypeBasketball), DifficultyLevel: string(domain.DifficultyLevelAdvanced)}
		running := application.ActivityInterestFilter{ActivityType: string(domain.ActivityTypeRunning)}

		assert.Len(t, discover(t, application.DiscoverActivityGroupsParams{Interests: []application.ActivityInterestFilter{running, basketball}}), 2)
		assert.Empty(t, discover(t, application.DiscoverActivityGroupsParams{Interests: []application.ActivityInterestFilter{running, advancedBasketball}}))
	})

	t.Run("an unknown activity type is rejected", func(t *testing.T) {
		_, _, err := env.groupSvc.DiscoverActivityGroups(env.ctx, application.DiscoverActivityGroupsParams{
			Interests: []application.ActivityInterestFilter{{ActivityType: "chess"}},
		})
		requireAppErrorCode(t, err, "invalid_activity_type")
	})
}

// Updating a group changes everything it was asked to, difficulty included
// (it used to be validated and then silently kept), and survives a reload.
func TestActivityGroupService_UpdateActivityGroup_PersistsEveryField(t *testing.T) {
	env := setupSessionTeamTest(t)
	creator := uuid.New()
	group := env.createGroup(t, creator)
	require.Equal(t, domain.DifficultyLevelBeginner, group.DifficultyLevel())

	update := func(requester uuid.UUID) error {
		return env.groupSvc.UpdateActivityGroup(env.ctx, group.ID(), application.UpdateActivityGroupParams{
			RequesterID:     requester,
			Title:           "Thursday Footy",
			Description:     "Moved to football",
			ActivityType:    string(domain.ActivityTypeFootball),
			DifficultyLevel: string(domain.DifficultyLevelAdvanced),
			LocationCity:    "Novi Sad",
			LocationCountry: "RS",
			Timezone:        "Europe/Vienna",
		})
	}

	t.Run("someone else may not", func(t *testing.T) {
		assert.ErrorIs(t, update(uuid.New()), domain.ErrUnauthorized)
	})

	t.Run("the creator changes it", func(t *testing.T) {
		require.NoError(t, update(creator))

		loaded, err := env.groupSvc.GetActivityGroup(env.ctx, group.ID(), creator)
		require.NoError(t, err)
		assert.Equal(t, "Thursday Footy", loaded.Title().Value())
		assert.Equal(t, "Moved to football", loaded.Description())
		assert.Equal(t, domain.ActivityTypeFootball, loaded.ActivityType())
		assert.Equal(t, domain.DifficultyLevelAdvanced, loaded.DifficultyLevel())
		assert.Equal(t, "Novi Sad", loaded.Location().City())
		assert.Equal(t, "Europe/Vienna", loaded.Timezone())
	})
}

func TestActivityGroupService_CreateActivityGroup_RejectsInvalidInput(t *testing.T) {
	env := setupSessionTeamTest(t)
	creator := uuid.New()
	valid := func() application.CreateActivityGroupParams {
		return application.CreateActivityGroupParams{
			CreatorID:       creator,
			Title:           "Tuesday Hoops",
			ActivityType:    string(domain.ActivityTypeBasketball),
			DifficultyLevel: string(domain.DifficultyLevelBeginner),
			Visibility:      string(domain.ActivityGroupVisibilityPublic),
			LocationCity:    "Belgrade",
			LocationCountry: "RS",
			Timezone:        "UTC",
		}
	}

	// each one a 400 with its own code (rekreativko-api-5o1: these were 500s)
	for code, change := range map[string]func(*application.CreateActivityGroupParams){
		"title_required":           func(p *application.CreateActivityGroupParams) { p.Title = "" },
		"city_required":            func(p *application.CreateActivityGroupParams) { p.LocationCity = "" },
		"invalid_activity_type":    func(p *application.CreateActivityGroupParams) { p.ActivityType = "chess" },
		"invalid_difficulty_level": func(p *application.CreateActivityGroupParams) { p.DifficultyLevel = "godlike" },
		"invalid_visibility":       func(p *application.CreateActivityGroupParams) { p.Visibility = "secret" },
		"invalid_capacity":         func(p *application.CreateActivityGroupParams) { p.DefaultCapacity = testutil.Ptr(0) },
	} {
		t.Run(code, func(t *testing.T) {
			params := valid()
			change(&params)

			_, err := env.groupSvc.CreateActivityGroup(env.ctx, params)
			requireAppErrorCode(t, err, code)
		})
	}

	groups, _, err := env.groupSvc.ListActivityGroups(env.ctx, application.ListActivityGroupsParams{CreatorID: &creator, RequesterID: creator})
	require.NoError(t, err)
	assert.Empty(t, groups, "nothing was created")
}

func TestActivityGroupService_DeleteActivityGroup_OnlyTheCreator(t *testing.T) {
	env := setupSessionTeamTest(t)
	creator := uuid.New()
	group := env.createGroup(t, creator)

	assert.ErrorIs(t, env.groupSvc.DeleteActivityGroup(env.ctx, group.ID(), env.addAdmin(t, group)), domain.ErrUnauthorized)
	_, err := env.groupSvc.GetActivityGroup(env.ctx, group.ID(), creator)
	require.NoError(t, err, "still there")

	require.NoError(t, env.groupSvc.DeleteActivityGroup(env.ctx, group.ID(), creator))
	assert.Equal(t, 1, env.outboxCount(t, domain.EventActivityGroupDeleted))
}
