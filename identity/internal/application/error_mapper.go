package application

import (
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rasparac/rekreativko-api/shared/domainerror"
)

// MapPostgresError maps Postgres errors to application errors with detailed constraint information
func MapPostgresError(err *pgconn.PgError) *domainerror.AppError {
	// Note: pgx.ErrNoRows is not a *pgconn.PgError, so it should be checked
	// before calling this function (in domain.MapErrToAppError)

	// Unique constraint violations
	if err.Code == pgerrcode.UniqueViolation {
		return domainerror.Conflict(
			"unique_violation",
			formatConstraintMessage(err, "already exists"),
			err,
		)
	}

	// Foreign key violations
	if err.Code == pgerrcode.ForeignKeyViolation {
		return domainerror.ValidationError(
			"foreign_key_violation",
			formatConstraintMessage(err, "references invalid or missing record"),
			err,
		)
	}

	// Not-null constraint violations
	if err.Code == pgerrcode.NotNullViolation {
		return domainerror.ValidationError(
			"not_null_violation",
			formatConstraintMessage(err, "is required"),
			err,
		)
	}

	// Check constraint violations
	if err.Code == pgerrcode.CheckViolation {
		return domainerror.ValidationError(
			"check_violation",
			formatConstraintMessage(err, "violates check constraint"),
			err,
		)
	}

	// Other integrity constraint violations
	if pgerrcode.IsIntegrityConstraintViolation(err.Code) {
		return domainerror.ValidationError(
			"constraint_violation",
			formatConstraintMessage(err, "violates database constraint"),
			err,
		)
	}

	// Catch-all for other Postgres errors
	return domainerror.InternalWithErr(err)
}

// formatConstraintMessage creates a user-friendly message from Postgres error details
func formatConstraintMessage(err *pgconn.PgError, defaultSuffix string) string {
	// Try to extract column name from constraint name
	// Common patterns: table_column_key, table_column_check, etc.
	if err.ConstraintName != "" {
		// Remove common suffixes to get a cleaner field name
		fieldName := err.ConstraintName

		// Try to extract meaningful field name from constraint
		// Examples: "accounts_email_key" -> "email"
		//           "refresh_tokens_account_id_fkey" -> "account_id"
		if err.ColumnName != "" {
			fieldName = err.ColumnName
		}

		return fieldName + " " + defaultSuffix
	}

	// Fallback to column name if available
	if err.ColumnName != "" {
		return err.ColumnName + " " + defaultSuffix
	}

	// Final fallback to generic message with detail if available
	if err.Detail != "" {
		return err.Detail
	}

	return "Database constraint violation"
}
