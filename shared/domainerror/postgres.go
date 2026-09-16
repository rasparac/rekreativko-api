package domainerror

import (
	"strings"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
)

// MapPostgresError maps a Postgres error to an AppError with a message safe
// to return to a client - it never leaks Postgres implementation details
// such as raw constraint/index names or full row contents.
func MapPostgresError(err *pgconn.PgError) *AppError {
	switch err.Code {
	case pgerrcode.UniqueViolation:
		return Conflict("unique_violation", formatConstraintMessage(err, "already exists"), err)
	case pgerrcode.ForeignKeyViolation:
		return ValidationError("foreign_key_violation", formatConstraintMessage(err, "references invalid or missing record"), err)
	case pgerrcode.NotNullViolation:
		return ValidationError("not_null_violation", formatConstraintMessage(err, "is required"), err)
	case pgerrcode.CheckViolation:
		return ValidationError("check_violation", formatConstraintMessage(err, "violates check constraint"), err)
	}

	if pgerrcode.IsIntegrityConstraintViolation(err.Code) {
		return ValidationError("constraint_violation", formatConstraintMessage(err, "violates database constraint"), err)
	}

	return InternalWithErr(err)
}

// formatConstraintMessage builds a "<field> <suffix>" message. It only ever
// names a field it can identify with confidence - it does not fall back to
// the raw constraint/index name (e.g. "accounts_email_uq_idx") or to
// Postgres' raw Detail text (which can include full row contents for
// not-null/check violations), since both are internal implementation
// details, not something a client should see.
func formatConstraintMessage(err *pgconn.PgError, suffix string) string {
	if field := fieldName(err); field != "" {
		return field + " " + suffix
	}

	return "Provided value " + suffix
}

func fieldName(err *pgconn.PgError) string {
	if err.ColumnName != "" {
		return humanize(err.ColumnName)
	}

	// Unique/foreign-key violations carry the real column name(s) in Detail,
	// e.g. `Key (email)=(foo@bar.com) already exists.` - reading it directly
	// is more reliable than guessing from the constraint/index name, and
	// works regardless of naming convention.
	if field := fieldFromDetail(err.Detail); field != "" {
		return humanize(field)
	}

	return fieldFromConstraintName(err.ConstraintName, err.TableName)
}

func fieldFromDetail(detail string) string {
	const marker = "Key ("
	start := strings.Index(detail, marker)
	if start == -1 {
		return ""
	}
	start += len(marker)

	end := strings.Index(detail[start:], ")")
	if end == -1 {
		return ""
	}

	return detail[start : start+end]
}

func fieldFromConstraintName(constraintName, tableName string) string {
	if constraintName == "" || tableName == "" || !strings.HasPrefix(constraintName, tableName+"_") {
		// Nothing to safely strip - don't risk leaking the raw constraint
		// name, it may not follow the usual table_column_suffix convention.
		return ""
	}

	name := strings.TrimPrefix(constraintName, tableName+"_")

	for _, sfx := range []string{"_uq_idx", "_idx", "_fkey", "_key", "_check", "_unique"} {
		if strings.HasSuffix(name, sfx) {
			name = strings.TrimSuffix(name, sfx)
			break
		}
	}

	if name == "" {
		return ""
	}

	return humanize(name)
}

func humanize(s string) string {
	return strings.ReplaceAll(s, "_", " ")
}
