package domain

import "errors"

// Activity domain errors.
var (
	ErrActivityGroupNotFound          = errors.New("activity group not found")
	ErrActivityGroupNotDraft          = errors.New("activity group is not in draft status")
	ErrActivityGroupCancelled         = errors.New("activity group is cancelled")
	ErrActivityGroupNotPublished      = errors.New("activity group is not in published status")
	ErrActivityGroupNotStarted        = errors.New("activity group is not started yet")
	ErrActivityGroupAlreadyStarted    = errors.New("activity group is already started")
	ErrActivityGroupCanceled          = errors.New("activity group is canceled")
	ErrActivityGroupDeleted           = errors.New("activity group is deleted")
	ErrActivityGroupNotActive         = errors.New("activity group is not active")
	ErrActivityGroupCompleted         = errors.New("activity group is completed")
	ErrActivityGroupFull              = errors.New("activity group is full")
	ErrActivityGroupNotJoinable       = errors.New("activity group is not open for join requests")
	ErrInvalidScheduleTime            = errors.New("start time must be before end time")
	ErrActivityGroupInvalidVisibility = errors.New("activity group visibility is required")
	ErrStartTimeInPast                = errors.New("start time cannot be in the past")
	ErrUnauthorized                   = errors.New("unauthorized to perform this action")
)

// Member errors
var (
	ErrMemberNotFound        = errors.New("member not found")
	ErrMemberAlreadyAdmin    = errors.New("member is already an admin")
	ErrMemberNotAdmin        = errors.New("member is not an admin")
	ErrAlreadyParticipating  = errors.New("user is already participating in this activity")
	ErrCreatorCannotLeave    = errors.New("creator cannot leave their own activity")
	ErrNotConfirmed          = errors.New("member is not confirmed")
	ErrNotPendingStatus      = errors.New("member status is not pending")
	ErrCannotRemoveCreator   = errors.New("cannot remove the creator from the activity")
	ErrCannotDemoteCreator   = errors.New("cannot demote the creator to a regular member")
	ErrMemberAlreadyPriority = errors.New("member is already a priority member")
	ErrMemberNotPriority     = errors.New("member is not a priority member")
	ErrInsufficientRole      = errors.New("member role cannot manage this group")
)

// Invite errors
var (
	ErrInviteNotFound         = errors.New("invite not found")
	ErrInviteExpired          = errors.New("invite has expired")
	ErrInviteHasNotExpiredYet = errors.New("invite has not expired yet")
	ErrInviteAlreadyProcessed = errors.New("invite has already been processed")
	ErrAlreadyInvited         = errors.New("user has already been invited to this activity")
	ErrCannotInviteCreator    = errors.New("cannot invite the creator of the activity")
	ErrUserAlreadyMember      = errors.New("user is already a member of this activity group")
)

// Session errors
var (
	ErrSessionNotFound                 = errors.New("session not found")
	ErrSessionNotScheduled             = errors.New("session is not scheduled")
	ErrSessionNotStarted               = errors.New("session has not started yet")
	ErrSessionCanceled                 = errors.New("session is canceled")
	ErrSessionCompleted                = errors.New("session is completed")
	ErrSessionNotExpiredYet            = errors.New("session has not expired yet")
	ErrSessionAlreadyStarted           = errors.New("session has already started")
	ErrSessionInvalidSchedule          = errors.New("session schedule is invalid")
	ErrInvalidSessionCapacity          = errors.New("session capacity must be a positive integer or nil for unlimited")
	ErrSessionStartTimeInPast          = errors.New("session start time cannot be in the past")
	ErrSessionLocationCityRequired     = errors.New("session location city is required")
	ErrSessionLocationCountryRequired  = errors.New("session location country is required")
	ErrSessionLocationLatitudeInvalid  = errors.New("session location latitude must be between -90 and 90")
	ErrSessionLocationLongitudeInvalid = errors.New("session location longitude must be between -180 and 180")
	ErrInvalidSessionVisibility        = errors.New("session visibility must be public or private")
	ErrSessionFull                     = errors.New("session has reached its capacity")
	ErrInvalidDiscoveryRadius          = errors.New("discovery radius must be greater than 0")
	ErrSessionNotOpen                  = errors.New("session is not open for regular members yet")
	ErrSessionNotStandalone            = errors.New("invites are only supported for standalone sessions")
)

// Attendee errors
var (
	ErrAttendeeNotFound            = errors.New("attendee not found")
	ErrAttendeeNotGroupMember      = errors.New("attendee is not a member of the activity group")
	ErrAttendeeAlreadyAttending    = errors.New("attendee is already attending this session")
	ErrAttendeeNotGoing            = errors.New("attendee is not marked as going to this session")
	ErrAttendeeCannotPromote       = errors.New("no pending attendees to promote to confirmed status")
	ErrInvalidAttendeeTransition   = errors.New("invalid attendee status transition")
	ErrInvalidAttendeeStatus       = errors.New("invalid attendee status")
	ErrAttendeeNotAwaitingApproval = errors.New("attendee is not awaiting approval")
)

// Team errors
var (
	ErrTeamsNotSupported        = errors.New("teams are only supported for team sports")
	ErrInvalidTeamCount         = errors.New("team count must be between 2 and 8")
	ErrInvalidMinPlayersPerTeam = errors.New("min players per team must be a positive integer or nil for none")
	ErrInvalidTeamColors        = errors.New("team colors must be #RRGGBB hex values, one per team")
	ErrSessionHasNoTeams        = errors.New("session is not split into teams")
	ErrTeamNotFound             = errors.New("team not found")
	ErrNotEnoughPlayers         = errors.New("not enough people are going for these teams")
)

// Team draft errors
var (
	ErrInvalidPickOrder         = errors.New("pick order must be snake or alternate")
	ErrInvalidDraftCaptains     = errors.New("a draft needs two different captains, each from their own team or the pool")
	ErrDraftCaptainNotConfirmed = errors.New("draft captains must be going")
	ErrDraftAlreadyActive       = errors.New("a team draft is running for this session")
	ErrDraftNotActive           = errors.New("no team draft is running for this session")
	ErrDraftPaused              = errors.New("the team draft is paused until a captain is replaced")
	ErrDraftNotPaused           = errors.New("the team draft is not paused")
	ErrDraftNotFound            = errors.New("team draft not found")
	ErrNotDraftCaptain          = errors.New("only a draft captain can pick players")
	ErrNotYourTurn              = errors.New("it is not your turn to pick")
	ErrPlayerNotAvailable       = errors.New("player is not available to pick")
)

// Recurrence errors
var (
	ErrRecurrenceRuleMissingInterval   = errors.New("interval must be specified for recurring activity")
	ErrInvalidRecurrenceRule           = errors.New("invalid recurrence rule")
	ErrNoRecurrenceRule                = errors.New("no recurrence rule provided for recurring activity")
	ErrRecurrenceRuleMissingTimeOfDay  = errors.New("time of day must be specified for recurring activity")
	ErrRecurrenceRuleInvalidDayOfWeek  = errors.New("day of week must be between 0 (Sunday) and 6 (Saturday)")
	ErrRecurrenceRuleInvalidDayOfMonth = errors.New("day of month must be between 1 and 31")
	ErrRecurrenceRuleMissingDayOfMonth = errors.New("day of month must be specified for monthly recurrence")
	ErrRecurrenceEndDateInPast         = errors.New("recurrence end date cannot be in the past")
)

var (
	ErrInvalidTimeOfDayHour   = errors.New("invalid time of day hour, must be between 0 and 23")
	ErrInvalidTimeOfDayMinute = errors.New("invalid time of day minute, must be between 0 and 59")
)

var (
	ErrInviteLinkAlreadyUsed      = errors.New("invite link has already been used")
	ErrInviteLinkHasNotExpiredYet = errors.New("invite link has not expired yet")
	ErrInviteLinkExpired          = errors.New("invite link has expired")
	ErrInviteLinkRevoked          = errors.New("invite link has been revoked")
)

// SessionTemplate errors
var (
	ErrSessionTemplateTitleRequired         = errors.New("session template title is required")
	ErrSessionTemplateLocationRequired      = errors.New("session template location is required")
	ErrInvalidRecurrenceFrequency           = errors.New("invalid recurrence frequency")
	ErrInvalidGenerateHoursBefore           = errors.New("generate hours before cut-off time must be non-negative")
	ErrInvalidDefaultCapacity               = errors.New("default capacity must be greater than 0")
	ErrSessionTemplateNotFound              = errors.New("session template not found")
	ErrSessionTemplateNotActive             = errors.New("session template is not active")
	ErrSessionTemplateInvalidCutOffTime     = errors.New("invalid cut-off time for session template")
	ErrSessionTemplateInvalidRecurrenceRule = errors.New("invalid recurrence rule for session template")
	ErrSessionTemplateDeleted               = errors.New("session template is deleted")
	ErrSessionTemplateNotRecurring          = errors.New("session template is not recurring")
)
