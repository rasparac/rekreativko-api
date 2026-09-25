package application

import (
	"errors"

	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/shared/domainerror"

	"github.com/jackc/pgx/v5"
)

// MapErrToAppError maps domain errors to application errors
func MapErrToAppError(err error) *domainerror.AppError {
	if err == nil {
		return nil
	}

	// Session Template errors
	switch {
	case errors.Is(err, domain.ErrSessionTemplateNotFound):
		return domainerror.NotFound("session_template_not_found", "Session template not found", err)
	case errors.Is(err, domain.ErrSessionTemplateNotActive):
		return domainerror.ValidationError("session_template_not_active", "Session template is not active", err)
	case errors.Is(err, domain.ErrSessionTemplateDeleted):
		return domainerror.NotFound("session_template_deleted", "Session template has been deleted", err)
	case errors.Is(err, domain.ErrSessionTemplateTitleRequired):
		return domainerror.ValidationError("title_required", "Session template title is required", err)
	case errors.Is(err, domain.ErrSessionTemplateLocationRequired):
		return domainerror.ValidationError("location_required", "Session template location is required", err)
	case errors.Is(err, domain.ErrInvalidDefaultCapacity):
		return domainerror.ValidationError("invalid_capacity", "Default capacity must be greater than 0", err)
	case errors.Is(err, domain.ErrInvalidRecurrenceFrequency):
		return domainerror.ValidationError("invalid_recurrence_frequency", "Invalid recurrence frequency", err)
	case errors.Is(err, domain.ErrSessionTemplateInvalidRecurrenceRule):
		return domainerror.ValidationError("invalid_recurrence_rule", "Invalid recurrence rule for session template", err)
	}

	// Recurrence errors
	switch {
	case errors.Is(err, domain.ErrInvalidRecurrenceRule):
		return domainerror.ValidationError("invalid_recurrence_rule", "Invalid recurrence rule", err)
	case errors.Is(err, domain.ErrRecurrenceRuleMissingInterval):
		return domainerror.ValidationError("missing_interval", "Interval must be specified for recurring activity", err)
	case errors.Is(err, domain.ErrRecurrenceRuleMissingTimeOfDay):
		return domainerror.ValidationError("missing_time_of_day", "Time of day must be specified for recurring activity", err)
	case errors.Is(err, domain.ErrRecurrenceRuleInvalidDayOfWeek):
		return domainerror.ValidationError("invalid_day_of_week", "Day of week must be between 0 (Sunday) and 6 (Saturday)", err)
	case errors.Is(err, domain.ErrRecurrenceRuleInvalidDayOfMonth):
		return domainerror.ValidationError("invalid_day_of_month", "Day of month must be between 1 and 31", err)
	case errors.Is(err, domain.ErrRecurrenceRuleMissingDayOfMonth):
		return domainerror.ValidationError("missing_day_of_month", "Day of month must be specified for monthly recurrence", err)
	case errors.Is(err, domain.ErrRecurrenceEndDateInPast):
		return domainerror.ValidationError("end_date_in_past", "Recurrence end date cannot be in the past", err)
	case errors.Is(err, domain.ErrInvalidTimeOfDayHour):
		return domainerror.ValidationError("invalid_hour", "Invalid time of day hour, must be between 0 and 23", err)
	case errors.Is(err, domain.ErrInvalidTimeOfDayMinute):
		return domainerror.ValidationError("invalid_minute", "Invalid time of day minute, must be between 0 and 59", err)
	case errors.Is(err, domain.ErrSessionLocationLatitudeInvalid):
		return domainerror.ValidationError("invalid_latitude", "Latitude must be between -90 and 90", err)
	case errors.Is(err, domain.ErrSessionLocationLongitudeInvalid):
		return domainerror.ValidationError("invalid_longitude", "Longitude must be between -180 and 180", err)
	case errors.Is(err, domain.ErrInvalidDiscoveryRadius):
		return domainerror.ValidationError("invalid_radius", "Discovery radius must be greater than 0", err)
	}

	// Session errors
	switch {
	case errors.Is(err, domain.ErrSessionNotFound):
		return domainerror.NotFound("session_not_found", "Session not found", err)
	case errors.Is(err, domain.ErrSessionCanceled):
		return domainerror.Conflict("session_canceled", "Session is canceled", err)
	case errors.Is(err, domain.ErrSessionCompleted):
		return domainerror.Conflict("session_completed", "Session is completed", err)
	case errors.Is(err, domain.ErrSessionAlreadyStarted):
		return domainerror.Conflict("session_already_started", "Session has already started", err)
	case errors.Is(err, domain.ErrSessionNotStarted):
		return domainerror.Conflict("session_not_started", "Session has not started yet", err)
	case errors.Is(err, domain.ErrSessionNotScheduled):
		return domainerror.Conflict("session_not_scheduled", "Session is not scheduled", err)
	case errors.Is(err, domain.ErrSessionNotOpen):
		return domainerror.Forbidden("session_not_open", "Session is not open for regular members yet", err)
	case errors.Is(err, domain.ErrSessionInvalidSchedule):
		return domainerror.ValidationError("invalid_schedule", "Session schedule is invalid", err)
	case errors.Is(err, domain.ErrSessionStartTimeInPast):
		return domainerror.ValidationError("start_time_in_past", "Session start time cannot be in the past", err)
	case errors.Is(err, domain.ErrInvalidSessionCapacity):
		return domainerror.ValidationError("invalid_capacity", "Session capacity must be a positive integer or nil for unlimited", err)
	case errors.Is(err, domain.ErrSessionLocationCityRequired):
		return domainerror.ValidationError("city_required", "Session location city is required", err)
	case errors.Is(err, domain.ErrSessionLocationCountryRequired):
		return domainerror.ValidationError("country_required", "Session location country is required", err)
	case errors.Is(err, domain.ErrInvalidSessionVisibility):
		return domainerror.ValidationError("invalid_visibility", "Session visibility must be public or private", err)
	case errors.Is(err, domain.ErrSessionFull):
		return domainerror.Conflict("session_full", "Session has reached its capacity", err)
	}

	// Team errors
	switch {
	case errors.Is(err, domain.ErrTeamsNotSupported):
		return domainerror.ValidationError("teams_not_supported", "Teams are only supported for team sports (basketball, football, volleyball)", err)
	case errors.Is(err, domain.ErrInvalidTeamCount):
		return domainerror.ValidationError("invalid_team_count", "Team count must be between 2 and 8", err)
	case errors.Is(err, domain.ErrInvalidPlayersPerTeam):
		return domainerror.ValidationError("invalid_players_per_team", "Players per team must be a positive integer", err)
	case errors.Is(err, domain.ErrInvalidTeamColors):
		return domainerror.ValidationError("invalid_team_colors", "Team colors must be #RRGGBB hex values, one per team", err)
	case errors.Is(err, domain.ErrSessionHasNoTeams):
		return domainerror.Conflict("session_has_no_teams", "Session is not split into teams", err)
	case errors.Is(err, domain.ErrTeamNotFound):
		return domainerror.NotFound("team_not_found", "Team not found", err)
	case errors.Is(err, domain.ErrTeamFull):
		return domainerror.Conflict("team_full", "Team has reached its player limit", err)
	}

	// Attendee errors
	switch {
	case errors.Is(err, domain.ErrAttendeeNotFound):
		return domainerror.NotFound("attendee_not_found", "Attendee not found", err)
	case errors.Is(err, domain.ErrAttendeeNotGroupMember):
		return domainerror.Forbidden("attendee_not_group_member", "You must be a member of this activity group", err)
	case errors.Is(err, domain.ErrAttendeeAlreadyAttending):
		return domainerror.Conflict("attendee_already_attending", "Already attending this session", err)
	case errors.Is(err, domain.ErrAttendeeNotGoing):
		return domainerror.Conflict("attendee_not_going", "Attendee is not marked as going to this session", err)
	case errors.Is(err, domain.ErrAttendeeCannotPromote):
		return domainerror.Conflict("no_pending_attendees", "No pending attendees to promote to confirmed status", err)
	case errors.Is(err, domain.ErrInvalidAttendeeTransition):
		return domainerror.ValidationError("invalid_attendee_transition", "Invalid attendee status transition", err)
	case errors.Is(err, domain.ErrInvalidAttendeeStatus):
		return domainerror.ValidationError("invalid_attendee_status", "Invalid attendee status", err)
	case errors.Is(err, domain.ErrAttendeeNotAwaitingApproval):
		return domainerror.Conflict("attendee_not_awaiting_approval", "Attendee is not awaiting approval", err)
	case errors.Is(err, domain.ErrCannotRemoveCreator):
		return domainerror.Conflict("cannot_remove_creator", "Cannot remove the creator from the activity", err)
	}

	// Member errors
	switch {
	case errors.Is(err, domain.ErrMemberNotFound):
		return domainerror.NotFound("member_not_found", "Member not found", err)
	case errors.Is(err, domain.ErrMemberAlreadyAdmin):
		return domainerror.Conflict("member_already_admin", "Member is already an admin", err)
	case errors.Is(err, domain.ErrMemberNotAdmin):
		return domainerror.Conflict("member_not_admin", "Member is not an admin", err)
	case errors.Is(err, domain.ErrCreatorCannotLeave):
		return domainerror.Conflict("creator_cannot_leave", "Creator cannot leave their own activity", err)
	case errors.Is(err, domain.ErrNotConfirmed):
		return domainerror.Conflict("member_not_confirmed", "Member is not confirmed", err)
	case errors.Is(err, domain.ErrNotPendingStatus):
		return domainerror.Conflict("member_not_pending", "Member status is not pending", err)
	case errors.Is(err, domain.ErrCannotDemoteCreator):
		return domainerror.Conflict("cannot_demote_creator", "Cannot demote the creator to a regular member", err)
	case errors.Is(err, domain.ErrMemberAlreadyPriority):
		return domainerror.Conflict("member_already_priority", "Member is already a priority member", err)
	case errors.Is(err, domain.ErrMemberNotPriority):
		return domainerror.Conflict("member_not_priority", "Member is not a priority member", err)
	case errors.Is(err, domain.ErrInsufficientRole):
		return domainerror.Forbidden("insufficient_role", "Your role cannot manage this group", err)
	}

	// Database errors
	if errors.Is(err, pgx.ErrNoRows) {
		return domainerror.NotFound("not_found", "Resource not found", err)
	}

	// Activity Group errors (in case they're relevant)
	switch {
	case errors.Is(err, domain.ErrActivityGroupNotFound):
		return domainerror.NotFound("activity_group_not_found", "Activity group not found", err)
	case errors.Is(err, domain.ErrActivityGroupDeleted):
		return domainerror.NotFound("activity_group_deleted", "Activity group has been deleted", err)
	case errors.Is(err, domain.ErrActivityGroupNotActive):
		return domainerror.Conflict("activity_group_not_active", "Activity group is not active", err)
	case errors.Is(err, domain.ErrActivityGroupNotJoinable):
		return domainerror.Forbidden("activity_group_not_joinable", "This activity group is not open for join requests", err)
	case errors.Is(err, domain.ErrActivityGroupFull):
		return domainerror.Conflict("activity_group_full", "Activity group has reached its member capacity", err)
	case errors.Is(err, domain.ErrAlreadyParticipating):
		return domainerror.Conflict("already_participating", "You already have a pending request or membership in this activity group", err)
	case errors.Is(err, domain.ErrUnauthorized):
		return domainerror.Unauthorized("unauthorized", "Unauthorized to perform this action", err)
	}

	// Invite errors
	switch {
	case errors.Is(err, domain.ErrInviteNotFound):
		return domainerror.NotFound("invite_not_found", "Invite not found", err)
	case errors.Is(err, domain.ErrUserAlreadyMember):
		return domainerror.Conflict("user_already_member", "User is already a member of this activity group", err)
	case errors.Is(err, domain.ErrAlreadyInvited):
		return domainerror.Conflict("already_invited", "User has already been invited to this activity group", err)
	case errors.Is(err, domain.ErrInviteAlreadyProcessed):
		return domainerror.Conflict("invite_already_processed", "Invite has already been accepted or declined", err)
	case errors.Is(err, domain.ErrInviteExpired):
		return domainerror.Conflict("invite_expired", "Invite has expired", err)
	case errors.Is(err, domain.ErrCannotInviteCreator):
		return domainerror.ValidationError("cannot_invite_self", "Cannot invite yourself", err)
	case errors.Is(err, domain.ErrSessionNotStandalone):
		return domainerror.ValidationError("session_not_standalone", "Invites are only supported for standalone sessions; group sessions are joined through the group", err)
	}

	// Default to internal error with wrapped error for debugging
	return domainerror.InternalWithErr(err)
}
