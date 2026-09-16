package activitycatalog

type ActivityLevel string

const (
	ActivityLevelBeginner     ActivityLevel = "beginner"
	ActivityLevelIntermediate ActivityLevel = "intermediate"
	ActivityLevelAdvanced     ActivityLevel = "advanced"
)

var validActivityLevels = map[ActivityLevel]struct{}{
	ActivityLevelBeginner:     {},
	ActivityLevelIntermediate: {},
	ActivityLevelAdvanced:     {},
}

func (l ActivityLevel) IsValid() bool {
	_, ok := validActivityLevels[l]
	return ok
}

func (l ActivityLevel) String() string {
	return string(l)
}
