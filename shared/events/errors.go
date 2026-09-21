package events

import (
	"errors"
	"fmt"
)

// ErrPermanent marks a handler error that retrying can never fix (e.g. a
// payload that fails to decode). The broker dead-letters such messages on the
// first delivery instead of burning every retry on them.
var ErrPermanent = errors.New("permanent error")

// Permanent wraps err so that errors.Is(err, ErrPermanent) is true. It
// returns nil for a nil err.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", ErrPermanent, err)
}
