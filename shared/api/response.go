package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

const (
	SuccessMsg = "success"
	ErrorMsg   = "error"

	OkMessage = "OK"
)

type (

	// Response is a generic response
	Response[T any] struct {
		Status    string    `json:"status" example:"success"`
		Message   string    `json:"message" example:"Operation completed successfully"`
		Timestamp time.Time `json:"timestamp" example:"2020-01-01T00:00:00Z"`
		Error     *APIError `json:"error,omitempty"` // only on error
		Data      T         `json:"data,omitempty"`  // only on success
	}

	// APIError is a generic error
	APIError struct {
		Code    string         `json:"code" example:"invalid_request"`
		Message string         `json:"message" example:"Invalid request"`
		Details map[string]any `json:"details,omitempty"`
	}
)

func SuccessResponse[T any](data T, message string) Response[T] {
	return Response[T]{
		Status:    SuccessMsg,
		Message:   message,
		Data:      data,
		Timestamp: time.Now().UTC(),
	}
}

func ErrorResponse(code string, message string, details map[string]any) Response[any] {
	return Response[any]{
		Status:    ErrorMsg,
		Error:     &APIError{Code: code, Message: message, Details: details},
		Timestamp: time.Now().UTC(),
	}
}

func WriteJSONResponse[T any](w http.ResponseWriter, statusCode int, response Response[T]) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	return json.NewEncoder(w).Encode(response)
}

func WriteCreatedResponse[T any](w http.ResponseWriter, data T, message string) error {
	return WriteJSONResponse(
		w,
		http.StatusCreated,
		SuccessResponse(data, message),
	)
}

func WriteOkResponse[T any](w http.ResponseWriter, data T, message string) error {
	return WriteJSONResponse(
		w,
		http.StatusOK,
		SuccessResponse(data, message),
	)
}

func WriteError(
	w http.ResponseWriter,
	stautsCode int,
	code, message string,
	details map[string]any,
) error {
	return WriteJSONResponse(
		w,
		stautsCode,
		ErrorResponse(code, message, details),
	)
}

func WriteBadRequestResponse(
	w http.ResponseWriter,
	code, message string,
) error {
	return WriteJSONResponse(
		w,
		http.StatusBadRequest,
		ErrorResponse(code, message, nil),
	)
}

// WriteBadRequestWithDetails writes a 400 Bad Request response with field-level error details
func WriteBadRequestWithDetails(
	w http.ResponseWriter,
	code, message string,
	details map[string]any,
) error {
	return WriteJSONResponse(
		w,
		http.StatusBadRequest,
		ErrorResponse(code, message, details),
	)
}

// WriteValidationErrorResponse writes a validation error response with field details
// It checks if the error is a ValidationError and extracts field details
func WriteValidationErrorResponse(w http.ResponseWriter, err error) error {
	if validationErr, ok := errors.AsType[*ValidationError](err); ok {
		details := make(map[string]any)
		for field, msg := range validationErr.Fields {
			details[field] = msg
		}
		return WriteBadRequestWithDetails(
			w,
			"validation_error",
			validationErr.Message,
			details,
		)
	}

	// Fallback to generic bad request
	return WriteBadRequestResponse(w, "bad_request", "Invalid request body")
}

func WriteNotFoundResponse(
	w http.ResponseWriter,
	code, message string,
) error {
	return WriteJSONResponse(
		w,
		http.StatusNotFound,
		ErrorResponse(code, message, nil),
	)
}

func WriteUnauthorizedResponse(
	w http.ResponseWriter,
	code, message string,
) error {
	return WriteJSONResponse(
		w,
		http.StatusUnauthorized,
		ErrorResponse(code, message, nil),
	)
}

func WriteForbiddenResponse(
	w http.ResponseWriter,
	code, message string,
) error {
	return WriteJSONResponse(
		w,
		http.StatusForbidden,
		ErrorResponse(code, message, nil),
	)
}

func WriteInternalServerErrorResponse(w http.ResponseWriter) error {
	return WriteJSONResponse(
		w,
		http.StatusInternalServerError,
		ErrorResponse("internal_server_error", http.StatusText(http.StatusInternalServerError), nil),
	)
}
