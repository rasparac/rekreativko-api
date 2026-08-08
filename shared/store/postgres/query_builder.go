package postgres

import "fmt"

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

// Build returns the final query and arguments.
// Call this method after adding all conditions to get the complete SQL query.
func (qb *QueryBuilder) Build() (string, []any) {
	return qb.BaseQuery, qb.Args
}
