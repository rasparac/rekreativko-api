package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestGroup(t *testing.T, visibility ActivityGroupVisibility) (*ActivityGroup, uuid.UUID) {
	t.Helper()

	creator := uuid.New()
	title, err := NewTitle("Tuesday Hoops")
	require.NoError(t, err)
	location, err := NewActivityGroupLocation("Belgrade", "RS")
	require.NoError(t, err)

	group, err := NewActivityGroup(creator, title, "Weekly game", ActivityTypeBasketball,
		location, DifficultyLevelBeginner, visibility, "Europe/Belgrade", nil)
	require.NoError(t, err)
	group.ClearEvents()

	return group, creator
}

func activeGroup(t *testing.T, visibility ActivityGroupVisibility) (*ActivityGroup, uuid.UUID) {
	t.Helper()

	group, creator := newTestGroup(t, visibility)
	require.NoError(t, group.Activate(creator))
	group.ClearEvents()

	return group, creator
}

func groupEventTypes(g *ActivityGroup) []string {
	types := make([]string, 0, len(g.Events()))
	for _, e := range g.Events() {
		types = append(types, e.GetEventType())
	}
	return types
}

func TestValueObjects_Validation(t *testing.T) {
	t.Run("title: 1 to 100 characters", func(t *testing.T) {
		_, err := NewTitle("")
		assert.Error(t, err)
		_, err = NewTitle(string(make([]byte, 101)))
		assert.Error(t, err)

		title, err := NewTitle("Five-a-side")
		require.NoError(t, err)
		assert.Equal(t, "Five-a-side", title.Value())
		_, err = NewTitle(string(make([]byte, 100)))
		assert.NoError(t, err, "100 is allowed")
	})

	t.Run("location needs a city and a country", func(t *testing.T) {
		_, err := NewActivityGroupLocation("", "RS")
		assert.Error(t, err)
		_, err = NewActivityGroupLocation("Belgrade", "")
		assert.Error(t, err)

		location, err := NewActivityGroupLocation("Belgrade", "RS")
		require.NoError(t, err)
		assert.Equal(t, "Belgrade", location.City())
		assert.Equal(t, "RS", location.Country())
	})

	t.Run("a schedule cannot end before it starts", func(t *testing.T) {
		start := time.Now()
		_, err := NewSchedule(start, start.Add(-time.Minute))
		assert.Error(t, err)

		schedule, err := NewSchedule(start, start.Add(time.Hour))
		require.NoError(t, err)
		assert.Equal(t, start, schedule.StartTime())
		assert.Equal(t, start.Add(time.Hour), schedule.EndTime())
	})

	t.Run("capacity: 1 to 1000", func(t *testing.T) {
		for _, bad := range []int{-1, 0, 1001} {
			_, err := NewCapacity(bad)
			assert.Error(t, err, bad)
		}
		for _, good := range []int{1, 50, 1000} {
			_, err := NewCapacity(good)
			assert.NoError(t, err, good)
		}
	})

	t.Run("visibility", func(t *testing.T) {
		assert.True(t, ActivityGroupVisibilityPublic.IsValid())
		assert.True(t, ActivityGroupVisibilityPrivate.IsValid())
		assert.False(t, ActivityGroupVisibility("secret").IsValid())
	})
}

func TestNewActivityGroup(t *testing.T) {
	title, _ := NewTitle("Tuesday Hoops")
	location, _ := NewActivityGroupLocation("Belgrade", "RS")

	t.Run("starts as a draft and raises created", func(t *testing.T) {
		group, err := NewActivityGroup(uuid.New(), title, "d", ActivityTypeBasketball, location,
			DifficultyLevelBeginner, ActivityGroupVisibilityPublic, "UTC", nil)
		require.NoError(t, err)

		assert.Equal(t, ActivityGroupStatusDraft, group.Status())
		assert.False(t, group.IsActive())
		assert.Equal(t, []string{EventActivityGroupCreated}, groupEventTypes(group))
	})

	t.Run("an unknown activity type, difficulty or visibility is rejected", func(t *testing.T) {
		_, err := NewActivityGroup(uuid.New(), title, "d", ActivityType("chess-boxing"), location, DifficultyLevelBeginner, ActivityGroupVisibilityPublic, "UTC", nil)
		assert.Error(t, err)
		_, err = NewActivityGroup(uuid.New(), title, "d", ActivityTypeBasketball, location, DifficultyLevel("godlike"), ActivityGroupVisibilityPublic, "UTC", nil)
		assert.Error(t, err)
		_, err = NewActivityGroup(uuid.New(), title, "d", ActivityTypeBasketball, location, DifficultyLevelBeginner, ActivityGroupVisibility("secret"), "UTC", nil)
		assert.Error(t, err)
	})
}

func TestActivityGroup_Activate(t *testing.T) {
	t.Run("the creator publishes a draft", func(t *testing.T) {
		group, creator := newTestGroup(t, ActivityGroupVisibilityPublic)

		require.NoError(t, group.Activate(creator))

		assert.True(t, group.IsActive())
		require.Len(t, group.Events(), 1)
		published, ok := group.Events()[0].(*ActivityPublishedEvent)
		require.True(t, ok, "publishing raises a published event")
		assert.Equal(t, creator, published.CreatorID)
	})

	t.Run("nobody else may", func(t *testing.T) {
		group, _ := newTestGroup(t, ActivityGroupVisibilityPublic)

		assert.ErrorIs(t, group.Activate(uuid.New()), ErrUnauthorized)
		assert.False(t, group.IsActive())
	})

	t.Run("only a draft", func(t *testing.T) {
		group, creator := activeGroup(t, ActivityGroupVisibilityPublic)

		assert.ErrorIs(t, group.Activate(creator), ErrActivityGroupNotDraft)
	})
}

func TestActivityGroup_CanRequestToJoin(t *testing.T) {
	active, _ := activeGroup(t, ActivityGroupVisibilityPublic)
	assert.True(t, active.CanRequestToJoin())

	private, _ := activeGroup(t, ActivityGroupVisibilityPrivate)
	assert.False(t, private.CanRequestToJoin(), "a private group takes no join requests")

	draft, _ := newTestGroup(t, ActivityGroupVisibilityPublic)
	assert.False(t, draft.CanRequestToJoin(), "nor does one that is not live yet")

	cancelled, creator := activeGroup(t, ActivityGroupVisibilityPublic)
	require.NoError(t, cancelled.Cancel(creator, "folded"))
	assert.False(t, cancelled.CanRequestToJoin())
}

func TestActivityGroup_IsVisibleTo(t *testing.T) {
	stranger := uuid.New()
	member := func(isMember bool) func() bool { return func() bool { return isMember } }
	mustNotAsk := func() bool {
		t.Error("membership looked up although the answer was already known")
		return false
	}

	public, _ := newTestGroup(t, ActivityGroupVisibilityPublic)
	assert.True(t, public.IsVisibleTo(stranger, mustNotAsk), "everyone sees a public group")

	private, creator := newTestGroup(t, ActivityGroupVisibilityPrivate)
	assert.True(t, private.IsVisibleTo(creator, mustNotAsk), "its creator")
	assert.True(t, private.IsVisibleTo(stranger, member(true)), "a confirmed member")
	assert.False(t, private.IsVisibleTo(stranger, member(false)), "nobody else")
}

func TestActivityGroup_Update(t *testing.T) {
	newTitle, _ := NewTitle("Wednesday Hoops")
	newLocation, _ := NewActivityGroupLocation("Novi Sad", "RS")

	update := func(g *ActivityGroup, requester uuid.UUID, activityType ActivityType, difficulty DifficultyLevel) error {
		return g.Update(requester, newTitle, "New description", activityType, newLocation, difficulty, "Europe/Vienna")
	}

	t.Run("the creator changes every field - difficulty included", func(t *testing.T) {
		group, creator := activeGroup(t, ActivityGroupVisibilityPublic)

		require.NoError(t, update(group, creator, ActivityTypeFootball, DifficultyLevelAdvanced))

		assert.Equal(t, "Wednesday Hoops", group.Title().Value())
		assert.Equal(t, "New description", group.Description())
		assert.Equal(t, ActivityTypeFootball, group.ActivityType())
		assert.Equal(t, "Novi Sad", group.Location().City())
		assert.Equal(t, DifficultyLevelAdvanced, group.DifficultyLevel(), "the new difficulty must be applied, not just validated")
		assert.Equal(t, "Europe/Vienna", group.Timezone())
		assert.Equal(t, []string{EventActivityGroupUpdated}, groupEventTypes(group))
	})

	t.Run("only the creator", func(t *testing.T) {
		group, _ := activeGroup(t, ActivityGroupVisibilityPublic)

		assert.ErrorIs(t, update(group, uuid.New(), ActivityTypeFootball, DifficultyLevelAdvanced), ErrUnauthorized)
		assert.Equal(t, "Tuesday Hoops", group.Title().Value(), "nothing changed")
	})

	t.Run("an unknown type or difficulty changes nothing", func(t *testing.T) {
		group, creator := activeGroup(t, ActivityGroupVisibilityPublic)

		assert.Error(t, update(group, creator, ActivityType("chess-boxing"), DifficultyLevelBeginner))
		assert.Error(t, update(group, creator, ActivityTypeFootball, DifficultyLevel("godlike")))
		assert.Equal(t, ActivityTypeBasketball, group.ActivityType())
		assert.Empty(t, group.Events())
	})

	t.Run("not once cancelled", func(t *testing.T) {
		group, creator := activeGroup(t, ActivityGroupVisibilityPublic)
		require.NoError(t, group.Cancel(creator, "folded"))

		assert.ErrorIs(t, update(group, creator, ActivityTypeFootball, DifficultyLevelAdvanced), ErrActivityGroupCancelled)
	})
}

func TestActivityGroup_SetVisibility(t *testing.T) {
	t.Run("the creator switches it and the old value is recorded", func(t *testing.T) {
		group, creator := activeGroup(t, ActivityGroupVisibilityPublic)

		require.NoError(t, group.SetVisibility(creator, ActivityGroupVisibilityPrivate))

		assert.Equal(t, ActivityGroupVisibilityPrivate, group.Visibility())
		assert.Equal(t, []string{EventActivityGroupVisibilityChanged}, groupEventTypes(group))
	})

	t.Run("only the creator, and only to a real value", func(t *testing.T) {
		group, creator := activeGroup(t, ActivityGroupVisibilityPublic)

		assert.ErrorIs(t, group.SetVisibility(uuid.New(), ActivityGroupVisibilityPrivate), ErrUnauthorized)
		assert.ErrorIs(t, group.SetVisibility(creator, ActivityGroupVisibility("secret")), ErrActivityGroupInvalidVisibility)
		assert.Equal(t, ActivityGroupVisibilityPublic, group.Visibility())
	})

	t.Run("only a live group", func(t *testing.T) {
		draft, creator := newTestGroup(t, ActivityGroupVisibilityPublic)

		assert.ErrorIs(t, draft.SetVisibility(creator, ActivityGroupVisibilityPrivate), ErrActivityGroupNotActive)
	})
}
