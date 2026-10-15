package application

import (
	"testing"

	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestVisibleStatuses(t *testing.T) {
	var (
		going    = domain.AttendeeStatusGoing
		promoted = domain.AttendeeStatusPromoted
		maybe    = domain.AttendeeStatusMaybe
		pending  = domain.AttendeeStatusPending
		notGoing = domain.AttendeeStatusNotGoing
	)

	for name, tc := range map[string]struct {
		requested []domain.AttendeeStatus
		want      []domain.AttendeeStatus
	}{
		"nothing asked: every public status":       {nil, []domain.AttendeeStatus{going, promoted, maybe}},
		"public statuses are kept, in order asked": {[]domain.AttendeeStatus{maybe, going}, []domain.AttendeeStatus{maybe, going}},
		"managers-only statuses are dropped":       {[]domain.AttendeeStatus{going, pending, notGoing}, []domain.AttendeeStatus{going}},
		"only managers-only statuses: nothing":     {[]domain.AttendeeStatus{pending, notGoing}, []domain.AttendeeStatus{}},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, visibleStatuses(tc.requested))
		})
	}
}
