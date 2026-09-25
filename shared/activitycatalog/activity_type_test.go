package activitycatalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestActivityType_Volleyball_IsValid(t *testing.T) {
	assert.True(t, ActivityTypeVolleyball.IsValid())
}

func TestActivityType_IsTeamSport(t *testing.T) {
	for _, teamSport := range []ActivityType{ActivityTypeBasketball, ActivityTypeFootball, ActivityTypeVolleyball} {
		assert.Truef(t, teamSport.IsTeamSport(), "%s should be a team sport", teamSport)
	}

	for _, other := range []ActivityType{ActivityTypeRunning, ActivityTypeTennis, ActivityTypeYoga, ActivityTypeOther, "unknown"} {
		assert.Falsef(t, other.IsTeamSport(), "%s should not be a team sport", other)
	}
}
