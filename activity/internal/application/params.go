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

	// Default session settings
	DefaultCapacity *int    // nil = unlimited
	LocationCity    *string // Optional for templates
	LocationCountry *string // Optional for templates
}

// UpdateSessionTemplateParams contains parameters for updating a session template
type UpdateSessionTemplateParams struct {
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

	// Default session settings
	DefaultCapacity *int    // nil = unlimited
	LocationCity    *string // Optional for templates
	LocationCountry *string // Optional for templates
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
	Limit  int
	Offset int
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
	Status    *string // "draft", "active", "cancelled"
	Title     *string
	Limit     int
	Offset    int
}

// DiscoverActivityGroupsParams contains parameters for discovering public activity groups
type DiscoverActivityGroupsParams struct {
	City         *string
	Country      *string
	ActivityType *string
	Limit        int
	Offset       int
}

// CreateSessionParams contains parameters for creating a new session
type CreateSessionParams struct {
	ActivityGroupID uuid.UUID
	CreatedByID     uuid.UUID
	LocationCity    string
	LocationCountry string
	LocationLat     float64
	LocationLng     float64
	StartTime       time.Time
	EndTime         *time.Time
	Capacity        *int // nil = unlimited
	Note            string
	IsRecurring     bool
}

// UpdateSessionParams contains parameters for updating a session
type UpdateSessionParams struct {
	RequesterID     uuid.UUID
	RequesterRole   string // "admin", "creator", "member"
	LocationCity    string
	LocationCountry string
	LocationLat     float64
	LocationLng     float64
	StartTime       time.Time
	EndTime         *time.Time
	Capacity        *int
	Note            string
}

// ListSessionsParams contains parameters for listing sessions
type ListSessionsParams struct {
	ActivityGroupID   *uuid.UUID
	SessionTemplateID *uuid.UUID
	Status            *string // "scheduled", "started", "canceled", "completed"
	IsRecurring       *bool
	StartTimeFrom     *time.Time
	StartTimeTo       *time.Time
	Limit             int
	Offset            int
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
	Offset          int
}

// CreateRSVPParams contains parameters for creating an RSVP
type CreateRSVPParams struct {
	SessionID       uuid.UUID
	ActivityGroupID uuid.UUID
	UserID          uuid.UUID
	Status          string // "going", "not_going", "maybe"
}

// UpdateRSVPParams contains parameters for updating an RSVP
type UpdateRSVPParams struct {
	SessionID       uuid.UUID
	ActivityGroupID uuid.UUID
	UserID          uuid.UUID
	NewStatus       string // "going", "not_going", "maybe"
}

// ListRSVPsParams contains parameters for listing RSVPs
type ListRSVPsParams struct {
	SessionID       *uuid.UUID
	ActivityGroupID *uuid.UUID
	UserID          *uuid.UUID
	Status          *string // "going", "pending", "not_going", "maybe", "promoted"
	Limit           int
	Offset          int
}
