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
	RecurrenceFrequency  *string        // "daily", "weekly", "monthly"
	RecurrenceInterval   *int           // Every N frequencies
	RecurrenceDayOfWeek  *time.Weekday  // For weekly: 0=Sun, 1=Mon, etc.
	RecurrenceDayOfMonth *int           // For monthly: 1-31
	RecurrenceTimeHour   int            // 0-23
	RecurrenceTimeMinute int            // 0-59
	RecurrenceEndsAt     *time.Time     // When recurrence ends (optional)

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
	RecurrenceFrequency  *string        // "daily", "weekly", "monthly"
	RecurrenceInterval   *int           // Every N frequencies
	RecurrenceDayOfWeek  *time.Weekday  // For weekly: 0=Sun, 1=Mon, etc.
	RecurrenceDayOfMonth *int           // For monthly: 1-31
	RecurrenceTimeHour   int            // 0-23
	RecurrenceTimeMinute int            // 0-59
	RecurrenceEndsAt     *time.Time     // When recurrence ends (optional)

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
