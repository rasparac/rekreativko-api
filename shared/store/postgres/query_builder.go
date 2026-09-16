package postgres

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
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

// AddKeysetCondition adds a keyset-pagination resume condition:
// "AND (sortColumn, id) > ($n, $n+1)" (or "<" for descending order), using
// Postgres row comparison so a single predicate captures both "strictly past
// the last sort value" and "same sort value, but past the last id" (the
// tiebreaker for rows that sort equally). direction must match the query's
// own ORDER BY direction exactly, or results will be silently wrong.
//
// sortColumn must be a hardcoded SQL fragment, never user input - same rule as AddCondition.
func (qb *QueryBuilder) AddKeysetCondition(sortColumn string, direction string, sortValue any, id uuid.UUID) {
	op := ">"
	if direction == "DESC" {
		op = "<"
	}

	qb.ParamCount++
	sortParam := qb.ParamCount
	qb.ParamCount++
	idParam := qb.ParamCount

	qb.BaseQuery += fmt.Sprintf(" AND (%s, id) %s ($%d, $%d)", sortColumn, op, sortParam, idParam)
	qb.Args = append(qb.Args, sortValue, id)
}

// InterestPair is one (activity type, difficulty level) combination to
// OR-match via AddInterestsCondition. An empty DifficultyLevel matches any
// level for that ActivityType.
type InterestPair struct {
	ActivityType    string
	DifficultyLevel string
}

// AddInterestsCondition adds "AND (pair1 OR pair2 OR ...)", one clause per
// interest pair, so a caller with several activity interests (e.g. running
// at any level, cycling at advanced only) gets everything matching *any* of
// them in a single query - instead of the caller needing to run one query
// per interest and merge/dedupe/paginate the results themselves.
//
// typeColumn and levelColumn must be hardcoded SQL fragments, never user
// input - same rule as AddCondition. A no-op if interests is empty.
func (qb *QueryBuilder) AddInterestsCondition(typeColumn, levelColumn string, interests []InterestPair) {
	clauses := make([]string, 0, len(interests))

	for _, interest := range interests {
		if interest.ActivityType == "" {
			continue
		}

		qb.ParamCount++
		typeParam := qb.ParamCount

		if interest.DifficultyLevel == "" {
			clauses = append(clauses, fmt.Sprintf("%s = $%d", typeColumn, typeParam))
			qb.Args = append(qb.Args, interest.ActivityType)
			continue
		}

		qb.ParamCount++
		levelParam := qb.ParamCount
		clauses = append(clauses, fmt.Sprintf("(%s = $%d AND %s = $%d)", typeColumn, typeParam, levelColumn, levelParam))
		qb.Args = append(qb.Args, interest.ActivityType, interest.DifficultyLevel)
	}

	if len(clauses) == 0 {
		return
	}

	qb.BaseQuery += " AND (" + strings.Join(clauses, " OR ") + ")"
}
