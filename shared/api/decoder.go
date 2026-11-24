package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	entranslations "github.com/go-playground/validator/v10/translations/en"
)

// Decoder reads a JSON request body into a DTO and checks the DTO's `validate:"..."` tags.
// Build one in main (validator caches what it learns about each struct, so one is enough)
// and hand it to the HTTP handlers; it is safe for concurrent use.
type Decoder struct {
	validate   *validator.Validate
	translator ut.Translator
}

// NewDecoder builds a Decoder whose messages are the validator's English translations.
// Field names in them (and in the errors' keys) are the JSON names, which is what the
// client sent and has to fix.
func NewDecoder() (*Decoder, error) {
	v := validator.New(validator.WithRequiredStructEnabled())

	v.RegisterTagNameFunc(func(f reflect.StructField) string {
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "" || name == "-" {
			return f.Name
		}

		return name
	})

	english := en.New()
	translator, found := ut.New(english, english).GetTranslator("en")
	if !found {
		return nil, errors.New("english translator not found")
	}

	if err := entranslations.RegisterDefaultTranslations(v, translator); err != nil {
		return nil, fmt.Errorf("register validator translations: %w", err)
	}

	return &Decoder{validate: v, translator: translator}, nil
}

// MustNewDecoder is NewDecoder for tests.
func MustNewDecoder() *Decoder {
	d, err := NewDecoder()
	if err != nil {
		panic(err)
	}

	return d
}

// Decode reads at most MaxRequestBodyBytes of the request body into body, then checks
// body's validate tags. A problem with the body is a *ValidationError (JSON that does not
// parse or fit the DTO, an unknown field, a body that is too large) or one with an entry
// per failing field (keyed by JSON name, nested as "items[0].name").
func (d *Decoder) Decode(r *http.Request, body any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, MaxRequestBodyBytes))
	decoder.DisallowUnknownFields() // Reject unknown fields

	if err := decoder.Decode(body); err != nil {
		// Parse JSON decode errors into field-level details
		return parseJSONDecodeError(err)
	}

	// Well-formed JSON of the right shape: now the validate tags
	return d.validateBody(body)
}

// validateBody returns nil, or a *ValidationError with one entry per failing field. Anything
// that is not a struct (a map, a slice) has no tags to check.
func (d *Decoder) validateBody(body any) error {
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

	err := d.validate.Struct(rv.Interface())
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
		fields[fieldPath(f)] = f.Translate(d.translator)
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
