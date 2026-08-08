//go:build integration

package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	testutil "github.com/rasparac/rekreativko-api/shared/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) *testutil.TestDatabase {
	t.Helper()

	db := testutil.NewTestDatabase(t)
	// Use the actual migration files as single source of truth
	// Migrations are co-located with the service in ./migrations/
	db.RunMigrationsForService()

	return db
}

func createTestActivityGroup(t *testing.T, db *testutil.TestDatabase) uuid.UUID {
	t.Helper()

	groupID := uuid.New()
	db.RunSQL(
		`INSERT INTO activity.activity_group (id, creator_id, title, description, activity_type) VALUES ($1, $2, $3, $4, $5)`,
		groupID, uuid.New(), "Test Group", "Test Description", "running",
	)
	return groupID
}

func TestSessionTemplateRepository_CreateAndGet(t *testing.T) {
	db := setupTestDB(t)
	groupID := createTestActivityGroup(t, db)

	txManager := db.CreateTransactionManager()
	repo := NewSessionTemplateManager(txManager, testutil.CreateLogger())

	ctx := context.Background()

	t.Run("create and retrieve simple template", func(t *testing.T) {
		// Arrange
		template := NewSessionTemplateBuilder().
			WithActivityGroup(groupID).
			WithTitle("Monday Training").
			WithDescription("Weekly training session").
			WithCapacity(25).
			WithLocation("Berlin", "DE").
			Build()

		// Act - Create
		err := repo.CreateSessionTemplate(ctx, template)
		require.NoError(t, err)

		// Act - Get
		retrieved, err := repo.GetSessionTemplateByID(ctx, template.ID())
		require.NoError(t, err)

		// Assert
		assert.Equal(t, template.ID(), retrieved.ID())
		assert.Equal(t, template.Title(), retrieved.Title())
		assert.Equal(t, template.Description(), retrieved.Description())
		assert.Equal(t, template.DefaultCapacity(), retrieved.DefaultCapacity())
		assert.Equal(t, "Berlin", retrieved.LocationCity())
		assert.Equal(t, "DE", retrieved.LocationCountry())
	})

	t.Run("create recurring template with weekly pattern", func(t *testing.T) {
		// Arrange
		template := NewSessionTemplateBuilder().
			WithActivityGroup(groupID).
			WithTitle("Weekly Yoga").
			WithWeeklyRecurrence(time.Monday, 18, 30).
			Build()

		// Act
		err := repo.CreateSessionTemplate(ctx, template)
		require.NoError(t, err)

		retrieved, err := repo.GetSessionTemplateByID(ctx, template.ID())
		require.NoError(t, err)

		// Assert
		require.NotNil(t, retrieved.RecurrenceRule())
		assert.Equal(t, domain.RecurrenceFrequencyWeekly, retrieved.RecurrenceRule().Frequency())
		assert.Equal(t, 1, retrieved.RecurrenceRule().Interval())
		assert.Equal(t, time.Monday, *retrieved.RecurrenceRule().DayOfWeek())
		assert.Equal(t, 18, retrieved.RecurrenceRule().TimeOfDay().Hour())
		assert.Equal(t, 30, retrieved.RecurrenceRule().TimeOfDay().Minute())
	})

	t.Run("create recurring template with monthly pattern", func(t *testing.T) {
		// Arrange
		template := NewSessionTemplateBuilder().
			WithActivityGroup(groupID).
			WithTitle("Monthly Meetup").
			WithMonthlyRecurrence(15, 19, 0). // 15th of month at 19:00
			Build()

		// Act
		err := repo.CreateSessionTemplate(ctx, template)
		require.NoError(t, err)

		retrieved, err := repo.GetSessionTemplateByID(ctx, template.ID())
		require.NoError(t, err)

		// Assert
		require.NotNil(t, retrieved.RecurrenceRule())
		assert.Equal(t, domain.RecurrenceFrequencyMonthly, retrieved.RecurrenceRule().Frequency())
		assert.Equal(t, 15, *retrieved.RecurrenceRule().DayOfMonth())
	})

	t.Run("get non-existent template returns error", func(t *testing.T) {
		// Act
		_, err := repo.GetSessionTemplateByID(ctx, uuid.New())

		// Assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no rows")
	})
}

func TestSessionTemplateRepository_Update(t *testing.T) {
	db := setupTestDB(t)
	groupID := createTestActivityGroup(t, db)

	txManager := db.CreateTransactionManager()
	repo := NewSessionTemplateManager(txManager, testutil.CreateLogger())

	ctx := context.Background()

	t.Run("update template fields", func(t *testing.T) {
		// Arrange - Create initial template
		template := NewSessionTemplateBuilder().
			WithActivityGroup(groupID).
			WithTitle("Original Title").
			WithCapacity(20).
			Build()

		err := repo.CreateSessionTemplate(ctx, template)
		require.NoError(t, err)

		// Act - Update
		err = template.Update(
			"Updated Title",
			"Updated Description",
			nil, // no change to recurrence
			testutil.Ptr(30),
			nil, // no change to location
		)
		require.NoError(t, err)

		err = repo.UpdateSessionTemplate(ctx, template)
		require.NoError(t, err)

		// Assert
		retrieved, err := repo.GetSessionTemplateByID(ctx, template.ID())
		require.NoError(t, err)

		assert.Equal(t, "Updated Title", retrieved.Title())
		assert.Equal(t, "Updated Description", retrieved.Description())
		assert.Equal(t, 30, *retrieved.DefaultCapacity())
	})

	t.Run("deactivate template", func(t *testing.T) {
		// Arrange
		template := NewSessionTemplateBuilder().
			WithActivityGroup(groupID).
			Build()

		err := repo.CreateSessionTemplate(ctx, template)
		require.NoError(t, err)

		// Act
		template.Deactivate()
		err = repo.UpdateSessionTemplate(ctx, template)
		require.NoError(t, err)

		// Assert
		retrieved, err := repo.GetSessionTemplateByID(ctx, template.ID())
		require.NoError(t, err)
		assert.Equal(t, domain.SessionTemplateStatusInactive, retrieved.Status())
	})
}

func TestSessionTemplateRepository_Delete(t *testing.T) {
	db := setupTestDB(t)
	groupID := createTestActivityGroup(t, db)

	txManager := db.CreateTransactionManager()
	repo := NewSessionTemplateManager(txManager, testutil.CreateLogger())

	ctx := context.Background()

	t.Run("soft delete template", func(t *testing.T) {
		// Arrange
		template := NewSessionTemplateBuilder().
			WithActivityGroup(groupID).
			Build()

		err := repo.CreateSessionTemplate(ctx, template)
		require.NoError(t, err)

		// Act
		err = repo.DeleteSessionTemplate(ctx, template.ID())
		require.NoError(t, err)

		// Assert - Should not be retrievable after deletion
		_, err = repo.GetSessionTemplateByID(ctx, template.ID())
		require.Error(t, err)
	})
}

func TestSessionTemplateRepository_Query(t *testing.T) {
	db := setupTestDB(t)
	groupID := createTestActivityGroup(t, db)

	txManager := db.CreateTransactionManager()
	repo := NewSessionTemplateManager(txManager, testutil.CreateLogger())

	ctx := context.Background()

	// Setup test data
	activeTemplate1 := NewSessionTemplateBuilder().
		WithActivityGroup(groupID).
		WithTitle("Active Template 1").
		WithWeeklyRecurrence(time.Monday, 10, 0).
		Build()

	activeTemplate2 := NewSessionTemplateBuilder().
		WithActivityGroup(groupID).
		WithTitle("Active Template 2").
		Build()

	inactiveTemplate := NewSessionTemplateBuilder().
		WithActivityGroup(groupID).
		WithTitle("Inactive Template").
		AsInactive().
		Build()

	require.NoError(t, repo.CreateSessionTemplate(ctx, activeTemplate1))
	require.NoError(t, repo.CreateSessionTemplate(ctx, activeTemplate2))
	require.NoError(t, repo.CreateSessionTemplate(ctx, inactiveTemplate))

	t.Run("query by activity group", func(t *testing.T) {
		// Act
		templates, err := repo.ListSessionTemplates(ctx, SessionTemplateFilter{
			ActivityGroupID: &groupID,
		})

		// Assert
		require.NoError(t, err)
		assert.Len(t, templates, 3)
	})

	t.Run("query by status - active only", func(t *testing.T) {
		// Arrange
		status := domain.SessionTemplateStatusActive

		// Act
		templates, err := repo.ListSessionTemplates(ctx, SessionTemplateFilter{
			ActivityGroupID: &groupID,
			Status:          &status,
		})

		// Assert
		require.NoError(t, err)
		assert.Len(t, templates, 2)
		for _, tmpl := range templates {
			assert.Equal(t, domain.SessionTemplateStatusActive, tmpl.Status())
		}
	})

	t.Run("query recurring templates only", func(t *testing.T) {
		// Arrange
		isRecurring := true

		// Act
		templates, err := repo.ListSessionTemplates(ctx, SessionTemplateFilter{
			ActivityGroupID: &groupID,
			IsRecurring:     &isRecurring,
		})

		// Assert
		require.NoError(t, err)
		assert.Len(t, templates, 1)
		assert.Equal(t, activeTemplate1.ID(), templates[0].ID())
	})

	t.Run("query with pagination", func(t *testing.T) {
		// Act
		templates, err := repo.ListSessionTemplates(ctx, SessionTemplateFilter{
			ActivityGroupID: &groupID,
			Limit:           2,
			Offset:          0,
		})

		// Assert
		require.NoError(t, err)
		assert.Len(t, templates, 2)

		// Get next page
		templatesPage2, err := repo.ListSessionTemplates(ctx, SessionTemplateFilter{
			ActivityGroupID: &groupID,
			Limit:           2,
			Offset:          2,
		})

		require.NoError(t, err)
		assert.Len(t, templatesPage2, 1)

		// Ensure different results
		assert.NotEqual(t, templates[0].ID(), templatesPage2[0].ID())
	})
}

func TestSessionTemplateRepository_FindRecurringTemplatesToGenerate(t *testing.T) {
	db := setupTestDB(t)
	groupID := createTestActivityGroup(t, db)

	txManager := db.CreateTransactionManager()
	repo := NewSessionTemplateManager(txManager, testutil.CreateLogger())

	ctx := context.Background()

	t.Run("find templates that need generation", func(t *testing.T) {
		// Arrange - Create template with weekly recurrence
		template := NewSessionTemplateBuilder().
			WithActivityGroup(groupID).
			WithWeeklyRecurrence(time.Monday, 10, 0).
			Build()

		require.NoError(t, repo.CreateSessionTemplate(ctx, template))

		// Act
		lookahead := 7 * 24 * time.Hour
		templates, err := repo.FindRecurringTemplatesToGenerate(ctx, lookahead)

		// Assert
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(templates), 1)

		// Find our template in results
		found := false
		for _, tmpl := range templates {
			if tmpl.ID() == template.ID() {
				found = true
				break
			}
		}
		assert.True(t, found, "created template should be in templates needing generation")
	})

	t.Run("inactive templates are not included", func(t *testing.T) {
		// Arrange
		inactiveTemplate := NewSessionTemplateBuilder().
			WithActivityGroup(groupID).
			WithWeeklyRecurrence(time.Tuesday, 14, 0).
			AsInactive().
			Build()

		require.NoError(t, repo.CreateSessionTemplate(ctx, inactiveTemplate))

		// Act
		lookahead := 7 * 24 * time.Hour
		templates, err := repo.FindRecurringTemplatesToGenerate(ctx, lookahead)

		// Assert
		require.NoError(t, err)

		// Inactive template should not be in results
		for _, tmpl := range templates {
			assert.NotEqual(t, inactiveTemplate.ID(), tmpl.ID())
		}
	})
}

func TestSessionTemplateRepository_UpdateGeneratedUpTo(t *testing.T) {
	db := setupTestDB(t)
	groupID := createTestActivityGroup(t, db)

	txManager := db.CreateTransactionManager()
	repo := NewSessionTemplateManager(txManager, testutil.CreateLogger())

	ctx := context.Background()

	t.Run("update generated_up_to timestamp", func(t *testing.T) {
		// Arrange
		template := NewSessionTemplateBuilder().
			WithActivityGroup(groupID).
			WithWeeklyRecurrence(time.Wednesday, 15, 30).
			Build()

		require.NoError(t, repo.CreateSessionTemplate(ctx, template))

		// Act
		newTimestamp := time.Now().UTC().Add(14 * 24 * time.Hour)
		err := repo.UpdateGeneratedUpTo(ctx, template.ID(), newTimestamp)
		require.NoError(t, err)

		// Assert - Verify by querying
		retrieved, err := repo.GetSessionTemplateByID(ctx, template.ID())
		require.NoError(t, err)

		require.NotNil(t, retrieved.GeneratedUpTo())
		testutil.AssertTimeAlmostEqual(t, newTimestamp, *retrieved.GeneratedUpTo(), time.Second)
	})
}

func TestSessionTemplateRepository_ConcurrentAccess(t *testing.T) {
	db := setupTestDB(t)
	groupID := createTestActivityGroup(t, db)

	txManager := db.CreateTransactionManager()
	repo := NewSessionTemplateManager(txManager, testutil.CreateLogger())

	ctx := context.Background()

	t.Run("concurrent creates do not conflict", func(t *testing.T) {
		// Arrange
		const concurrency = 5
		errors := make(chan error, concurrency)

		// Act - Create templates concurrently
		for i := 0; i < concurrency; i++ {
			go func(index int) {
				template := NewSessionTemplateBuilder().
					WithActivityGroup(groupID).
					WithTitle("Concurrent Template " + string(rune('A'+index))).
					Build()

				errors <- repo.CreateSessionTemplate(ctx, template)
			}(i)
		}

		// Collect results
		for i := 0; i < concurrency; i++ {
			err := <-errors
			assert.NoError(t, err)
		}

		// Assert - All templates should be created
		templates, err := repo.ListSessionTemplates(ctx, SessionTemplateFilter{
			ActivityGroupID: &groupID,
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(templates), concurrency)
	})
}
