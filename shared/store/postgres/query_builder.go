package postgres

import (
	"fmt"
	"strings"
)

// QueryBuilder is a helper for building dynamic SQL queries with parameterized arguments.
// It helps construct SQL queries safely by managing parameter placeholders and arguments.
//
// Example usage:
//
//	qb := &QueryBuilder{
//		BaseQuery: "SELECT * FROM users WHERE deleted_at IS NULL",
//		Args:      make([]any, 0),
//	}
//	qb.AddCondition("email = ", userEmail)
//	qb.AddRawCondition("is_active = true")
//	query, args := qb.Build()
type QueryBuilder struct {
	BaseQuery  string
	Args       []any
	ParamCount int
}

// AddCondition adds a parameterized condition to the query.
//
// WARNING: The condition parameter must be a hardcoded SQL fragment, never user input.
// Only the value parameter is safely parameterized.
//
// Example:
//
//	qb.AddCondition("user_id = ", userID)  // Safe: generates "AND user_id = $1"
//
// Never do:
//
//	qb.AddCondition(userInput + " = ", value)  // UNSAFE: SQL injection risk!
func (qb *QueryBuilder) AddCondition(condition string, value any) {
	qb.ParamCount++
	qb.BaseQuery += fmt.Sprintf(" AND %s$%d", condition, qb.ParamCount)
	qb.Args = append(qb.Args, value)
}

// AddRawCondition adds a raw SQL condition without parameters.
//
// WARNING: The condition parameter must be a hardcoded SQL fragment, NEVER user input.
// For user-provided values, use AddCondition() instead.
//
// Example:
//
//	qb.AddRawCondition("is_active = true")  // Safe: hardcoded
//	qb.AddRawCondition("created_at > NOW()")  // Safe: hardcoded
//
// Never do:
//
//	qb.AddRawCondition("status = '" + userInput + "'")  // UNSAFE: SQL injection risk!
func (qb *QueryBuilder) AddRawCondition(condition string) {
	qb.BaseQuery += " AND " + condition
}

// AddLikeCondition adds a LIKE/ILIKE condition with proper wildcard escaping.
// Automatically escapes special LIKE characters (%, _, \) in user input and wraps with %.
//
// Security: Prevents wildcard injection attacks where users could send patterns like
// "%" to match everything or "%%%...%" to cause performance issues.
//
// Example:
//
//	qb.AddLikeCondition("city ILIKE ", userCity)
//	// User sends: "New%York" -> searches for literal "New%York"
//	// User sends: "New York" -> searches for "New York"
//	// Generates: AND city ILIKE $1 with arg "%New\\%York%"
func (qb *QueryBuilder) AddLikeCondition(condition string, value string) {
	escapedValue := EscapeLikePattern(value)
	qb.AddCondition(condition, "%"+escapedValue+"%")
}

// EscapeLikePattern escapes special characters in LIKE patterns to prevent wildcard injection.
// Escapes: \ (backslash), % (percent), _ (underscore)
//
// This prevents users from injecting wildcards that could:
// 1. Match unintended data (security issue)
// 2. Cause expensive queries (performance attack)
//
// Example:
//
//	EscapeLikePattern("50%")      -> "50\\%"     (literal percent)
//	EscapeLikePattern("test_db")  -> "test\\_db" (literal underscore)
//	EscapeLikePattern("a\\b")     -> "a\\\\b"    (literal backslash)
func EscapeLikePattern(s string) string {
	// Order matters: escape backslash first, then % and _
	s = strings.ReplaceAll(s, "\\", "\\\\") // \ -> \\
	s = strings.ReplaceAll(s, "%", "\\%")   // % -> \%
	s = strings.ReplaceAll(s, "_", "\\_")   // _ -> \_
	return s
}

// Build returns the final query and arguments.
// Call this method after adding all conditions to get the complete SQL query.
func (qb *QueryBuilder) Build() (string, []any) {
	return qb.BaseQuery, qb.Args
}
