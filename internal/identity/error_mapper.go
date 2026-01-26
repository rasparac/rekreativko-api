package identity

import (
	"errors"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rasparac/rekreativko-api/internal/shared/domainerror"
)

func MapPostgresError(err *pgconn.PgError) *domainerror.AppError {
	if errors.Is(err, pgx.ErrNoRows) {
		return domainerror.NotFound(
			"not_found",
			"Not found",
			err,
		)
	}

	if pgerrcode.IsIntegrityConstraintViolation(err.Code) {
		return domainerror.ValidationError(
			"unique_violation",
			"Unique constraint violation",
			err,
		)
	}

	return domainerror.Internal()
}
