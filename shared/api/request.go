package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ValidationError represents field-level validation errors
type ValidationError struct {
	Message string
	Fields  map[string]string // field -> error message
}

func (e *ValidationError) Error() string {
	return e.Message
}

// DecodeJSONBody decodes the request body and returns detailed validation errors
func DecodeJSONBody(r *http.Request, body any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields() // Reject unknown fields

	err := decoder.Decode(body)
	if err == nil {
		return nil
	}

	// Parse JSON decode errors into field-level details
	return parseJSONDecodeError(err)
}

// parseJSONDecodeError converts JSON decode errors into structured ValidationError
func parseJSONDecodeError(err error) error {
	// Syntax errors (malformed JSON)
	if syntaxError, ok := errors.AsType[*json.SyntaxError](err); ok {
		return &ValidationError{
			Message: "Invalid JSON syntax",
			Fields: map[string]string{
				"_json": fmt.Sprintf("Syntax error at position %d", syntaxError.Offset),
			},
		}
	}

	// Type mismatch errors (e.g., string provided where int expected)
	if unmarshalTypeError, ok := errors.AsType[*json.UnmarshalTypeError](err); ok {
		fieldName := unmarshalTypeError.Field
		if fieldName == "" {
			fieldName = "unknown"
		}
		return &ValidationError{
			Message: "Invalid field type",
			Fields: map[string]string{
				fieldName: fmt.Sprintf("Expected %s but got %s", unmarshalTypeError.Type.String(), unmarshalTypeError.Value),
			},
		}
	}

	// Unknown fields error
	if strings.Contains(err.Error(), "unknown field") {
		// Extract field name from error message: "json: unknown field \"foo\""
		msg := err.Error()
		if start := strings.Index(msg, "\""); start != -1 {
			if end := strings.Index(msg[start+1:], "\""); end != -1 {
				fieldName := msg[start+1 : start+1+end]
				return &ValidationError{
					Message: "Unknown field in request",
					Fields: map[string]string{
						fieldName: "This field is not recognized",
					},
				}
			}
		}
		return &ValidationError{
			Message: "Unknown field in request",
			Fields: map[string]string{
				"_request": err.Error(),
			},
		}
	}

	// EOF error (empty body)
	if errors.Is(err, io.EOF) {
		return &ValidationError{
			Message: "Request body is empty",
			Fields: map[string]string{
				"_body": "Request body cannot be empty",
			},
		}
	}

	// Unexpected EOF (incomplete JSON)
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return &ValidationError{
			Message: "Incomplete JSON",
			Fields: map[string]string{
				"_json": "JSON is incomplete or malformed",
			},
		}
	}

	// Default case
	return &ValidationError{
		Message: "Invalid request body",
		Fields: map[string]string{
			"_request": err.Error(),
		},
	}
}
