package postgres

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestQueryBuilder_AddCondition(t *testing.T) {
	t.Run("increments parameter count correctly", func(t *testing.T) {
		qb := &QueryBuilder{
			BaseQuery: "SELECT * FROM users WHERE deleted_at IS NULL",
			Args:      make([]any, 0),
		}

		qb.AddCondition("email = ", "user@example.com")
		qb.AddCondition("age > ", 18)
		qb.AddCondition("status = ", "active")

		query, args := qb.Build()

		assert.Contains(t, query, "AND email = $1")
		assert.Contains(t, query, "AND age > $2")
		assert.Contains(t, query, "AND status = $3")
		assert.Equal(t, []any{"user@example.com", 18, "active"}, args)
		assert.Equal(t, 3, qb.ParamCount)
	})

	t.Run("handles different value types", func(t *testing.T) {
		id := uuid.New()
		qb := &QueryBuilder{
			BaseQuery: "SELECT * FROM users WHERE 1=1",
			Args:      make([]any, 0),
		}

		qb.AddCondition("id = ", id)
		qb.AddCondition("is_active = ", true)
		qb.AddCondition("score = ", 99.5)

		query, args := qb.Build()

		assert.Contains(t, query, "AND id = $1")
		assert.Contains(t, query, "AND is_active = $2")
		assert.Contains(t, query, "AND score = $3")
		assert.Equal(t, []any{id, true, 99.5}, args)
	})
}

func TestQueryBuilder_AddRawCondition(t *testing.T) {
	t.Run("does not add parameters", func(t *testing.T) {
		qb := &QueryBuilder{
			BaseQuery: "SELECT * FROM users WHERE 1=1",
			Args:      make([]any, 0),
		}

		qb.AddRawCondition("email IS NOT NULL")
		qb.AddRawCondition("created_at > NOW() - INTERVAL '7 days'")

		query, args := qb.Build()

		assert.Contains(t, query, "AND email IS NOT NULL")
		assert.Contains(t, query, "AND created_at > NOW() - INTERVAL '7 days'")
		assert.Len(t, args, 0)
		assert.Equal(t, 0, qb.ParamCount)
	})
}

func TestQueryBuilder_MixedConditions(t *testing.T) {
	t.Run("maintains correct parameter count when mixing conditions", func(t *testing.T) {
		qb := &QueryBuilder{
			BaseQuery: "SELECT * FROM orders WHERE deleted_at IS NULL",
			Args:      make([]any, 0),
		}

		qb.AddCondition("user_id = ", uuid.New())            // $1
		qb.AddRawCondition("status IN ('pending', 'paid')")  // no param
		qb.AddCondition("amount > ", 100.00)                 // $2
		qb.AddRawCondition("created_at > NOW() - INTERVAL '1 month'") // no param
		qb.AddCondition("currency = ", "USD")                // $3

		query, args := qb.Build()

		assert.Contains(t, query, "user_id = $1")
		assert.Contains(t, query, "status IN ('pending', 'paid')")
		assert.Contains(t, query, "amount > $2")
		assert.Contains(t, query, "created_at > NOW() - INTERVAL '1 month'")
		assert.Contains(t, query, "currency = $3")
		assert.Len(t, args, 3)
		assert.Equal(t, 3, qb.ParamCount)
	})
}

func TestQueryBuilder_Build(t *testing.T) {
	t.Run("returns query and args unchanged", func(t *testing.T) {
		qb := &QueryBuilder{
			BaseQuery: "SELECT * FROM users WHERE id = $1",
			Args:      []any{uuid.New()},
			ParamCount: 1,
		}

		query, args := qb.Build()

		assert.Equal(t, qb.BaseQuery, query)
		assert.Equal(t, qb.Args, args)
	})

	t.Run("can be called multiple times", func(t *testing.T) {
		qb := &QueryBuilder{
			BaseQuery: "SELECT * FROM users WHERE 1=1",
			Args:      make([]any, 0),
		}

		qb.AddCondition("status = ", "active")

		query1, args1 := qb.Build()
		query2, args2 := qb.Build()

		assert.Equal(t, query1, query2)
		assert.Equal(t, args1, args2)
	})
}

func TestQueryBuilder_EmptyBuilder(t *testing.T) {
	t.Run("works with empty builder", func(t *testing.T) {
		qb := &QueryBuilder{
			BaseQuery: "SELECT * FROM users",
			Args:      make([]any, 0),
		}

		query, args := qb.Build()

		assert.Equal(t, "SELECT * FROM users", query)
		assert.Len(t, args, 0)
	})
}

func TestEscapeLikePattern(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "escapes percent sign",
			input:    "50%",
			expected: "50\\%",
		},
		{
			name:     "escapes underscore",
			input:    "test_db",
			expected: "test\\_db",
		},
		{
			name:     "escapes backslash",
			input:    "a\\b",
			expected: "a\\\\b",
		},
		{
			name:     "escapes all special characters",
			input:    "100%_off\\sale",
			expected: "100\\%\\_off\\\\sale",
		},
		{
			name:     "handles multiple occurrences",
			input:    "%%%",
			expected: "\\%\\%\\%",
		},
		{
			name:     "handles normal text without special chars",
			input:    "New York",
			expected: "New York",
		},
		{
			name:     "handles empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "real world attack: match everything",
			input:    "%",
			expected: "\\%",
		},
		{
			name:     "real world attack: performance DoS",
			input:    "%%%%%%%%%%",
			expected: "\\%\\%\\%\\%\\%\\%\\%\\%\\%\\%",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EscapeLikePattern(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestQueryBuilder_AddLikeCondition(t *testing.T) {
	t.Run("escapes wildcards and wraps with percent", func(t *testing.T) {
		qb := &QueryBuilder{
			BaseQuery: "SELECT * FROM cities WHERE 1=1",
			Args:      make([]any, 0),
		}

		qb.AddLikeCondition("name ILIKE ", "New%York")

		query, args := qb.Build()

		assert.Contains(t, query, "AND name ILIKE $1")
		assert.Equal(t, []any{"%New\\%York%"}, args)
	})

	t.Run("prevents wildcard injection attack", func(t *testing.T) {
		qb := &QueryBuilder{
			BaseQuery: "SELECT * FROM cities WHERE 1=1",
			Args:      make([]any, 0),
		}

		// User tries to match everything by sending "%"
		qb.AddLikeCondition("name ILIKE ", "%")

		_, args := qb.Build()

		// Should search for literal "%" character, not wildcard
		assert.Equal(t, []any{"%\\%%"}, args)
	})

	t.Run("multiple like conditions with proper parameter numbering", func(t *testing.T) {
		qb := &QueryBuilder{
			BaseQuery: "SELECT * FROM activity_groups WHERE status = 'active'",
			Args:      make([]any, 0),
		}

		qb.AddLikeCondition("city ILIKE ", "San_Francisco")
		qb.AddLikeCondition("country ILIKE ", "USA")
		qb.AddCondition("capacity > ", 10)

		query, args := qb.Build()

		assert.Contains(t, query, "AND city ILIKE $1")
		assert.Contains(t, query, "AND country ILIKE $2")
		assert.Contains(t, query, "AND capacity > $3")
		assert.Equal(t, []any{"%San\\_Francisco%", "%USA%", 10}, args)
	})

	t.Run("normal search works as expected", func(t *testing.T) {
		qb := &QueryBuilder{
			BaseQuery: "SELECT * FROM cities WHERE 1=1",
			Args:      make([]any, 0),
		}

		qb.AddLikeCondition("name ILIKE ", "Paris")

		query, args := qb.Build()

		assert.Contains(t, query, "AND name ILIKE $1")
		assert.Equal(t, []any{"%Paris%"}, args)
	})
}
