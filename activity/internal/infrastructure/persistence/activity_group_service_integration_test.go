//go:build integration

package persistence_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
