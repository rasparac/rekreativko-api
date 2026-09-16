package domain

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/shared/activitycatalog"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
)

type ActivityType = activitycatalog.ActivityType

const (
	ActivityTypeHiking        = activitycatalog.ActivityTypeHiking
	ActivityTypeCycling       = activitycatalog.ActivityTypeCycling
	ActivityTypeRunning       = activitycatalog.ActivityTypeRunning
	ActivityTypeSwimming      = activitycatalog.ActivityTypeSwimming
	ActivityTypeYoga          = activitycatalog.ActivityTypeYoga
	ActivityTypeGym           = activitycatalog.ActivityTypeGym
	ActivityTypeOther         = activitycatalog.ActivityTypeOther
	ActivityTypeWalking       = activitycatalog.ActivityTypeWalking
	ActivityTypeJogging       = activitycatalog.ActivityTypeJogging
	ActivityTypeBasketball    = activitycatalog.ActivityTypeBasketball
	ActivityTypeFootball      = activitycatalog.ActivityTypeFootball
	ActivityTypeTennis        = activitycatalog.ActivityTypeTennis
	ActivityTypeDancing       = activitycatalog.ActivityTypeDancing
	ActivityTypeSkiing        = activitycatalog.ActivityTypeSkiing
	ActivityTypeClimbing      = activitycatalog.ActivityTypeClimbing
	ActivityTypeWeightlifting = activitycatalog.ActivityTypeWeightlifting
)

type ActivityGroupStatus string

const (
	ActivityGroupStatusDraft     ActivityGroupStatus = "draft"
	ActivityGroupStatusActive    ActivityGroupStatus = "active"
	ActivityGroupStatusCancelled ActivityGroupStatus = "cancelled"
)

type ActivityGroupVisibility string

const (
	ActivityGroupVisibilityPublic  ActivityGroupVisibility = "public"
	ActivityGroupVisibilityPrivate ActivityGroupVisibility = "private"
)

func (v ActivityGroupVisibility) IsValid() bool {
	switch v {
	case ActivityGroupVisibilityPublic, ActivityGroupVisibilityPrivate:
		return true
	default:
		return false
	}
}

func (v ActivityGroupVisibility) String() string {
	return string(v)
}

type DifficultyLevel = activitycatalog.ActivityLevel

const (
	DifficultyLevelBeginner     = activitycatalog.ActivityLevelBeginner
	DifficultyLevelIntermediate = activitycatalog.ActivityLevelIntermediate
	DifficultyLevelAdvanced     = activitycatalog.ActivityLevelAdvanced
)

type Title struct {
	value string
}

func NewTitle(value string) (Title, error) {
	if len(value) == 0 {
		return Title{}, errors.New("title cannot be empty")
	}
	if len(value) > 100 {
		return Title{}, fmt.Errorf("title cannot exceed 100 characters")
	}
	return Title{value: value}, nil
}

func (t Title) Value() string {
	return t.value
}

type Location struct {
	city      string
	country   string
	street    string // optional - venue/address line, e.g. "Ada Ciganlija bb, Court 3"
	latitude  float64
	longitude float64
}

func NewLocation(city, country, street string, latitude, longitude float64) (Location, error) {
	if len(city) == 0 {
		return Location{}, errors.New("city cannot be empty")
	}
	if len(country) == 0 {
		return Location{}, errors.New("country cannot be empty")
	}
	if latitude < -90 || latitude > 90 {
		return Location{}, errors.New("latitude must be between -90 and 90")
	}
	if longitude < -180 || longitude > 180 {
		return Location{}, errors.New("longitude must be between -180 and 180")
	}
	return Location{
		city:      city,
		country:   country,
		street:    street,
		latitude:  latitude,
		longitude: longitude,
	}, nil
}

func (l Location) City() string {
	return l.city
}

func (l Location) Country() string {
	return l.country
}

func (l Location) Street() string {
	return l.street
}

func (l Location) Latitude() float64 {
	return l.latitude
}

func (l Location) Longitude() float64 {
	return l.longitude
}

type AcitvityGroupLocation struct {
	city    string
	country string
}

func NewActivityGroupLocation(city, country string) (AcitvityGroupLocation, error) {
	if len(city) == 0 {
		return AcitvityGroupLocation{}, errors.New("city cannot be empty")
	}
	if len(country) == 0 {
		return AcitvityGroupLocation{}, errors.New("country cannot be empty")
	}
	return AcitvityGroupLocation{
		city:    city,
		country: country,
	}, nil
}

func (l AcitvityGroupLocation) City() string {
	return l.city
}

func (l AcitvityGroupLocation) Country() string {
	return l.country
}

type Schedule struct {
	startTime time.Time
	endTime   time.Time
}

func NewSchedule(startTime, endTime time.Time) (Schedule, error) {
	if endTime.Before(startTime) {
		return Schedule{}, errors.New("end time must be after start time")
	}
	return Schedule{
		startTime: startTime,
		endTime:   endTime,
	}, nil
}

func (s Schedule) StartTime() time.Time {
	return s.startTime
}

func (s Schedule) EndTime() time.Time {
	return s.endTime
}

type Capacity struct {
	max int
}

func NewCapacity(max int) (Capacity, error) {
	if max < 1 {
		return Capacity{}, errors.New("capacity must be at least 1")
	}
	if max > 1000 {
		return Capacity{}, errors.New("capacity cannot exceed 1000")
	}
	return Capacity{max: max}, nil
}

func (c Capacity) Capacity() int {
	return c.max
}

type ActivityGroup struct {
	id          uuid.UUID
	creatorID   uuid.UUID
	title       Title
	description string

	activityType    ActivityType
	difficultyLevel DifficultyLevel
	visibility      ActivityGroupVisibility
	status          ActivityGroupStatus
	timezone        string

	location AcitvityGroupLocation
	capacity *Capacity

	createdAt   time.Time
	updatedAt   time.Time
	cancelledAt *time.Time
	deletedAt   *time.Time

	events []domainevent.Event
}

func NewActivityGroup(
	creatorID uuid.UUID,
	title Title,
	description string,
	activityType ActivityType,
	location AcitvityGroupLocation,
	difficultyLevel DifficultyLevel,
	groupVisibility ActivityGroupVisibility,
	timezone string,
	capacity *Capacity,
) (*ActivityGroup, error) {
	if !activityType.IsValid() {
		return nil, fmt.Errorf("invalid activity type: %s", activityType)
	}

	if !difficultyLevel.IsValid() {
		return nil, fmt.Errorf("invalid difficulty level: %s", difficultyLevel)
	}

	if !groupVisibility.IsValid() {
		return nil, fmt.Errorf("invalid group visibility: %s", groupVisibility)
	}

	now := time.Now()

	ag := &ActivityGroup{
		id:              uuid.New(),
		creatorID:       creatorID,
		title:           title,
		activityType:    activityType,
		location:        location,
		description:     description,
		status:          ActivityGroupStatusDraft,
		timezone:        timezone,
		difficultyLevel: difficultyLevel,
		capacity:        capacity,
		visibility:      groupVisibility,
		createdAt:       now,
		updatedAt:       now,
	}

	ag.addEvent(NewActivityCreatedEvent(ag))

	return ag, nil
}

type ActivityGroupInput struct {
	Title           Title
	Description     string
	ActivityType    ActivityType
	DifficultyLevel DifficultyLevel
	Visibility      ActivityGroupVisibility
	Location        AcitvityGroupLocation
	Timezone        string
	DefaultCapacity *Capacity
}

func ReconstructActivityGroup(
	id uuid.UUID,
	creatorID uuid.UUID,
	title Title,
	description string,
	activityType ActivityType,
	difficultyLevel DifficultyLevel,
	visibility ActivityGroupVisibility,
	status ActivityGroupStatus,
	location AcitvityGroupLocation,
	timezone string,
	capacity *Capacity,
	createdAt time.Time,
	updatedAt time.Time,
	cancelledAt *time.Time,
	deletedAt *time.Time,
) *ActivityGroup {
	return &ActivityGroup{
		id:              id,
		creatorID:       creatorID,
		title:           title,
		description:     description,
		activityType:    activityType,
		difficultyLevel: difficultyLevel,
		visibility:      visibility,
		status:          status,
		location:        location,
		timezone:        timezone,
		capacity:        capacity,
		createdAt:       createdAt,
		updatedAt:       updatedAt,
		cancelledAt:     cancelledAt,
		deletedAt:       deletedAt,
	}
}

func (ag *ActivityGroup) ID() uuid.UUID {
	return ag.id
}

func (ag *ActivityGroup) CreatorID() uuid.UUID {
	return ag.creatorID
}

func (ag *ActivityGroup) Title() Title {
	return ag.title
}

func (ag *ActivityGroup) ActivityType() ActivityType {
	return ag.activityType
}

func (ag *ActivityGroup) DifficultyLevel() DifficultyLevel {
	return ag.difficultyLevel
}

func (ag *ActivityGroup) DefaultCapacity() *Capacity {
	if ag.capacity == nil {
		return nil
	}
	return ag.capacity
}

func (ag *ActivityGroup) Location() AcitvityGroupLocation {
	return ag.location
}

func (ag *ActivityGroup) Description() string {
	return ag.description
}

func (ag *ActivityGroup) Status() ActivityGroupStatus {
	return ag.status
}

func (ag *ActivityGroup) CreatedAt() time.Time {
	return ag.createdAt
}

func (ag *ActivityGroup) UpdatedAt() time.Time {
	return ag.updatedAt
}

func (ag *ActivityGroup) CancelledAt() *time.Time {
	return ag.cancelledAt
}

func (ag *ActivityGroup) DeletedAt() *time.Time {
	return ag.deletedAt
}

func (ag *ActivityGroup) Visibility() ActivityGroupVisibility {
	return ag.visibility
}

func (ag *ActivityGroup) Timezone() string {
	return ag.timezone
}

func (ag *ActivityGroup) IsDeleted() bool {
	return ag.deletedAt != nil
}

func (ag *ActivityGroup) IsActive() bool {
	return ag.status == ActivityGroupStatusActive
}

func (ag *ActivityGroup) IsCancelled() bool {
	return ag.status == ActivityGroupStatusCancelled
}

func (ag *ActivityGroup) CanRequestToJoin() bool {
	return ag.IsActive() && ag.Visibility() == ActivityGroupVisibilityPublic
}

// IsVisibleTo reports whether userID may see this group at all. Public
// groups are visible to everyone; private groups only to their creator or a
// confirmed member - isMember must be resolved by the caller (a DB lookup,
// not something this aggregate can answer on its own).
func (ag *ActivityGroup) IsVisibleTo(userID uuid.UUID, isMember bool) bool {
	if ag.visibility == ActivityGroupVisibilityPublic {
		return true
	}

	if ag.creatorID == userID {
		return true
	}

	return isMember
}

func (ag *ActivityGroup) Update(
	requesterID uuid.UUID,
	title Title,
	description string,
	activityType ActivityType,
	location AcitvityGroupLocation,
	difficultyLevel DifficultyLevel,
	timezone string,
) error {
	if ag.IsCancelled() {
		return ErrActivityGroupCancelled
	}

	if ag.IsDeleted() {
		return ErrActivityGroupDeleted
	}

	if ag.CreatorID() != requesterID {
		return ErrUnauthorized
	}

	if !activityType.IsValid() {
		return fmt.Errorf("invalid activity type: %s", activityType)
	}

	if !difficultyLevel.IsValid() {
		return fmt.Errorf("invalid difficulty level: %s", difficultyLevel)
	}

	ag.title = title
	ag.activityType = activityType
	ag.location = location
	ag.description = description
	ag.timezone = timezone

	ag.touch()
	ag.addEvent(NewActivityUpdatedEvent(ag))
	return nil
}

func (ag *ActivityGroup) SetVisibility(
	requesterID uuid.UUID,
	visibility ActivityGroupVisibility,
) error {
	if !ag.IsActive() {
		return ErrActivityGroupNotActive
	}

	if ag.CreatorID() != requesterID {
		return ErrUnauthorized
	}

	if !visibility.IsValid() {
		return fmt.Errorf("%w: %s", ErrActivityGroupInvalidVisibility, visibility)
	}

	old := ag.visibility
	ag.visibility = visibility

	ag.touch()
	ag.addEvent(NewActivityVisibilityChangedEvent(
		ag,
		old,
		requesterID,
	))
	return nil
}

func (ag *ActivityGroup) Activate(
	requesterID uuid.UUID,
) error {
	if ag.Status() != ActivityGroupStatusDraft {
		return ErrActivityGroupNotDraft
	}

	if ag.IsDeleted() {
		return ErrActivityGroupDeleted
	}

	if ag.CreatorID() != requesterID {
		return ErrUnauthorized
	}

	ag.status = ActivityGroupStatusActive
	ag.touch()

	ag.addEvent(NewActivityPublishedEvent(ag))

	return nil
}

func (ag *ActivityGroup) Cancel(
	requesterID uuid.UUID,
	reason string,
) error {
	if ag.IsCancelled() {
		return ErrActivityGroupCancelled
	}

	if ag.IsDeleted() {
		return ErrActivityGroupDeleted
	}

	if ag.creatorID != requesterID {
		return ErrUnauthorized
	}

	now := time.Now().UTC()

	ag.status = ActivityGroupStatusCancelled
	ag.cancelledAt = &now
	ag.touch()

	ag.addEvent(NewActivityCancelledEvent(ag, requesterID, reason))

	return nil
}

// Delete soft-deletes the activity group. Unlike Cancel, which marks the
// group as called off while keeping it visible to members, Delete removes
// it from listings entirely (deleted_at is checked by list/discover queries).
func (ag *ActivityGroup) Delete(requesterID uuid.UUID) error {
	if ag.IsDeleted() {
		return ErrActivityGroupDeleted
	}

	if ag.creatorID != requesterID {
		return ErrUnauthorized
	}

	now := time.Now().UTC()

	ag.deletedAt = &now
	ag.touch()

	ag.addEvent(NewActivityGroupDeletedEvent(ag, requesterID))

	return nil
}

func (ag *ActivityGroup) ClearEvents() {
	ag.events = make([]domainevent.Event, 0)
}

func (ag *ActivityGroup) Events() []domainevent.Event {
	return ag.events
}

func (ag *ActivityGroup) addEvent(event domainevent.Event) {
	ag.events = append(ag.events, event)
}

func (ag *ActivityGroup) touch() {
	ag.updatedAt = time.Now().UTC()
}
