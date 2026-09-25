package domain

import (
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	// DefaultTeamCount is used when team config is requested without an
	// explicit team count - the common "two sides" setup.
	DefaultTeamCount = 2
	// MaxTeamCount caps how many teams a single session can be split into.
	MaxTeamCount = 8
)

var teamColorPattern = regexp.MustCompile(`^#[0-9A-F]{6}$`)

// TeamConfig is the optional team setup of a session or session template.
// A nil *TeamConfig means the session has no teams (a flat attendee pool).
//
// PlayersPerTeam is optional and deliberately independent of the session
// capacity: nil means a team has no size limit. Colors are optional too
// (e.g. shirt/bib colors) - either none, or exactly one per team.
type TeamConfig struct {
	teamCount      int
	playersPerTeam *int
	colors         []string
}

// NewTeamConfig validates and builds a team config. teamCount nil falls back
// to DefaultTeamCount. Colors are "#RRGGBB" hex strings, normalized to upper
// case.
func NewTeamConfig(teamCount *int, playersPerTeam *int, colors []string) (TeamConfig, error) {
	count := DefaultTeamCount
	if teamCount != nil {
		count = *teamCount
	}

	if count < 2 || count > MaxTeamCount {
		return TeamConfig{}, ErrInvalidTeamCount
	}

	if playersPerTeam != nil && *playersPerTeam <= 0 {
		return TeamConfig{}, ErrInvalidPlayersPerTeam
	}

	if len(colors) != 0 && len(colors) != count {
		return TeamConfig{}, ErrInvalidTeamColors
	}

	normalized := make([]string, 0, len(colors))
	for _, color := range colors {
		color = strings.ToUpper(color)
		if !teamColorPattern.MatchString(color) {
			return TeamConfig{}, ErrInvalidTeamColors
		}
		normalized = append(normalized, color)
	}

	return TeamConfig{
		teamCount:      count,
		playersPerTeam: playersPerTeam,
		colors:         normalized,
	}, nil
}

// ReconstructTeamConfig rebuilds a team config from persisted data without
// running validations.
func ReconstructTeamConfig(teamCount int, playersPerTeam *int, colors []string) TeamConfig {
	return TeamConfig{
		teamCount:      teamCount,
		playersPerTeam: playersPerTeam,
		colors:         colors,
	}
}

func (c TeamConfig) TeamCount() int {
	return c.teamCount
}

func (c TeamConfig) PlayersPerTeam() *int {
	return c.playersPerTeam
}

func (c TeamConfig) Colors() []string {
	return append([]string(nil), c.colors...)
}

// EnsureSupportedBy rejects team config for activity types that aren't team
// sports. Checked by NewSession, and by the application layer for templates
// (a template inherits its activity type from its group).
func (c TeamConfig) EnsureSupportedBy(activityType ActivityType) error {
	if !activityType.IsTeamSport() {
		return ErrTeamsNotSupported
	}
	return nil
}

// hasRoomFor reports whether a team currently holding currentSize players can
// take one more.
func (c TeamConfig) hasRoomFor(currentSize int) bool {
	if c.playersPerTeam == nil {
		return true
	}
	return currentSize < *c.playersPerTeam
}

// Team is one side of a session split into teams. Teams are created together
// with the session (Team A, Team B, ...) and live inside the Session
// aggregate; which attendee plays for which team is stored on the attendee
// (Attendee.TeamID).
type Team struct {
	id        uuid.UUID
	sessionID uuid.UUID
	name      string
	color     string // optional "#RRGGBB"
	position  int    // 0-based order, also the source of the default name
	createdAt time.Time
}

func newTeams(sessionID uuid.UUID, config TeamConfig, now time.Time) []*Team {
	teams := make([]*Team, 0, config.teamCount)
	for i := range config.teamCount {
		var color string
		if len(config.colors) > 0 {
			color = config.colors[i]
		}

		teams = append(teams, &Team{
			id:        uuid.New(),
			sessionID: sessionID,
			name:      defaultTeamName(i),
			color:     color,
			position:  i,
			createdAt: now,
		})
	}

	return teams
}

func defaultTeamName(position int) string {
	return "Team " + string(rune('A'+position))
}

// ReconstructTeam rebuilds a Team from persisted data.
func ReconstructTeam(
	id uuid.UUID,
	sessionID uuid.UUID,
	name string,
	color string,
	position int,
	createdAt time.Time,
) *Team {
	return &Team{
		id:        id,
		sessionID: sessionID,
		name:      name,
		color:     color,
		position:  position,
		createdAt: createdAt,
	}
}

func (t *Team) ID() uuid.UUID {
	return t.id
}

func (t *Team) SessionID() uuid.UUID {
	return t.sessionID
}

func (t *Team) Name() string {
	return t.name
}

func (t *Team) Color() string {
	return t.color
}

func (t *Team) Position() int {
	return t.position
}

func (t *Team) CreatedAt() time.Time {
	return t.createdAt
}
