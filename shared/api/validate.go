package api

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

// validate checks the `validate:"..."` tags of the request DTOs. Field names in its
// errors are the JSON names, which is what the client sent and has to fix.
var validate = newValidator()

func newValidator() *validator.Validate {
	v := validator.New(validator.WithRequiredStructEnabled())

	v.RegisterTagNameFunc(func(f reflect.StructField) string {
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "" || name == "-" {
			return f.Name
		}

		return name
	})

	return v
}

// validateBody checks body against its validate tags and returns a *ValidationError
// with one entry per failing field (keyed by JSON name, nested as "items[0].name"),
// or nil. Anything that is not a struct (a map, a slice) has no tags to check.
func validateBody(body any) error {
	rv := reflect.ValueOf(body)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil
	}

	err := validate.Struct(rv.Interface())
	if err == nil {
		return nil
	}

	failures, ok := errors.AsType[validator.ValidationErrors](err)
	if !ok {
		// a tag the validator does not know: a bug in a DTO, not in the request
		return fmt.Errorf("validate request: %w", err)
	}

	fields := make(map[string]string, len(failures))
	for _, f := range failures {
		fields[fieldPath(f)] = validationMessage(f)
	}

	return &ValidationError{Message: "Validation failed", Fields: fields}
}

// fieldPath is the failing field's path without the DTO's own type name:
// "CreateSessionRequest.teams[0].name" -> "teams[0].name".
func fieldPath(f validator.FieldError) string {
	_, path, found := strings.Cut(f.Namespace(), ".")
	if !found {
		return f.Field()
	}

	return path
}

// validationMessage says what is wrong in words a client can show.
func validationMessage(f validator.FieldError) string {
	param := f.Param()

	switch f.Tag() {
	case "required":
		return "This field is required"
	case "min":
		return "Must be " + bound("at least", param, f)
	case "max":
		return "Must be " + bound("at most", param, f)
	case "len":
		return "Must be exactly " + param + " characters"
	case "oneof":
		return "Must be one of: " + strings.ReplaceAll(param, " ", ", ")
	case "email":
		return "Must be a valid email address"
	case "e164":
		return "Must be a phone number in international format, e.g. +381601234567"
	case "timezone":
		return "Must be a valid IANA time zone, e.g. Europe/Belgrade"
	case "hexcolor":
		return "Must be a hex colour, e.g. #FF8800"
	case "datetime":
		return "Must be a date and time in the format " + param
	default:
		return "Invalid value"
	}
}

// bound words a min/max limit: a length for text and lists, a plain number otherwise.
func bound(kind, limit string, f validator.FieldError) string {
	switch f.Kind() {
	case reflect.String:
		return kind + " " + limit + " characters"
	case reflect.Slice, reflect.Array, reflect.Map:
		return kind + " " + limit + " items"
	default:
		return kind + " " + limit
	}
}
