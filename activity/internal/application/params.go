package application

import (
	"time"

	"github.com/google/uuid"
)

// CreateSessionTemplateParams contains parameters for creating a new session template
type CreateSessionTemplateParams struct {
	ActivityGroupID uuid.UUID
	CreatedByID     uuid.UUID
	Title           string
	Description     string

	// Recurrence configuration (all nil if not recurring)
	RecurrenceFrequency  *string       // "daily", "weekly", "monthly"
	RecurrenceInterval   *int          // Every N frequencies
	RecurrenceDayOfWeek  *time.Weekday // For weekly: 0=Sun, 1=Mon, etc.
	RecurrenceDayOfMonth *int          // For monthly: 1-31
	RecurrenceTimeHour   int           // 0-23
	RecurrenceTimeMinute int           // 0-59
	RecurrenceEndsAt     *time.Time    // When recurrence ends (optional)

	// Default session settings, inherited by every session generated from this template
	DefaultCapacity *int // nil = unlimited
	LocationCity    string
	LocationCountry string
	LocationStreet  string // optional
	LocationLat     float64
	LocationLng     float64
}

// UpdateSessionTemplateParams contains parameters for updating a session template
type UpdateSessionTemplateParams struct {
	RequesterID uuid.UUID
	Title       string
	Description string

	// Recurrence configuration (all nil if not recurring)
	RecurrenceFrequency  *string       // "daily", "weekly", "monthly"
	RecurrenceInterval   *int          // Every N frequencies
	RecurrenceDayOfWeek  *time.Weekday // For weekly: 0=Sun, 1=Mon, etc.
	RecurrenceDayOfMonth *int          // For monthly: 1-31
	RecurrenceTimeHour   int           // 0-23
	RecurrenceTimeMinute int           // 0-59
	RecurrenceEndsAt     *time.Time    // When recurrence ends (optional)

	// Default session settings, inherited by every session generated from this template
	DefaultCapacity *int // nil = unlimited
	LocationCity    string
	LocationCountry string
	LocationStreet  string // optional
	LocationLat     float64
	LocationLng     float64
}

// ListSessionTemplatesParams contains parameters for listing session templates
type ListSessionTemplatesParams struct {
	ActivityGroupID *uuid.UUID
	CreatedByID     *uuid.UUID
	Status          *string // "active", "inactive"
	IsRecurring     *bool

	// For cron job
	NeedsGeneration *bool
	LookaheadWindow *time.Duration

	// Pagination
	Limit     int
	PageToken string
}

// CreateActivityGroupParams contains parameters for creating a new activity group
type CreateActivityGroupParams struct {
	CreatorID       uuid.UUID
	Title           string
	Description     string
	ActivityType    string
	DifficultyLevel string
	Visibility      string
	LocationCity    string
	LocationCountry string
	Timezone        string
	DefaultCapacity *int // nil = unlimited
}

// UpdateActivityGroupParams contains parameters for updating an activity group
type UpdateActivityGroupParams struct {
	RequesterID     uuid.UUID
	Title           string
	Description     string
	ActivityType    string
	DifficultyLevel string
	LocationCity    string
	LocationCountry string
	Timezone        string
}

// ListActivityGroupsParams contains parameters for listing activity groups
type ListActivityGroupsParams struct {
	CreatorID *uuid.UUID
	// MemberID filters to groups this account has a membership row in -
	// "groups I've joined" (as opposed to CreatorID's "groups I created").
	// A caller passing someone else's account here only sees that account's
	// public group memberships, per the same visibility rule RequesterID
	// applies everywhere else.
	MemberID *uuid.UUID
	// MemberStatus restricts MemberID to these membership statuses (e.g.
	// "pending" for outstanding join requests); empty defaults to
	// "confirmed" only. Non-"confirmed" values are only honored when
	// MemberID is the requester's own account - your pending/rejected/left
	// history with a group is never exposed on someone else's profile view.
	MemberStatus    []string
	Status          *string // "draft", "active", "cancelled"
	Title           *string
	ActivityType    *string
	DifficultyLevel *string // "beginner", "intermediate", "advanced"
	RequesterID     uuid.UUID
	Limit           int
	PageToken       string
}

// ActivityInterestFilter is one (activity_type, difficulty_level) pair to
// OR-match in a discover query - DifficultyLevel empty means "any level for
// this activity type". Lets a caller with several interests (e.g. running
// at any level, cycling at advanced only) get everything matching any of
// them in a single request instead of one request per interest.
type ActivityInterestFilter struct {
	ActivityType    string
	DifficultyLevel string
}

// DiscoverActivityGroupsParams contains parameters for discovering public activity groups
type DiscoverActivityGroupsParams struct {
	City      *string
	Country   *string
	Interests []ActivityInterestFilter
	Limit     int
	PageToken string
}

// CreateSessionParams contains parameters for creating a new session
type CreateSessionParams struct {
	ActivityGroupID *uuid.UUID // nil for a standalone session with no group
	CreatedByID     uuid.UUID
	Title           string
	// ActivityType and DifficultyLevel are only used when ActivityGroupID is nil
	// (standalone session). For a grouped session, both are always inherited
	// from the group.
	ActivityType    string
	DifficultyLevel string
	LocationCity    string
	LocationCountry string
	LocationStreet  string // optional
	LocationLat     float64
	LocationLng     float64
	StartTime       time.Time
	EndTime         *time.Time
	Capacity        *int // nil = unlimited
	Note            string
	IsRecurring     bool
	// Visibility is required and applies to both group-scoped and standalone
	// sessions.
	Visibility       string
	RequiresApproval bool
}

// UpdateSessionParams contains parameters for updating a session
type UpdateSessionParams struct {
	RequesterID     uuid.UUID
	RequesterRole   string // "admin", "creator", "member"
	LocationCity    string
	LocationCountry string
	LocationStreet  string // optional
	LocationLat     float64
	LocationLng     float64
	StartTime       time.Time
	EndTime         *time.Time
	Capacity        *int
	Note            string
	// Visibility is required and applies to both group-scoped and standalone
	// sessions.
	Visibility string
}

// ListSessionsParams contains parameters for listing sessions
type ListSessionsParams struct {
	ActivityGroupID   *uuid.UUID
	SessionTemplateID *uuid.UUID
	CreatedByID       *uuid.UUID
	// AttendeeID filters to sessions this account has RSVP'd to - "sessions
	// I've joined" (as opposed to CreatedByID's "sessions I created"). A
	// caller passing someone else's account here only sees that account's
	// public sessions, mirroring ListActivityGroupsParams.MemberID.
	AttendeeID *uuid.UUID
	// AttendeeStatus restricts AttendeeID's RSVP statuses (e.g. "going",
	// "promoted"); empty defaults to whichever statuses currently hold a
	// spot. Ignored when AttendeeID is nil.
	AttendeeStatus  []string
	Status          *string // "scheduled", "started", "canceled", "completed"
	ActivityType    *string
	DifficultyLevel *string
	IsRecurring     *bool
	StartTimeFrom   *time.Time
	StartTimeTo     *time.Time
	Limit           int
	PageToken       string
}

// DiscoverSessionsParams contains parameters for finding public sessions near a location
type DiscoverSessionsParams struct {
	Lat       float64
	Lng       float64
	RadiusKM  float64
	Interests []ActivityInterestFilter
	Limit     int
	PageToken string
}

// InviteMemberParams contains parameters for inviting a member to a group
type InviteMemberParams struct {
	ActivityGroupID uuid.UUID
	RequesterID     uuid.UUID
	RequesterRole   string // "creator", "admin"
	UserID          uuid.UUID
}

// SendInviteParams contains parameters for sending a group invite
type SendInviteParams struct {
	ActivityGroupID uuid.UUID
	RequesterID     uuid.UUID
	RequesterRole   string // "creator", "admin"
	InvitedUserID   uuid.UUID
}

// SendSessionInviteParams contains parameters for inviting a user to a standalone session
type SendSessionInviteParams struct {
	SessionID     uuid.UUID
	RequesterID   uuid.UUID
	InvitedUserID uuid.UUID
}

// RemoveMemberParams contains parameters for removing a member from a group
type RemoveMemberParams struct {
	ActivityGroupID uuid.UUID
	RequesterID     uuid.UUID
	RequesterRole   string // "creator", "admin"
	UserID          uuid.UUID
}

// UpdateMemberRoleParams contains parameters for updating a member's role
type UpdateMemberRoleParams struct {
	ActivityGroupID uuid.UUID
	RequesterID     uuid.UUID
	RequesterRole   string // must be "creator"
	UserID          uuid.UUID
	NewRole         string // "admin" or "member"
}

// RequestToJoinGroupParams contains parameters for a user self-requesting to join a public group
type RequestToJoinGroupParams struct {
	ActivityGroupID uuid.UUID
	UserID          uuid.UUID
}

// ApproveMemberParams contains parameters for approving a join request
type ApproveMemberParams struct {
	ActivityGroupID uuid.UUID
	RequesterID     uuid.UUID
	RequesterRole   string // "creator", "admin"
	UserID          uuid.UUID
}

// RejectMemberParams contains parameters for rejecting a join request
type RejectMemberParams struct {
	ActivityGroupID uuid.UUID
	RequesterID     uuid.UUID
	RequesterRole   string // "creator", "admin"
	UserID          uuid.UUID
}

// ListMembersParams contains parameters for listing members
type ListMembersParams struct {
	ActivityGroupID *uuid.UUID
	UserID          *uuid.UUID
	Status          *string // "pending", "confirmed", "rejected", "left", "removed"
	Role            *string // "creator", "admin", "member"
	Limit           int
	PageToken       string
}

// CreateRSVPParams contains parameters for creating an RSVP
type CreateRSVPParams struct {
	SessionID uuid.UUID
	UserID    uuid.UUID
	Status    string // "going", "not_going", "maybe"
}

// ApproveAttendeeParams contains parameters for approving a pending join request
type ApproveAttendeeParams struct {
	SessionID     uuid.UUID
	UserID        uuid.UUID
	RequesterID   uuid.UUID
	RequesterRole string // "creator", "admin", or "" for a standalone session's creator
}

// RejectAttendeeParams contains parameters for rejecting a pending join request
type RejectAttendeeParams struct {
	SessionID     uuid.UUID
	UserID        uuid.UUID
	RequesterID   uuid.UUID
	RequesterRole string
}

// RemoveAttendeeParams contains parameters for removing an already-confirmed attendee from a session
type RemoveAttendeeParams struct {
	SessionID     uuid.UUID
	UserID        uuid.UUID
	RequesterID   uuid.UUID
	RequesterRole string
}

// UpdateRSVPParams contains parameters for updating an RSVP
type UpdateRSVPParams struct {
	SessionID uuid.UUID
	UserID    uuid.UUID
	NewStatus string // "going", "not_going", "maybe"
}

// ListRSVPsParams contains parameters for listing RSVPs
type ListRSVPsParams struct {
	SessionID       *uuid.UUID
	ActivityGroupID *uuid.UUID
	UserID          *uuid.UUID
	Status          *string // "going", "pending", "not_going", "maybe", "promoted"
	Limit           int
	PageToken       string
}

// ListMyInvitesParams contains parameters for listing a user's pending invites
type ListMyInvitesParams struct {
	Limit     int
	PageToken string
}
