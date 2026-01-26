package domainerror

import (
	"errors"
	"net/http"
)

type AppError struct {
	Message    string `json:"message"`
	Code       string `json:"code"`
	StatusCode int    `json:"-"`
	Err        error  `json:"-"`
}

func New(message, code string, statusCode int) *AppError {
	return &AppError{
		Message:    message,
		Code:       code,
		StatusCode: statusCode,
	}
}

func NewWithErr(message, code string, statusCode int, err error) *AppError {
	return &AppError{
		Message:    message,
		Code:       code,
		StatusCode: statusCode,
		Err:        err,
	}
}

func (e *AppError) Error() string {
	if e.Err == nil {
		return e.Message
	}

	return e.Err.Error()
}

func (e *AppError) Unwrap() error {
	return e.Err
}

func BadRequest(code, message string, err error) *AppError {
	return NewWithErr(message, code, http.StatusBadRequest, err)
}

func NotFound(code, message string, err error) *AppError {
	return NewWithErr(message, code, http.StatusNotFound, err)
}

func Internal() *AppError {
	return New(
		http.StatusText(http.StatusInternalServerError),
		"internal_error",
		http.StatusInternalServerError,
	)
}

func InternalWithErr(err error) *AppError {
	return NewWithErr(
		http.StatusText(http.StatusInternalServerError),
		"internal_error",
		http.StatusInternalServerError,
		err,
	)
}

func Unauthorized(code, message string, err error) *AppError {
	return New(message, code, http.StatusUnauthorized)
}

func Forbidden(code, message string, err error) *AppError {
	return New(message, code, http.StatusForbidden)
}

func Conflict(code, message string, err error) *AppError {
	return New(message, code, http.StatusConflict)
}

func ValidationError(code, message string, err error) *AppError {
	return NewWithErr(message, code, http.StatusUnprocessableEntity, err)
}

func IsAppError(err error) bool {
	var appErr *AppError
	return errors.Is(err, appErr)
}

func GetAppError(err error) *AppError {
	var appErr *AppError

	if errors.As(err, &appErr) {
		return appErr
	}

	return nil
}
