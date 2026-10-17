package application

import (
	"bytes"
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/activity/internal/infrastructure/persistence"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/telemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// templateListRepo behaves like the real repository: it returns up to
// filter.Limit rows and a next-page token when there are more.
type templateListRepo struct {
	SessionTemplateRepository
	rows      int
	gotFilter persistence.SessionTemplateFilter
}

func (f *templateListRepo) ListSessionTemplates(_ context.Context, filter persistence.SessionTemplateFilter) ([]*domain.SessionTemplate, string, error) {
	f.gotFilter = filter

	out := make([]*domain.SessionTemplate, min(f.rows, filter.Limit))
	for i := range out {
		out[i] = &domain.SessionTemplate{}
	}

	next := ""
	if f.rows > filter.Limit {
		next = "more"
	}
	return out, next, nil
}

func newTemplateListService(repo SessionTemplateRepository, logs *bytes.Buffer) *SessionTemplateService {
	return &SessionTemplateService{
		logger:       logger.NewWithWriter("warn", "json", logs).WithName("test"),
		templateRepo: repo,
		tracer:       telemetry.Tracer(telemetry.TracerActivityService),
	}
}

func TestListSessionTemplates_IsOneCappedList(t *testing.T) {
	for name, tc := range map[string]struct {
		rows, want int
		warns      bool
	}{
		"none":                       {rows: 0, want: 0},
		"a few: everything":          {rows: 4, want: 4},
		"at the cap: everything":     {rows: MaxListedTemplates, want: MaxListedTemplates},
		"over the cap: cut and warn": {rows: MaxListedTemplates + 30, want: MaxListedTemplates, warns: true},
	} {
		t.Run(name, func(t *testing.T) {
			var logs bytes.Buffer
			repo := &templateListRepo{rows: tc.rows}
			groupID := uuid.New()
			status := "active"

			got, err := newTemplateListService(repo, &logs).ListSessionTemplates(context.Background(), ListSessionTemplatesParams{
				ActivityGroupID: &groupID,
				Status:          &status,
			})
			require.NoError(t, err)

			assert.Len(t, got, tc.want)
			assert.Equal(t, MaxListedTemplates, repo.gotFilter.Limit, "always the cap, never a client's limit")
			assert.Empty(t, repo.gotFilter.PageToken)
			assert.Equal(t, &groupID, repo.gotFilter.ActivityGroupID)
			assert.Equal(t, tc.warns, bytes.Contains(logs.Bytes(), []byte("hit the cap")))
		})
	}
}
