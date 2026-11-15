//go:build integration

package persistence_test

import (
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

func (e *sessionTeamTestEnv) sessionsOfTemplate(t *testing.T, templateID uuid.UUID) int {
	t.Helper()

	var n int
	require.NoError(t, e.txManager.Querier(e.ctx).QueryRow(e.ctx,
		`SELECT count(*) FROM activity.session WHERE session_template_id = $1`, templateID,
	).Scan(&n))

	return n
}

// The cron job's pass over every template: active recurring ones get their
// sessions for the lookahead window, once; the rest are left alone.
func TestSessionGenerator_GenerateSessionsFromTemplates(t *testing.T) {
	env := setupSessionTeamTest(t)
	logger := testutil.CreateLogger()

	// Any 21 days hold exactly three of a weekly occurrence.
	const lookahead = 21 * 24 * time.Hour
	generator := application.NewSessionGeneratorService(
		logger, env.txManager, env.templateRepo, env.sessionRepo,
		persistence.NewActivityGroupRepository(env.txManager, logger),
		domainevent.MustNewEventWriter(env.txManager, logger, "activity"), testMetrics(), lookahead,
	)

	creator := uuid.New()
	group := env.createGroup(t, creator)
	store := func(b *persistence.SessionTemplateBuilder) *domain.SessionTemplate {
		template := b.WithActivityGroup(group.ID()).WithCreator(creator).WithTitle("Tuesday Hoops").Build()
		require.NoError(t, env.templateRepo.CreateSessionTemplate(env.ctx, template))
		return template
	}

	weekly := store(persistence.NewSessionTemplateBuilder().WithWeeklyRecurrence(time.Tuesday, 19, 0))
	inactive := store(persistence.NewSessionTemplateBuilder().WithWeeklyRecurrence(time.Friday, 19, 0).AsInactive())
	oneOff := store(persistence.NewSessionTemplateBuilder())
	// yesterday's weekday: its next occurrence is days away, past the end
	yesterday := time.Now().UTC().AddDate(0, 0, -1).Weekday()
	ended := store(persistence.NewSessionTemplateBuilder().WithWeeklyRecurrence(yesterday, 19, 0).WithRecurrenceEndDate(time.Now().Add(time.Hour)))

	require.NoError(t, generator.GenerateSessionsFromTemplates(env.ctx))

	t.Run("an active weekly template gets the window's sessions", func(t *testing.T) {
		assert.Equal(t, 3, env.sessionsOfTemplate(t, weekly.ID()))

		stored, err := env.templateRepo.GetSessionTemplateByID(env.ctx, weekly.ID())
		require.NoError(t, err)
		require.NotNil(t, stored.GeneratedUpTo())
		assert.WithinDuration(t, time.Now().Add(lookahead), *stored.GeneratedUpTo(), time.Minute)
	})

	t.Run("inactive, one-off and ended templates get none", func(t *testing.T) {
		for _, template := range []*domain.SessionTemplate{inactive, oneOff, ended} {
			assert.Zero(t, env.sessionsOfTemplate(t, template.ID()), template.ID())
		}
	})

	t.Run("the next run doesn't generate them again", func(t *testing.T) {
		require.NoError(t, generator.GenerateSessionsFromTemplates(env.ctx))
		assert.Equal(t, 3, env.sessionsOfTemplate(t, weekly.ID()))
	})

	t.Run("a manual run of a one-off or unknown template is rejected", func(t *testing.T) {
		_, err := generator.GenerateSessionsForTemplate(env.ctx, oneOff.ID())
		assert.ErrorIs(t, err, domain.ErrSessionTemplateNotRecurring)

		_, err = generator.GenerateSessionsForTemplate(env.ctx, uuid.New())
		requireAppErrorCode(t, err, "not_found")
	})
}
