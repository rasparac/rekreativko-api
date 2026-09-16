// Package activitycatalog is the single source of truth for the set of
// activity types and skill levels shared across bounded contexts
// (account-profile's interests, activity's groups).
package activitycatalog

type ActivityType string

const (
	ActivityTypeRunning       ActivityType = "running"
	ActivityTypeWalking       ActivityType = "walking"
	ActivityTypeJogging       ActivityType = "jogging"
	ActivityTypeBasketball    ActivityType = "basketball"
	ActivityTypeFootball      ActivityType = "football"
	ActivityTypeTennis        ActivityType = "tennis"
	ActivityTypeGym           ActivityType = "gym"
	ActivityTypeDancing       ActivityType = "dancing"
	ActivityTypeSkiing        ActivityType = "skiing"
	ActivityTypeClimbing      ActivityType = "climbing"
	ActivityTypeCycling       ActivityType = "cycling"
	ActivityTypeSwimming      ActivityType = "swimming"
	ActivityTypeHiking        ActivityType = "hiking"
	ActivityTypeYoga          ActivityType = "yoga"
	ActivityTypeWeightlifting ActivityType = "weightlifting"
	ActivityTypeOther         ActivityType = "other"
)

var validActivityTypes = map[ActivityType]struct{}{
	ActivityTypeRunning:       {},
	ActivityTypeWalking:       {},
	ActivityTypeJogging:       {},
	ActivityTypeBasketball:    {},
	ActivityTypeFootball:      {},
	ActivityTypeTennis:        {},
	ActivityTypeGym:           {},
	ActivityTypeDancing:       {},
	ActivityTypeSkiing:        {},
	ActivityTypeClimbing:      {},
	ActivityTypeCycling:       {},
	ActivityTypeSwimming:      {},
	ActivityTypeHiking:        {},
	ActivityTypeYoga:          {},
	ActivityTypeWeightlifting: {},
	ActivityTypeOther:         {},
}

func (t ActivityType) IsValid() bool {
	_, ok := validActivityTypes[t]
	return ok
}

func (t ActivityType) String() string {
	return string(t)
}
