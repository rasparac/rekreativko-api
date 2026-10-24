// Package activitycatalog is the single source of truth for the set of
// activity types and skill levels shared across bounded contexts
// (account-profile's interests, activity's groups).
package activitycatalog

type ActivityType string

// Types commented out are not offered in the first version; uncomment the
// constant and its catalog row to bring one back.
const (
	ActivityTypeRunning ActivityType = "running"
	// ActivityTypeWalking       ActivityType = "walking"
	// ActivityTypeJogging       ActivityType = "jogging"
	ActivityTypeBasketball ActivityType = "basketball"
	ActivityTypeFootball   ActivityType = "football"
	ActivityTypeTennis     ActivityType = "tennis"
	ActivityTypeVolleyball ActivityType = "volleyball"
	ActivityTypeGym        ActivityType = "gym"
	// ActivityTypeDancing       ActivityType = "dancing"
	// ActivityTypeSkiing        ActivityType = "skiing"
	// ActivityTypeClimbing      ActivityType = "climbing"
	ActivityTypeCycling ActivityType = "cycling"
	// ActivityTypeSwimming      ActivityType = "swimming"
	ActivityTypeHiking ActivityType = "hiking"
	// ActivityTypeYoga          ActivityType = "yoga"
	// ActivityTypeWeightlifting ActivityType = "weightlifting"
	ActivityTypeOther ActivityType = "other"
)

// sport is what the catalog knows about one activity type.
type sport struct {
	// teamSport: sessions of this type can be split into teams (team-vs-team
	// sports only).
	teamSport bool
}

// catalog has one row per activity type; add a new type here.
var catalog = map[ActivityType]sport{
	ActivityTypeRunning: {},
	// ActivityTypeWalking:       {},
	// ActivityTypeJogging:       {},
	ActivityTypeBasketball: {teamSport: true},
	ActivityTypeFootball:   {teamSport: true},
	ActivityTypeTennis:     {},
	ActivityTypeVolleyball: {teamSport: true},
	ActivityTypeGym:        {},
	// ActivityTypeDancing:       {},
	// ActivityTypeSkiing:        {},
	// ActivityTypeClimbing:      {},
	ActivityTypeCycling: {},
	// ActivityTypeSwimming:      {},
	ActivityTypeHiking: {},
	// ActivityTypeYoga:          {},
	// ActivityTypeWeightlifting: {},
	ActivityTypeOther: {},
}

func (t ActivityType) IsValid() bool {
	_, ok := catalog[t]
	return ok
}

// IsTeamSport reports whether sessions of this activity type can be split
// into teams.
func (t ActivityType) IsTeamSport() bool {
	return catalog[t].teamSport
}

func (t ActivityType) String() string {
	return string(t)
}
