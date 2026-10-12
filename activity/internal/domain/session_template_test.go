package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestTemplate(t *testing.T, rule *RecurrenceRule) *SessionTemplate {
	t.Helper()

	location, err := NewLocation("Belgrade", "RS", "Ada Ciganlija", 44.78, 20.41)
	require.NoError(t, err)

	template, err := NewSessionTemplate(uuid.New(), uuid.New(), "Tuesday Hoops", "weekly game", rule, intPtr(10), &location)
	require.NoError(t, err)
	template.ClearEvents()

	return template
}

func templateEventTypes(st *SessionTemplate) []string {
	types := make([]string, 0, len(st.Events()))
	for _, e := range st.Events() {
		types = append(types, e.GetEventType())
	}
	return types
}

func TestSessionTemplateStatus_IsValid(t *testing.T) {
	assert.True(t, SessionTemplateStatusActive.IsValid())
	assert.True(t, SessionTemplateStatusInactive.IsValid())
	assert.False(t, SessionTemplateStatus("").IsValid())
	assert.False(t, SessionTemplateStatus("paused").IsValid())
}

func TestNewSessionTemplate(t *testing.T) {
	location, err := NewLocation("Belgrade", "RS", "", 44.78, 20.41)
	require.NoError(t, err)
	rule := weekly(t, time.Tuesday, 20, 0, 1)

	t.Run("starts active, with a created event", func(t *testing.T) {
		template, err := NewSessionTemplate(uuid.New(), uuid.New(), "Tuesday Hoops", "d", &rule, intPtr(10), &location)
		require.NoError(t, err)

		assert.True(t, template.IsActive())
		assert.True(t, template.IsRecurring())
		assert.Equal(t, "Tuesday Hoops", template.Title())
		assert.Nil(t, template.GeneratedUpTo())
		assert.Equal(t, []string{EventActivitySessionTemplateCreated}, templateEventTypes(template))
	})

	t.Run("a template without a rule is allowed but not recurring", func(t *testing.T) {
		template, err := NewSessionTemplate(uuid.New(), uuid.New(), "One-off", "d", nil, nil, &location)
		require.NoError(t, err)

		assert.False(t, template.IsRecurring())
		assert.Nil(t, template.NextSessionTime(time.Now()))
	})

	t.Run("what is rejected", func(t *testing.T) {
		_, err := NewSessionTemplate(uuid.New(), uuid.New(), "", "d", &rule, nil, &location)
		assert.ErrorIs(t, err, ErrSessionTemplateTitleRequired)

		_, err = NewSessionTemplate(uuid.New(), uuid.New(), "T", "d", &rule, nil, nil)
		assert.ErrorIs(t, err, ErrSessionTemplateLocationRequired)

		for _, capacity := range []int{-5, 0} {
			_, err = NewSessionTemplate(uuid.New(), uuid.New(), "T", "d", &rule, intPtr(capacity), &location)
			assert.ErrorIs(t, err, ErrInvalidDefaultCapacity, "capacity %d: every session generated from it would fail", capacity)
		}

		_, err = NewSessionTemplate(uuid.New(), uuid.New(), "T", "d", &rule, intPtr(1), &location)
		assert.NoError(t, err, "1 is the smallest capacity")
	})
}

func TestSessionTemplate_Update(t *testing.T) {
	rule := weekly(t, time.Tuesday, 20, 0, 1)
	newRule := weekly(t, time.Friday, 19, 0, 1)
	location, _ := NewLocation("Novi Sad", "RS", "", 45.25, 19.84)

	t.Run("replaces everything it is given", func(t *testing.T) {
		template := newTestTemplate(t, &rule)

		require.NoError(t, template.Update("Friday Hoops", "new", &newRule, intPtr(12), &location))

		assert.Equal(t, "Friday Hoops", template.Title())
		assert.Equal(t, "new", template.Description())
		assert.Equal(t, time.Friday, *template.RecurrenceRule().DayOfWeek())
		assert.Equal(t, 12, *template.DefaultCapacity())
		assert.Equal(t, "Novi Sad", template.DefaultLocation().City())
		assert.Equal(t, []string{EventActivitySessionTemplateUpdated}, templateEventTypes(template))
	})

	t.Run("the same validation as creating one, and nothing changes when it fails", func(t *testing.T) {
		template := newTestTemplate(t, &rule)

		assert.ErrorIs(t, template.Update("", "d", &rule, nil, &location), ErrSessionTemplateTitleRequired)
		assert.Equal(t, "Tuesday Hoops", template.Title())
		assert.Empty(t, template.Events())
	})

	t.Run("not once deleted", func(t *testing.T) {
		template := newTestTemplate(t, &rule)
		template.Delete()

		assert.ErrorIs(t, template.Update("x", "d", &rule, nil, &location), ErrSessionTemplateDeleted)
	})
}

func TestSessionTemplate_ActivateAndDeactivate(t *testing.T) {
	rule := weekly(t, time.Tuesday, 20, 0, 1)

	t.Run("deactivating and activating again", func(t *testing.T) {
		template := newTestTemplate(t, &rule)

		require.NoError(t, template.Deactivate())
		assert.False(t, template.IsActive())
		assert.Equal(t, SessionTemplateStatusInactive, template.Status())

		require.NoError(t, template.Activate())
		assert.True(t, template.IsActive())
		assert.Equal(t, []string{EventActivitySessionTemplateDeactivated, EventActivitySessionTemplateActivated}, templateEventTypes(template))
	})

	t.Run("doing it twice is fine and raises nothing", func(t *testing.T) {
		template := newTestTemplate(t, &rule)

		require.NoError(t, template.Activate(), "already active")
		assert.Empty(t, template.Events())

		require.NoError(t, template.Deactivate())
		template.ClearEvents()
		require.NoError(t, template.Deactivate(), "already inactive")
		assert.Empty(t, template.Events())
	})

	t.Run("not once deleted", func(t *testing.T) {
		template := newTestTemplate(t, &rule)
		template.Delete()

		assert.ErrorIs(t, template.Activate(), ErrSessionTemplateDeleted)
		assert.ErrorIs(t, template.Deactivate(), ErrSessionTemplateDeleted)
	})
}

func TestSessionTemplate_Delete(t *testing.T) {
	rule := weekly(t, time.Tuesday, 20, 0, 1)
	template := newTestTemplate(t, &rule)

	template.Delete()

	assert.NotNil(t, template.DeletedAt())
	assert.Equal(t, []string{EventActivitySessionTemplateDeleted}, templateEventTypes(template))

	template.ClearEvents()
	deletedAt := *template.DeletedAt()
	template.Delete()
	assert.Equal(t, deletedAt, *template.DeletedAt(), "deleting again keeps the first time")
	assert.Empty(t, template.Events(), "and raises nothing")
}

func TestSessionTemplate_UpdateGeneratedUpTo(t *testing.T) {
	rule := weekly(t, time.Tuesday, 20, 0, 1)
	template := newTestTemplate(t, &rule)
	until := time.Now().UTC().Add(30 * 24 * time.Hour)

	require.NoError(t, template.UpdateGeneratedUpTo(until))
	assert.Equal(t, until, *template.GeneratedUpTo())

	template.Delete()
	assert.ErrorIs(t, template.UpdateGeneratedUpTo(until.Add(time.Hour)), ErrSessionTemplateDeleted)
	assert.Equal(t, until, *template.GeneratedUpTo(), "a deleted template is not generated for")
}

func TestSessionTemplate_NextSessionTime(t *testing.T) {
	rule := weekly(t, time.Tuesday, 20, 0, 1) // 2026-03-10 is a Tuesday
	template := newTestTemplate(t, &rule)

	next := template.NextSessionTime(utc(2026, 3, 10, 21, 0))

	require.NotNil(t, next)
	assert.Equal(t, utc(2026, 3, 17, 20, 0), *next)
}
