package domain

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewReport(t *testing.T) {
	reporter, target := uuid.New(), uuid.New()

	t.Run("a valid report", func(t *testing.T) {
		report, err := NewReport(reporter, ReportTargetGroup, target, ReportReasonSpam, "selling stuff")
		require.NoError(t, err)
		assert.Equal(t, ReportTargetGroup, report.TargetType())
		assert.Equal(t, "selling stuff", report.Note())
	})

	for name, tc := range map[string]struct {
		targetType ReportTargetType
		targetID   uuid.UUID
		reason     ReportReason
		note       string
		want       error
	}{
		"unknown target type":  {"comment", target, ReportReasonSpam, "", ErrInvalidReportTarget},
		"no target":            {ReportTargetUser, uuid.Nil, ReportReasonSpam, "", ErrInvalidReportTarget},
		"unknown reason":       {ReportTargetUser, target, "rude", "", ErrInvalidReportReason},
		"note over 1000 chars": {ReportTargetUser, target, ReportReasonOther, strings.Repeat("ž", 1001), ErrReportNoteTooLong},
		"reporting yourself":   {ReportTargetUser, reporter, ReportReasonOther, "", ErrCannotReportYourself},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := NewReport(reporter, tc.targetType, tc.targetID, tc.reason, tc.note)
			assert.ErrorIs(t, err, tc.want)
		})
	}

	t.Run("1000 characters is fine, counted as characters not bytes", func(t *testing.T) {
		_, err := NewReport(reporter, ReportTargetUser, target, ReportReasonOther, strings.Repeat("ž", 1000))
		assert.NoError(t, err)
	})
}

func TestNewAccountBlock(t *testing.T) {
	me := uuid.New()

	_, err := NewAccountBlock(me, me)
	assert.ErrorIs(t, err, ErrCannotBlockYourself)

	block, err := NewAccountBlock(me, uuid.New())
	require.NoError(t, err)
	assert.Equal(t, me, block.BlockerID())
}
