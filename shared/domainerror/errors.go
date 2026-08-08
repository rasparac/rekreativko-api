package domainerror

import (
	"errors"
	"net/http"
)

// Error codes for API responses
const (
	ErrCodeInternal          = "internal_error"
	ErrCodeNotFound          = "not_found"
	ErrCodeUnauthorized      = "unauthorized"
	ErrCodeForbidden         = "forbidden"
	ErrCodeBadRequest        = "bad_request"
	ErrCodeValidation        = "validation_error"
	ErrCodeConflict          = "conflict"
	ErrCodeDuplicateTemplate = "duplicate_template"
	ErrCodeInvalidGroup      = "invalid_group"
	ErrCodeTemplateNotFound  = "template_not_found"
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
		ErrCodeInternal,
		http.StatusInternalServerError,
	)
}

func InternalWithErr(err error) *AppError {
	return NewWithErr(
		http.StatusText(http.StatusInternalServerError),
		ErrCodeInternal,
		http.StatusInternalServerError,
		err,
	)
}

// InternalWithUserMessage creates an internal error with a user-friendly message
// while preserving the actual error for logging purposes
func InternalWithUserMessage(userMessage string, err error) *AppError {
	return NewWithErr(
		userMessage,
		ErrCodeInternal,
		http.StatusInternalServerError,
		err,
	)
}

func Unauthorized(code, message string, err error) *AppError {
	return NewWithErr(message, code, http.StatusUnauthorized, err)
}

func Forbidden(code, message string, err error) *AppError {
	return NewWithErr(message, code, http.StatusForbidden, err)
}

func Conflict(code, message string, err error) *AppError {
	return NewWithErr(message, code, http.StatusConflict, err)
}

func ValidationError(code, message string, err error) *AppError {
	return NewWithErr(message, code, http.StatusUnprocessableEntity, err)
}

// GetAppError extracts an AppError from the error chain.
// If the error is not an AppError, it returns a default internal server error
// with the original error wrapped for debugging purposes.
// This function never returns nil.
func GetAppError(err error) *AppError {
	if err == nil {
		return Internal()
	}

	if appErr, ok := errors.AsType[*AppError](err); ok {
		return appErr
	}

	// Return a default internal error with the original error wrapped
	return InternalWithErr(err)
}
