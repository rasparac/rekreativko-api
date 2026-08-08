package testing

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Ptr returns a pointer to the given value
// Useful for creating pointer fields in test data
func Ptr[T any](v T) *T {
	return &v
}

// MustUUID parses a UUID or fails the test
func MustUUID(t *testing.T, s string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(s)
	require.NoError(t, err, "failed to parse UUID: %s", s)
	return id
}

// MustTime parses a time string or fails the test
func MustTime(t *testing.T, layout, value string) time.Time {
	t.Helper()
	tm, err := time.Parse(layout, value)
	require.NoError(t, err, "failed to parse time: %s", value)
	return tm
}

// AssertTimeAlmostEqual asserts that two times are equal within a delta
func AssertTimeAlmostEqual(t *testing.T, expected, actual time.Time, delta time.Duration, msgAndArgs ...any) {
	t.Helper()
	diff := expected.Sub(actual)
	if diff < 0 {
		diff = -diff
	}
	assert.LessOrEqual(t, diff, delta, msgAndArgs...)
}

// AssertUUIDNotNil asserts that a UUID is not nil/zero
func AssertUUIDNotNil(t *testing.T, id uuid.UUID, msgAndArgs ...any) {
	t.Helper()
	assert.NotEqual(t, uuid.Nil, id, msgAndArgs...)
}

// RequireNoRows asserts that a query returns no rows
func RequireNoRows(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no rows")
}

// NowUTC returns current time in UTC truncated to microseconds
// (Postgres timestamp precision)
func NowUTC() time.Time {
	return time.Now().UTC().Truncate(time.Microsecond)
}

// TimeUTC creates a time in UTC truncated to microseconds
func TimeUTC(year int, month time.Month, day, hour, min, sec int) time.Time {
	return time.Date(year, month, day, hour, min, sec, 0, time.UTC)
}
