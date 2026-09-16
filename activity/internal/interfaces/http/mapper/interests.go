package mapper

import (
	"net/url"
	"strings"

	"github.com/rasparac/rekreativko-api/activity/internal/application"
)

// parseInterestsFromQuery parses the repeated "interests" query param into
// OR-matched (activity_type, difficulty_level) pairs, for discover endpoints
// that need to match several activity interests in a single call instead of
// one call per interest. Each value is either just an activity type
// ("running", matching any level) or "type:level" ("running:beginner").
// Also honors the older singular activity_type/difficulty_level params as
// one more pair, for simple single-filter callers that don't need the list
// form.
func parseInterestsFromQuery(query url.Values) []application.ActivityInterestFilter {
	var interests []application.ActivityInterestFilter

	for _, raw := range query["interests"] {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}

		parts := strings.SplitN(raw, ":", 2)
		interest := application.ActivityInterestFilter{ActivityType: strings.TrimSpace(parts[0])}
		if len(parts) == 2 {
			interest.DifficultyLevel = strings.TrimSpace(parts[1])
		}
		if interest.ActivityType == "" {
			continue
		}

		interests = append(interests, interest)
	}

	if activityType, difficultyLevel := query.Get("activity_type"), query.Get("difficulty_level"); activityType != "" || difficultyLevel != "" {
		interests = append(interests, application.ActivityInterestFilter{
			ActivityType:    activityType,
			DifficultyLevel: difficultyLevel,
		})
	}

	return interests
}
