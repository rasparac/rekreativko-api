package persistence

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/shared/store/postgres"
	"github.com/stretchr/testify/assert"
)

func TestBuildSessionTemplateQuery(t *testing.T) {
	groupID := uuid.New()
	creatorID := uuid.New()
	status := domain.SessionTemplateStatusActive
	isRecurringTrue := true
	isRecurringFalse := false
	needsGenerationTrue := true
	lookahead := 24 * time.Hour
	cursorID := uuid.New()
	cursorTime := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		filter        SessionTemplateFilter
		expectedQuery string // substring that must be in query
		expectedArgs  []interface{}
		assertions    []func(query string) bool // additional query assertions
	}{
		{
			name:          "empty filter - base query only",
			filter:        SessionTemplateFilter{},
			expectedQuery: "WHERE deleted_at IS NULL",
			expectedArgs:  []interface{}{},
			assertions: []func(query string) bool{
				func(q string) bool { return strings.Contains(q, "ORDER BY created_at DESC, id DESC") },
				func(q string) bool { return !strings.Contains(q, "$1") },
			},
		},
		{
			name: "filter by activity group",
			filter: SessionTemplateFilter{
				ActivityGroupID: &groupID,
			},
			expectedQuery: "AND activity_group_id = $1",
			expectedArgs:  []interface{}{groupID},
		},
		{
			name: "filter by creator",
			filter: SessionTemplateFilter{
				CreatedByID: &creatorID,
			},
			expectedQuery: "AND created_by_id = $1",
			expectedArgs:  []interface{}{creatorID},
		},
		{
			name: "filter by status",
			filter: SessionTemplateFilter{
				Status: &status,
			},
			expectedQuery: "AND status = $1",
			expectedArgs:  []interface{}{"active"},
		},
		{
			name: "filter by is recurring - true",
			filter: SessionTemplateFilter{
				IsRecurring: &isRecurringTrue,
			},
			expectedQuery: "AND recurrence_frequency IS NOT NULL",
			expectedArgs:  []interface{}{},
		},
		{
			name: "filter by is recurring - false",
			filter: SessionTemplateFilter{
				IsRecurring: &isRecurringFalse,
			},
			expectedQuery: "AND recurrence_frequency IS NULL",
			expectedArgs:  []interface{}{},
		},
		{
			name: "filter needs generation with lookahead",
			filter: SessionTemplateFilter{
				NeedsGeneration: &needsGenerationTrue,
				LookaheadWindow: &lookahead,
			},
			expectedQuery: "AND status = 'active'",
			expectedArgs:  []interface{}{lookahead},
			assertions: []func(query string) bool{
				func(q string) bool { return strings.Contains(q, "AND recurrence_frequency IS NOT NULL") },
				func(q string) bool {
					return strings.Contains(q, "generated_up_to IS NULL OR generated_up_to < NOW() + $1")
				},
			},
		},
		{
			name: "filter needs generation without lookahead (default 7 days)",
			filter: SessionTemplateFilter{
				NeedsGeneration: &needsGenerationTrue,
			},
			expectedQuery: "AND status = 'active'",
			expectedArgs:  []interface{}{},
			assertions: []func(query string) bool{
				func(q string) bool { return strings.Contains(q, "AND recurrence_frequency IS NOT NULL") },
				func(q string) bool { return strings.Contains(q, "INTERVAL '7 days'") },
			},
		},
		{
			name: "pagination - limit only fetches one extra row",
			filter: SessionTemplateFilter{
				Limit: 10,
			},
			expectedQuery: "LIMIT $1",
			expectedArgs:  []interface{}{11},
		},
		{
			name: "pagination - page token resumes via keyset condition",
			filter: SessionTemplateFilter{
				Limit:     10,
				PageToken: postgres.EncodePageToken(cursorTime.Format(time.RFC3339Nano), cursorID),
			},
			expectedQuery: "AND (created_at, id) < ($1, $2)",
			expectedArgs:  []interface{}{cursorTime, cursorID, 11},
			assertions: []func(query string) bool{
				func(q string) bool { return strings.Contains(q, "AND (created_at, id) < ($1, $2)") },
				func(q string) bool { return strings.Contains(q, "LIMIT $3") },
			},
		},
		{
			name: "complex filter - multiple conditions",
			filter: SessionTemplateFilter{
				ActivityGroupID: &groupID,
				Status:          &status,
				IsRecurring:     &isRecurringTrue,
				Limit:           50,
			},
			expectedQuery: "AND activity_group_id = $1",
			expectedArgs:  []interface{}{groupID, "active", 51},
			assertions: []func(query string) bool{
				func(q string) bool { return strings.Contains(q, "AND activity_group_id = $1") },
				func(q string) bool { return strings.Contains(q, "AND status = $2") },
				func(q string) bool { return strings.Contains(q, "AND recurrence_frequency IS NOT NULL") },
				func(q string) bool { return strings.Contains(q, "LIMIT $3") },
			},
		},
		{
			name: "cron job scenario - all filters",
			filter: SessionTemplateFilter{
				ActivityGroupID: &groupID,
				NeedsGeneration: &needsGenerationTrue,
				LookaheadWindow: &lookahead,
			},
			expectedQuery: "AND activity_group_id = $1",
			expectedArgs:  []interface{}{groupID, lookahead},
			assertions: []func(query string) bool{
				func(q string) bool { return strings.Contains(q, "AND status = 'active'") },
				func(q string) bool { return strings.Contains(q, "AND recurrence_frequency IS NOT NULL") },
				func(q string) bool {
					return strings.Contains(q, "generated_up_to IS NULL OR generated_up_to < NOW() + $2")
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, args, err := buildSessionTemplateQuery(tt.filter)
			assert.NoError(t, err)

			// Check expected query substring
			assert.Contains(t, query, tt.expectedQuery,
				"Query should contain expected substring")

			// Check arguments
			assert.Equal(t, tt.expectedArgs, args,
				"Arguments should match expected")

			// Run additional assertions
			for i, assertion := range tt.assertions {
				assert.True(t, assertion(query),
					"Additional assertion %d failed", i)
			}

			// Always check base conditions
			assert.Contains(t, query, "SELECT", "Query should have SELECT")
			assert.Contains(t, query, "FROM activity.session_template", "Query should have FROM clause")
			assert.Contains(t, query, "WHERE deleted_at IS NULL", "Query should filter deleted")
			assert.Contains(t, query, "ORDER BY created_at DESC", "Query should have ORDER BY")

			// Verify parameter placeholders match args count
			placeholderCount := strings.Count(query, "$")
			assert.Equal(t, len(args), placeholderCount,
				"Number of placeholders should match number of arguments")
		})
	}
}

func TestQueryBuilder(t *testing.T) {
	t.Run("AddCondition increments parameter count correctly", func(t *testing.T) {
		qb := &postgres.QueryBuilder{
			BaseQuery: "SELECT * FROM table WHERE 1=1",
			Args:      make([]interface{}, 0),
		}

		qb.AddCondition("column1 = ", "value1")
		qb.AddCondition("column2 = ", "value2")
		qb.AddCondition("column3 = ", 123)

		query, args := qb.Build()

		assert.Contains(t, query, "AND column1 = $1")
		assert.Contains(t, query, "AND column2 = $2")
		assert.Contains(t, query, "AND column3 = $3")
		assert.Equal(t, []interface{}{"value1", "value2", 123}, args)
	})

	t.Run("AddRawCondition does not add parameters", func(t *testing.T) {
		qb := &postgres.QueryBuilder{
			BaseQuery: "SELECT * FROM table WHERE 1=1",
			Args:      make([]interface{}, 0),
		}

		qb.AddRawCondition("column IS NOT NULL")
		qb.AddCondition("id = ", uuid.New())

		query, args := qb.Build()

		assert.Contains(t, query, "AND column IS NOT NULL")
		assert.Contains(t, query, "AND id = $1")
		assert.Len(t, args, 1)
	})

	t.Run("mixing AddCondition and AddRawCondition maintains correct parameter count", func(t *testing.T) {
		qb := &postgres.QueryBuilder{
			BaseQuery: "SELECT * FROM table WHERE 1=1",
			Args:      make([]interface{}, 0),
		}

		qb.AddCondition("col1 = ", "val1")     // $1
		qb.AddRawCondition("col2 IS NOT NULL") // no param
		qb.AddCondition("col3 = ", 42)         // $2
		qb.AddRawCondition("col4 > 0")         // no param
		qb.AddCondition("col5 = ", "val5")     // $3

		query, args := qb.Build()

		assert.Contains(t, query, "col1 = $1")
		assert.Contains(t, query, "col2 IS NOT NULL")
		assert.Contains(t, query, "col3 = $2")
		assert.Contains(t, query, "col4 > 0")
		assert.Contains(t, query, "col5 = $3")
		assert.Equal(t, []interface{}{"val1", 42, "val5"}, args)
	})
}
