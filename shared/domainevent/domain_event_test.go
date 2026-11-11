package domainevent

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rasparac/rekreativko-api/shared/logger"
)

// The schema is put into the SQL as is, so the writer refuses any name that
// is not a plain identifier.
func TestNewEventWriter_ChecksTheSchemaName(t *testing.T) {
	log := logger.New("error", "json")

	for _, schema := range []string{"activity", "account_profile", "identity", "notifications"} {
		_, err := NewEventWriter(nil, log, schema)
		assert.NoError(t, err, schema)
	}

	for _, schema := range []string{"", "Activity", "1activity", "activity.event_outbox", "activity; DROP TABLE x", "activity "} {
		_, err := NewEventWriter(nil, log, schema)
		assert.Error(t, err, "%q", schema)
	}

	assert.Panics(t, func() { MustNewEventWriter(nil, log, "bad schema") })
}
