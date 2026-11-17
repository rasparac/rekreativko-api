package application

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"testing"

	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapErrToAppError_WeeklyRuleWithoutADayOfWeek(t *testing.T) {
	appErr := MapErrToAppError(domain.ErrRecurrenceRuleMissingDayOfWeek)

	require.NotNil(t, appErr)
	assert.Equal(t, "missing_day_of_week", appErr.Code)
}

func TestMapErrToAppError_ProposalLimitsSayWhichAndHowMany(t *testing.T) {
	tests := []struct {
		err   error
		code  string
		limit int
	}{
		{domain.ErrRoundProposalLimit, "round_proposal_limit", domain.MaxProposalsPerRound},
		{domain.ErrAuthorProposalLimit, "author_proposal_limit", domain.MaxProposalsPerAuthor},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			appErr := MapErrToAppError(fmt.Errorf("propose: %w", tt.err))

			require.NotNil(t, appErr)
			assert.Equal(t, tt.code, appErr.Code)
			assert.Equal(t, http.StatusConflict, appErr.StatusCode)
			assert.Equal(t, tt.limit, appErr.Details["limit"])
		})
	}
}

// Every sentinel in domain/errors.go answers something other than 500: a new
// one without a case in MapErrToAppError fails here, not in production.
func TestMapErrToAppError_EveryDomainErrorIsMapped(t *testing.T) {
	sentinels := map[string]error{
		"ErrActivityGroupNotFound":           domain.ErrActivityGroupNotFound,
		"ErrActivityGroupNotDraft":           domain.ErrActivityGroupNotDraft,
		"ErrActivityGroupCancelled":          domain.ErrActivityGroupCancelled,
		"ErrActivityGroupDeleted":            domain.ErrActivityGroupDeleted,
		"ErrActivityGroupNotActive":          domain.ErrActivityGroupNotActive,
		"ErrActivityGroupFull":               domain.ErrActivityGroupFull,
		"ErrActivityGroupNotJoinable":        domain.ErrActivityGroupNotJoinable,
		"ErrInvalidScheduleTime":             domain.ErrInvalidScheduleTime,
		"ErrActivityGroupInvalidVisibility":  domain.ErrActivityGroupInvalidVisibility,
		"ErrUnauthorized":                    domain.ErrUnauthorized,
		"ErrInvalidActivityType":             domain.ErrInvalidActivityType,
		"ErrInvalidDifficultyLevel":          domain.ErrInvalidDifficultyLevel,
		"ErrTitleRequired":                   domain.ErrTitleRequired,
		"ErrTitleTooLong":                    domain.ErrTitleTooLong,
		"ErrInvalidCapacity":                 domain.ErrInvalidCapacity,
		"ErrLocationCityRequired":            domain.ErrLocationCityRequired,
		"ErrLocationCountryRequired":         domain.ErrLocationCountryRequired,
		"ErrLocationLatitudeInvalid":         domain.ErrLocationLatitudeInvalid,
		"ErrLocationLongitudeInvalid":        domain.ErrLocationLongitudeInvalid,
		"ErrAccountDeleted":                  domain.ErrAccountDeleted,
		"ErrMemberNotFound":                  domain.ErrMemberNotFound,
		"ErrMemberAlreadyAdmin":              domain.ErrMemberAlreadyAdmin,
		"ErrMemberNotAdmin":                  domain.ErrMemberNotAdmin,
		"ErrAlreadyParticipating":            domain.ErrAlreadyParticipating,
		"ErrRemovedFromGroup":                domain.ErrRemovedFromGroup,
		"ErrCreatorCannotLeave":              domain.ErrCreatorCannotLeave,
		"ErrNotConfirmed":                    domain.ErrNotConfirmed,
		"ErrNotPendingStatus":                domain.ErrNotPendingStatus,
		"ErrCannotRemoveCreator":             domain.ErrCannotRemoveCreator,
		"ErrCannotDemoteCreator":             domain.ErrCannotDemoteCreator,
		"ErrMemberAlreadyPriority":           domain.ErrMemberAlreadyPriority,
		"ErrMemberNotPriority":               domain.ErrMemberNotPriority,
		"ErrInsufficientRole":                domain.ErrInsufficientRole,
		"ErrInvalidMemberRole":               domain.ErrInvalidMemberRole,
		"ErrInviteNotFound":                  domain.ErrInviteNotFound,
		"ErrInviteExpired":                   domain.ErrInviteExpired,
		"ErrInviteHasNotExpiredYet":          domain.ErrInviteHasNotExpiredYet,
		"ErrInviteAlreadyProcessed":          domain.ErrInviteAlreadyProcessed,
		"ErrAlreadyInvited":                  domain.ErrAlreadyInvited,
		"ErrCannotInviteCreator":             domain.ErrCannotInviteCreator,
		"ErrUserAlreadyMember":               domain.ErrUserAlreadyMember,
		"ErrSessionNotFound":                 domain.ErrSessionNotFound,
		"ErrSessionNotScheduled":             domain.ErrSessionNotScheduled,
		"ErrSessionNotStarted":               domain.ErrSessionNotStarted,
		"ErrSessionCanceled":                 domain.ErrSessionCanceled,
		"ErrSessionCompleted":                domain.ErrSessionCompleted,
		"ErrSessionNotExpiredYet":            domain.ErrSessionNotExpiredYet,
		"ErrSessionAlreadyStarted":           domain.ErrSessionAlreadyStarted,
		"ErrInvalidSessionCapacity":          domain.ErrInvalidSessionCapacity,
		"ErrSessionStartTimeInPast":          domain.ErrSessionStartTimeInPast,
		"ErrSessionLocationCityRequired":     domain.ErrSessionLocationCityRequired,
		"ErrSessionLocationCountryRequired":  domain.ErrSessionLocationCountryRequired,
		"ErrSessionLocationLatitudeInvalid":  domain.ErrSessionLocationLatitudeInvalid,
		"ErrSessionLocationLongitudeInvalid": domain.ErrSessionLocationLongitudeInvalid,
		"ErrInvalidSessionVisibility":        domain.ErrInvalidSessionVisibility,
		"ErrSessionFull":                     domain.ErrSessionFull,
		"ErrInvalidDiscoveryRadius":          domain.ErrInvalidDiscoveryRadius,
		"ErrSessionNotOpen":                  domain.ErrSessionNotOpen,
		"ErrSessionNotStandalone":            domain.ErrSessionNotStandalone,
		"ErrAttendeeNotFound":                domain.ErrAttendeeNotFound,
		"ErrAttendeeNotGroupMember":          domain.ErrAttendeeNotGroupMember,
		"ErrAttendeeAlreadyAttending":        domain.ErrAttendeeAlreadyAttending,
		"ErrAttendeeNotGoing":                domain.ErrAttendeeNotGoing,
		"ErrAttendeeCannotPromote":           domain.ErrAttendeeCannotPromote,
		"ErrInvalidAttendeeTransition":       domain.ErrInvalidAttendeeTransition,
		"ErrInvalidAttendeeStatus":           domain.ErrInvalidAttendeeStatus,
		"ErrInvalidSessionTimeframe":         domain.ErrInvalidSessionTimeframe,
		"ErrAttendeeNotAwaitingApproval":     domain.ErrAttendeeNotAwaitingApproval,
		"ErrTeamsNotSupported":               domain.ErrTeamsNotSupported,
		"ErrInvalidTeamCount":                domain.ErrInvalidTeamCount,
		"ErrInvalidMinPlayersPerTeam":        domain.ErrInvalidMinPlayersPerTeam,
		"ErrInvalidTeamColors":               domain.ErrInvalidTeamColors,
		"ErrSessionHasNoTeams":               domain.ErrSessionHasNoTeams,
		"ErrTeamNotFound":                    domain.ErrTeamNotFound,
		"ErrNotEnoughPlayers":                domain.ErrNotEnoughPlayers,
		"ErrInvalidPickOrder":                domain.ErrInvalidPickOrder,
		"ErrInvalidDraftCaptains":            domain.ErrInvalidDraftCaptains,
		"ErrDraftCaptainNotConfirmed":        domain.ErrDraftCaptainNotConfirmed,
		"ErrDraftAlreadyActive":              domain.ErrDraftAlreadyActive,
		"ErrNotProposalAuthor":               domain.ErrNotProposalAuthor,
		"ErrTeamsDrafted":                    domain.ErrTeamsDrafted,
		"ErrDraftNotActive":                  domain.ErrDraftNotActive,
		"ErrDraftNotFound":                   domain.ErrDraftNotFound,
		"ErrNotDraftCaptain":                 domain.ErrNotDraftCaptain,
		"ErrNotYourTurn":                     domain.ErrNotYourTurn,
		"ErrPlayerNotAvailable":              domain.ErrPlayerNotAvailable,
		"ErrInvalidDivision":                 domain.ErrInvalidDivision,
		"ErrVotingNotOpen":                   domain.ErrVotingNotOpen,
		"ErrVotingOpen":                      domain.ErrVotingOpen,
		"ErrProposalNotFound":                domain.ErrProposalNotFound,
		"ErrDuplicateProposal":               domain.ErrDuplicateProposal,
		"ErrRoundProposalLimit":              domain.ErrRoundProposalLimit,
		"ErrAuthorProposalLimit":             domain.ErrAuthorProposalLimit,
		"ErrKeepCurrentNotAvailable":         domain.ErrKeepCurrentNotAvailable,
		"ErrTieRequiresWinner":               domain.ErrTieRequiresWinner,
		"ErrInvalidWinner":                   domain.ErrInvalidWinner,
		"ErrRecurrenceRuleMissingInterval":   domain.ErrRecurrenceRuleMissingInterval,
		"ErrInvalidRecurrenceRule":           domain.ErrInvalidRecurrenceRule,
		"ErrRecurrenceRuleMissingTimeOfDay":  domain.ErrRecurrenceRuleMissingTimeOfDay,
		"ErrRecurrenceRuleMissingDayOfWeek":  domain.ErrRecurrenceRuleMissingDayOfWeek,
		"ErrRecurrenceRuleInvalidDayOfWeek":  domain.ErrRecurrenceRuleInvalidDayOfWeek,
		"ErrRecurrenceRuleInvalidDayOfMonth": domain.ErrRecurrenceRuleInvalidDayOfMonth,
		"ErrRecurrenceRuleMissingDayOfMonth": domain.ErrRecurrenceRuleMissingDayOfMonth,
		"ErrRecurrenceEndDateInPast":         domain.ErrRecurrenceEndDateInPast,
		"ErrInvalidTimeOfDayHour":            domain.ErrInvalidTimeOfDayHour,
		"ErrInvalidTimeOfDayMinute":          domain.ErrInvalidTimeOfDayMinute,
		"ErrInviteLinkAlreadyUsed":           domain.ErrInviteLinkAlreadyUsed,
		"ErrInviteLinkHasNotExpiredYet":      domain.ErrInviteLinkHasNotExpiredYet,
		"ErrInviteLinkExpired":               domain.ErrInviteLinkExpired,
		"ErrInviteLinkRevoked":               domain.ErrInviteLinkRevoked,
		"ErrSessionTemplateTitleRequired":    domain.ErrSessionTemplateTitleRequired,
		"ErrSessionTemplateLocationRequired": domain.ErrSessionTemplateLocationRequired,
		"ErrInvalidRecurrenceFrequency":      domain.ErrInvalidRecurrenceFrequency,
		"ErrInvalidDefaultCapacity":          domain.ErrInvalidDefaultCapacity,
		"ErrSessionTemplateNotFound":         domain.ErrSessionTemplateNotFound,
		"ErrSessionTemplateNotActive":        domain.ErrSessionTemplateNotActive,
		"ErrSessionTemplateDeleted":          domain.ErrSessionTemplateDeleted,
		"ErrSessionTemplateNotRecurring":     domain.ErrSessionTemplateNotRecurring,
		"ErrInvalidReportTarget":             domain.ErrInvalidReportTarget,
		"ErrInvalidReportReason":             domain.ErrInvalidReportReason,
		"ErrReportNoteTooLong":               domain.ErrReportNoteTooLong,
		"ErrCannotReportYourself":            domain.ErrCannotReportYourself,
		"ErrCannotBlockYourself":             domain.ErrCannotBlockYourself,
		"ErrAccountBlocked":                  domain.ErrAccountBlocked,
	}

	file, err := parser.ParseFile(token.NewFileSet(), "../domain/errors.go", nil, 0)
	require.NoError(t, err)
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			for _, name := range spec.(*ast.ValueSpec).Names {
				_, listed := sentinels[name.Name]
				assert.True(t, listed, "add domain.%s to this test", name.Name)
			}
		}
	}

	for name, sentinel := range sentinels {
		appErr := MapErrToAppError(fmt.Errorf("wrapped: %w", sentinel))
		require.NotNil(t, appErr, name)
		assert.NotEqual(t, http.StatusInternalServerError, appErr.StatusCode, "%s has no case in MapErrToAppError", name)
	}
}
