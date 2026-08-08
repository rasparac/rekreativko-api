package application

import (
	"errors"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/shared/domainerror"
)

// MapErrToAppError maps domain errors to application errors
func MapErrToAppError(err error) *domainerror.AppError {
	if err == nil {
		return nil
	}

	// Session Template errors
	switch {
	case errors.Is(err, domain.ErrSessionTemplateNotFound):
		return domainerror.NotFound("session_template_not_found", "Session template not found", err)
	case errors.Is(err, domain.ErrSessionTemplateNotActive):
		return domainerror.ValidationError("session_template_not_active", "Session template is not active", err)
	case errors.Is(err, domain.ErrSessionTemplateDeleted):
		return domainerror.NotFound("session_template_deleted", "Session template has been deleted", err)
	case errors.Is(err, domain.ErrSessionTemplateTitleRequired):
		return domainerror.ValidationError("title_required", "Session template title is required", err)
	case errors.Is(err, domain.ErrInvalidDefaultCapacity):
		return domainerror.ValidationError("invalid_capacity", "Default capacity must be greater than 0", err)
	case errors.Is(err, domain.ErrInvalidRecurrenceFrequency):
		return domainerror.ValidationError("invalid_recurrence_frequency", "Invalid recurrence frequency", err)
	case errors.Is(err, domain.ErrSessionTemplateInvalidRecurrenceRule):
		return domainerror.ValidationError("invalid_recurrence_rule", "Invalid recurrence rule for session template", err)
	}

	// Recurrence errors
	switch {
	case errors.Is(err, domain.ErrInvalidRecurrenceRule):
		return domainerror.ValidationError("invalid_recurrence_rule", "Invalid recurrence rule", err)
	case errors.Is(err, domain.ErrRecurrenceRuleMissingInterval):
		return domainerror.ValidationError("missing_interval", "Interval must be specified for recurring activity", err)
	case errors.Is(err, domain.ErrRecurrenceRuleMissingTimeOfDay):
		return domainerror.ValidationError("missing_time_of_day", "Time of day must be specified for recurring activity", err)
	case errors.Is(err, domain.ErrRecurrenceRuleInvalidDayOfWeek):
		return domainerror.ValidationError("invalid_day_of_week", "Day of week must be between 0 (Sunday) and 6 (Saturday)", err)
	case errors.Is(err, domain.ErrRecurrenceRuleInvalidDayOfMonth):
		return domainerror.ValidationError("invalid_day_of_month", "Day of month must be between 1 and 31", err)
	case errors.Is(err, domain.ErrRecurrenceRuleMissingDayOfMonth):
		return domainerror.ValidationError("missing_day_of_month", "Day of month must be specified for monthly recurrence", err)
	case errors.Is(err, domain.ErrRecurrenceEndDateInPast):
		return domainerror.ValidationError("end_date_in_past", "Recurrence end date cannot be in the past", err)
	case errors.Is(err, domain.ErrInvalidTimeOfDayHour):
		return domainerror.ValidationError("invalid_hour", "Invalid time of day hour, must be between 0 and 23", err)
	case errors.Is(err, domain.ErrInvalidTimeOfDayMinute):
		return domainerror.ValidationError("invalid_minute", "Invalid time of day minute, must be between 0 and 59", err)
	}

	// Database errors
	if errors.Is(err, pgx.ErrNoRows) {
		return domainerror.NotFound("not_found", "Resource not found", err)
	}

	// Activity Group errors (in case they're relevant)
	switch {
	case errors.Is(err, domain.ErrActivityGroupNotFound):
		return domainerror.NotFound("activity_group_not_found", "Activity group not found", err)
	case errors.Is(err, domain.ErrActivityGroupDeleted):
		return domainerror.NotFound("activity_group_deleted", "Activity group has been deleted", err)
	case errors.Is(err, domain.ErrUnauthorized):
		return domainerror.Unauthorized("unauthorized", "Unauthorized to perform this action", err)
	}

	// Default to internal error with wrapped error for debugging
	return domainerror.InternalWithErr(err)
}

// MapPostgresError maps Postgres errors to application errors with detailed constraint information
func MapPostgresError(err *pgconn.PgError) *domainerror.AppError {
	// Note: pgx.ErrNoRows is not a *pgconn.PgError, so it should be checked
	// before calling this function

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
		// Examples: "session_templates_title_key" -> "title"
		//           "accounts_email_unique" -> "email"
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
