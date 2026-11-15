//go:build integration

package persistence_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	testutil "github.com/rasparac/rekreativko-api/shared/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func weeklyTemplateParams(groupID, creatorID uuid.UUID) application.CreateSessionTemplateParams {
	return application.CreateSessionTemplateParams{
		ActivityGroupID:     groupID,
		CreatedByID:         creatorID,
		Title:               "Tuesday Hoops",
		Description:         "Half court",
		RecurrenceFrequency: testutil.Ptr(string(domain.RecurrenceFrequencyWeekly)),
		RecurrenceDayOfWeek: testutil.Ptr(time.Tuesday),
		RecurrenceTimeHour:  19,
		DefaultCapacity:     testutil.Ptr(10),
		LocationCity:        "Belgrade",
		LocationCountry:     "RS",
		LocationLat:         44.8,
		LocationLng:         20.4,
	}
}

func (e *sessionTeamTestEnv) createTemplate(t *testing.T, groupID, creatorID uuid.UUID) *domain.SessionTemplate {
	t.Helper()

	template, err := e.templateSvc.CreateSessionTemplate(e.ctx, weeklyTemplateParams(groupID, creatorID))
	require.NoError(t, err)

	return template
}

func TestSessionTemplateService_CreateSessionTemplate(t *testing.T) {
	env := setupSessionTeamTest(t)
	creator := uuid.New()
	group := env.createGroup(t, creator)

	t.Run("only a group manager may", func(t *testing.T) {
		member := env.addMember(t, group.ID())

		for _, requester := range []uuid.UUID{uuid.New(), member} {
			_, err := env.templateSvc.CreateSessionTemplate(env.ctx, weeklyTemplateParams(group.ID(), requester))
			assert.ErrorIs(t, err, domain.ErrUnauthorized)
		}
	})

	t.Run("an invalid recurrence is rejected", func(t *testing.T) {
		params := weeklyTemplateParams(group.ID(), creator)
		params.RecurrenceDayOfWeek = nil

		_, err := env.templateSvc.CreateSessionTemplate(env.ctx, params)
		assert.ErrorIs(t, err, domain.ErrRecurrenceRuleMissingDayOfWeek)
	})

	t.Run("the creator creates an active template", func(t *testing.T) {
		created := env.createTemplate(t, group.ID(), creator)

		loaded, err := env.templateSvc.GetSessionTemplate(env.ctx, created.ID())
		require.NoError(t, err)
		assert.Equal(t, "Tuesday Hoops", loaded.Title())
		assert.Equal(t, domain.SessionTemplateStatusActive, loaded.Status())
		assert.Equal(t, domain.RecurrenceFrequencyWeekly, loaded.RecurrenceFrequency())
		assert.Equal(t, int(time.Tuesday), loaded.RecurrenceDayOfWeek())
		assert.Equal(t, 19, loaded.RecurrenceTimeHour())
		assert.Equal(t, testutil.Ptr(10), loaded.DefaultCapacity())
		assert.Equal(t, 1, env.outboxCount(t, domain.EventActivitySessionTemplateCreated))
	})
}

func TestSessionTemplateService_GetSessionTemplate_UnknownIsNotFound(t *testing.T) {
	env := setupSessionTeamTest(t)

	_, err := env.templateSvc.GetSessionTemplate(env.ctx, uuid.New())
	requireAppErrorCode(t, err, "not_found")
}

func TestSessionTemplateService_UpdateSessionTemplate(t *testing.T) {
	env := setupSessionTeamTest(t)
	creator := uuid.New()
	group := env.createGroup(t, creator)
	template := env.createTemplate(t, group.ID(), creator)

	update := func(requester uuid.UUID) error {
		return env.templateSvc.UpdateSessionTemplate(env.ctx, template.ID(), application.UpdateSessionTemplateParams{
			RequesterID:          requester,
			Title:                "Thursday Hoops",
			RecurrenceFrequency:  testutil.Ptr(string(domain.RecurrenceFrequencyMonthly)),
			RecurrenceDayOfMonth: testutil.Ptr(15),
			RecurrenceTimeHour:   18,
			RecurrenceTimeMinute: 30,
			LocationCity:         "Novi Sad",
			LocationCountry:      "RS",
			LocationLat:          45.2,
			LocationLng:          19.8,
		})
	}

	t.Run("a non-manager may not", func(t *testing.T) {
		assert.ErrorIs(t, update(env.addMember(t, group.ID())), domain.ErrUnauthorized)
	})

	t.Run("an unknown template is not found", func(t *testing.T) {
		err := env.templateSvc.UpdateSessionTemplate(env.ctx, uuid.New(), application.UpdateSessionTemplateParams{RequesterID: creator})
		requireAppErrorCode(t, err, "not_found")
	})

	t.Run("an admin changes everything it was asked to", func(t *testing.T) {
		require.NoError(t, update(env.addAdmin(t, group)))

		loaded, err := env.templateSvc.GetSessionTemplate(env.ctx, template.ID())
		require.NoError(t, err)
		assert.Equal(t, "Thursday Hoops", loaded.Title())
		assert.Empty(t, loaded.Description())
		assert.Equal(t, domain.RecurrenceFrequencyMonthly, loaded.RecurrenceFrequency())
		assert.Equal(t, 15, loaded.RecurrenceDayOfMonth())
		assert.Equal(t, 30, loaded.RecurrenceTimeMinute())
		assert.Nil(t, loaded.DefaultCapacity(), "no capacity sent = unlimited")
		assert.Equal(t, "Novi Sad", loaded.LocationCity())
		assert.Equal(t, 1, env.outboxCount(t, domain.EventActivitySessionTemplateUpdated))
	})
}

func TestSessionTemplateService_ActivateAndDeactivate(t *testing.T) {
	env := setupSessionTeamTest(t)
	creator := uuid.New()
	group := env.createGroup(t, creator)
	template := env.createTemplate(t, group.ID(), creator)

	status := func(t *testing.T) domain.SessionTemplateStatus {
		t.Helper()
		loaded, err := env.templateSvc.GetSessionTemplate(env.ctx, template.ID())
		require.NoError(t, err)
		return loaded.Status()
	}

	t.Run("a non-manager may do neither", func(t *testing.T) {
		member := env.addMember(t, group.ID())
		assert.ErrorIs(t, env.templateSvc.DeactivateSessionTemplate(env.ctx, template.ID(), member), domain.ErrUnauthorized)
		assert.ErrorIs(t, env.templateSvc.ActivateSessionTemplate(env.ctx, template.ID(), member), domain.ErrUnauthorized)
		assert.Equal(t, domain.SessionTemplateStatusActive, status(t))
	})

	t.Run("the creator deactivates it", func(t *testing.T) {
		require.NoError(t, env.templateSvc.DeactivateSessionTemplate(env.ctx, template.ID(), creator))
		assert.Equal(t, domain.SessionTemplateStatusInactive, status(t))
		assert.Equal(t, 1, env.outboxCount(t, domain.EventActivitySessionTemplateDeactivated))
	})

	t.Run("deactivating again changes nothing", func(t *testing.T) {
		require.NoError(t, env.templateSvc.DeactivateSessionTemplate(env.ctx, template.ID(), creator))
		assert.Equal(t, 1, env.outboxCount(t, domain.EventActivitySessionTemplateDeactivated))
	})

	t.Run("and activates it again", func(t *testing.T) {
		require.NoError(t, env.templateSvc.ActivateSessionTemplate(env.ctx, template.ID(), creator))
		assert.Equal(t, domain.SessionTemplateStatusActive, status(t))
		assert.Equal(t, 1, env.outboxCount(t, domain.EventActivitySessionTemplateActivated))
	})
}

func TestSessionTemplateService_DeleteSessionTemplate(t *testing.T) {
	env := setupSessionTeamTest(t)
	creator := uuid.New()
	group := env.createGroup(t, creator)
	template := env.createTemplate(t, group.ID(), creator)

	t.Run("a non-manager may not", func(t *testing.T) {
		assert.ErrorIs(t, env.templateSvc.DeleteSessionTemplate(env.ctx, template.ID(), uuid.New()), domain.ErrUnauthorized)
	})

	t.Run("the creator deletes it, and it is gone", func(t *testing.T) {
		require.NoError(t, env.templateSvc.DeleteSessionTemplate(env.ctx, template.ID(), creator))
		assert.Equal(t, 1, env.outboxCount(t, domain.EventActivitySessionTemplateDeleted))

		_, err := env.templateSvc.GetSessionTemplate(env.ctx, template.ID())
		requireAppErrorCode(t, err, "not_found")

		listed, err := env.templateSvc.ListSessionTemplates(env.ctx, application.ListSessionTemplatesParams{ActivityGroupID: testutil.Ptr(group.ID())})
		require.NoError(t, err)
		assert.Empty(t, listed)
	})

	t.Run("a deleted template can't be activated", func(t *testing.T) {
		err := env.templateSvc.ActivateSessionTemplate(env.ctx, template.ID(), creator)
		requireAppErrorCode(t, err, "not_found")
	})
}
