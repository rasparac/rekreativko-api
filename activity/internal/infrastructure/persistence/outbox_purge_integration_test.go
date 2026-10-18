//go:build integration

package persistence_test

import (
	"testing"

	"github.com/rasparac/rekreativko-api/shared/domainevent"
	testutil "github.com/rasparac/rekreativko-api/shared/testing"
)

// Published outbox rows older than the retention window are purged; pending
// and dead-lettered ones are not.
func TestOutboxPurge_Activity(t *testing.T) {
	db := testutil.NewTestDatabase(t)
	db.RunMigrationsForService()

	events := domainevent.NewDomainEventManager(db.CreateTransactionManager(), testutil.CreateLogger())

	testutil.VerifyOutboxPurge(t, db, "activity", events.DeletePublishedEvents)
}
