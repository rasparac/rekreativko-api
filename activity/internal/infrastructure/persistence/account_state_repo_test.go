package persistence

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tablesKeptOnErasure are the activity tables accountErasureSteps leaves
// alone, and why.
var tablesKeptOnErasure = map[string]string{
	"event_outbox":    "events, purged by the outbox publisher",
	"deleted_account": "records the deletion",
	"erased_account":  "records the erasure",
}

// Every table of the schema is either erased from or deliberately kept, so a
// new table holding account data cannot be forgotten by the erasure.
func TestAccountErasureSteps_CoverEveryTable(t *testing.T) {
	schema, err := os.ReadFile("migrations/000001_activity_schema.up.sql")
	require.NoError(t, err)

	tables := regexp.MustCompile(`CREATE TABLE IF NOT EXISTS activity\.(\w+)`).FindAllStringSubmatch(string(schema), -1)
	require.NotEmpty(t, tables)

	erased := map[string]struct{}{}
	deleteFrom := regexp.MustCompile(`DELETE FROM activity\.(\w+)`)
	for _, step := range accountErasureSteps {
		for _, m := range deleteFrom.FindAllStringSubmatch(step.query, -1) {
			erased[m[1]] = struct{}{}
		}
	}

	for _, m := range tables {
		table := m[1]
		_, isErased := erased[table]
		_, isKept := tablesKeptOnErasure[table]
		assert.True(t, isErased != isKept, "activity.%s must be in accountErasureSteps or tablesKeptOnErasure (exactly one)", table)
	}
}

// Each step deletes children before the parents its subqueries and foreign
// keys need: no table is deleted from after a table it references.
func TestAccountErasureSteps_DeleteChildrenFirst(t *testing.T) {
	schema, err := os.ReadFile("migrations/000001_activity_schema.up.sql")
	require.NoError(t, err)

	position := map[string]int{}
	deleteFrom := regexp.MustCompile(`^DELETE FROM activity\.(\w+)`)
	for i, step := range accountErasureSteps {
		m := deleteFrom.FindStringSubmatch(step.query)
		require.NotNil(t, m, step.name)
		if _, seen := position[m[1]]; !seen {
			position[m[1]] = i
		}
	}

	// table -> tables it references, from the schema
	table := regexp.MustCompile(`CREATE TABLE IF NOT EXISTS activity\.(\w+)\(`)
	references := regexp.MustCompile(`REFERENCES activity\.(\w+)`)
	for _, block := range strings.Split(string(schema), "CREATE TABLE")[1:] {
		m := table.FindStringSubmatch("CREATE TABLE" + block)
		if m == nil {
			continue
		}
		child := m[1]
		body := block[:strings.Index(block, ");")]
		for _, ref := range references.FindAllStringSubmatch(body, -1) {
			parent := ref[1]
			childAt, childErased := position[child]
			parentAt, parentErased := position[parent]
			if childErased && parentErased && parent != child {
				assert.Less(t, childAt, parentAt, "activity.%s is deleted from before activity.%s, which it references", child, parent)
			}
		}
	}
}
