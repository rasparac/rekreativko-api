package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
)

const (
	// Activity events
	EventActivityGroupCreated           = "activity.group.created"
	EventActivityGroupUpdated           = "activity.group.updated"
	EventActivityGroupCancelled         = "activity.group.cancelled"
	EventActivityGroupDeleted           = "activity.group.deleted"
	EventActivityGroupStarted           = "activity.group.started"
	EventActivityGroupCompleted         = "activity.group.completed"
	EventActivityGroupRecurrenceUpdated = "activity.group.recurrence_updated"
	EventActivityGroupVisibilityChanged = "activity.group.visibility_changed"

	// Activity Member events
	EventActivityGroupMemberJoinRequested   = "activity.member.join_requested"
	EventActivityGroupMemberJoined          = "activity.member.joined"
	EventActivityGroupMemberLeft            = "activity.member.left"
	EventActivityGroupMemberApproved        = "activity.member.approved"
	EventActivityGroupMemberRejected        = "activity.member.rejected"
	EventActivityGroupMemberRemoved         = "activity.member.removed"
	EventActivityGroupMemberRoleChanged     = "activity.member.role_changed"
	EventActivityGroupMemberPrioritySet     = "activity.member.priority_set"
	EventActivityGroupMemberPriorityRemoved = "activity.member.priority_removed"

	// Activity invite events
	EventActivityInviteSent     = "activity.invite.sent"
	EventActivityInviteAccepted = "activity.invite.accepted"
	EventActivityInviteDeclined = "activity.invite.declined"
	EventActivityInviteExpired  = "activity.invite.expired"

	// Session invite events (standalone sessions only)
	EventActivitySessionInviteSent     = "activity.session_invite.sent"
	EventActivitySessionInviteAccepted = "activity.session_invite.accepted"
	EventActivitySessionInviteDeclined = "activity.session_invite.declined"
	EventActivitySessionInviteExpired  = "activity.session_invite.expired"

	// Session events
	EventActivitySessionCreated             = "activity.session.created"
	EventActivitySessionUpdated             = "activity.session.updated"
	EventActivitySessionDeleted             = "activity.session.deleted"
	EventActivitySessionStarted             = "activity.session.started"
	EventActivitySessionCancelled           = "activity.session.cancelled"
	EventActivitySessionCompleted           = "activity.session.completed"
	EventActivitySessionExpired             = "activity.session.expired"
	EventActivitySessionVisibilityChanged   = "activity.session.visibility_changed"
	EventActivitySessionAttendeeAutoPending = "activity.session.attendee.auto_pending"

	// Session attendee events
	EventActivitySessionAttendeeAutoConfirmed   = "activity.session.attendee.auto_confirmed"
	EventActivitySessionAttendeeRSVPGoing       = "activity.session.attendee.rsvp_going"
	EventActivitySessionAttendeeRSVPNotGoing    = "activity.session.attendee.rsvp_not_going"
	EventActivitySessionAttendeeRSVPMaybe       = "activity.session.attendee.rsvp_maybe"
	EventActivitySessionAttendeeRSVPAutoPending = "activity.session.attendee.rsvp_auto_pending"
	EventActivitySessionAttendeePromoted        = "activity.session.attendee.promoted" // e.g. from waiting list to confirmed
	EventActivitySessionAttendeeJoinRequested   = "activity.session.attendee.join_requested"
	EventActivitySessionAttendeeJoinApproved    = "activity.session.attendee.join_approved"
	EventActivitySessionAttendeeJoinRejected    = "activity.session.attendee.join_rejected"
	EventActivitySessionAttendeeRemoved         = "activity.session.attendee.removed"

	// Invite link events
	EventActivityInviteLinkCreated = "activity.invite_link.created"
	EventActivityInviteLinkUsed    = "activity.invite_link.used"
	EventActivityInviteLinkRevoked = "activity.invite_link.revoked"
	EventActivityInviteLinkExpired = "activity.invite_link.expired"

	// SessionTemplate events
	EventActivitySessionTemplateCreated     = "activity.session_template.created"
	EventActivitySessionTemplateActivated   = "activity.session_template.activated"
	EventActivitySessionTemplateDeactivated = "activity.session_template.deactivated"
	EventActivitySessionTemplateDeleted     = "activity.session_template.deleted"
	EventActivitySessionTemplateUpdated     = "activity.session_template.updated"
)

type ActivityCreatedEvent struct {
	domainevent.BaseEvent
	CreatorID       uuid.UUID `json:"creator_id"`
	Title           string    `json:"title"`
	ActivityType    string    `json:"activity_type"`
	LocationCity    string    `json:"location_city"`
	Visibility      string    `json:"visibility"`
	LocationCountry string    `json:"location_country"`
}

func NewActivityCreatedEvent(activity *ActivityGroup) *ActivityCreatedEvent {
	return &ActivityCreatedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivityGroupCreated,
			OccurredAt:  time.Now().UTC(),
			AggregateID: activity.ID(),
		},
		CreatorID:       activity.CreatorID(),
		Title:           activity.Title().Value(),
		ActivityType:    activity.ActivityType().String(),
		LocationCity:    activity.Location().City(),
		LocationCountry: activity.Location().Country(),
		Visibility:      activity.Visibility().String(),
	}
}

type ActivityPublishedEvent struct {
	domainevent.BaseEvent
	CreatorID uuid.UUID `json:"creator_id"`
}

func NewActivityPublishedEvent(activity *ActivityGroup) *ActivityPublishedEvent {
	return &ActivityPublishedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivityGroupUpdated,
			OccurredAt:  time.Now().UTC(),
			AggregateID: activity.ID(),
		},
		CreatorID: activity.CreatorID(),
	}
}

type ActivityUpdatedEvent struct {
	domainevent.BaseEvent
	UpdatedBy uuid.UUID `json:"updated_by"`
}

func NewActivityUpdatedEvent(activity *ActivityGroup) *ActivityUpdatedEvent {
	return &ActivityUpdatedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivityGroupUpdated,
			OccurredAt:  time.Now().UTC(),
			AggregateID: activity.ID(),
		},
		UpdatedBy: activity.CreatorID(),
	}
}

type ActivityRecurrenceUpdatedEvent struct {
	domainevent.BaseEvent
	UpdatedBy uuid.UUID `json:"updated_by"`
}

func NewActivityRecurrenceUpdatedEvent(
	activity *ActivityGroup,
	updatedBy uuid.UUID,
) *ActivityRecurrenceUpdatedEvent {
	return &ActivityRecurrenceUpdatedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivityGroupRecurrenceUpdated,
			OccurredAt:  time.Now().UTC(),
			AggregateID: activity.ID(),
		},
		UpdatedBy: updatedBy,
	}
}

type ActivityVisibilityChangedEvent struct {
	domainevent.BaseEvent
	ChangedBy     uuid.UUID `json:"changed_by"`
	OldVisibility string    `json:"old_visibility"`
	Visibility    string    `json:"visibility"`
}

func NewActivityVisibilityChangedEvent(
	activity *ActivityGroup,
	oldVisibility ActivityGroupVisibility,
	changedBy uuid.UUID,
) *ActivityVisibilityChangedEvent {
	return &ActivityVisibilityChangedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivityGroupVisibilityChanged,
			OccurredAt:  time.Now().UTC(),
			AggregateID: activity.ID(),
		},
		ChangedBy:     changedBy,
		Visibility:    activity.Visibility().String(),
		OldVisibility: oldVisibility.String(),
	}
}

type ActivityCancelledEvent struct {
	domainevent.BaseEvent
	CancelledBy uuid.UUID `json:"cancelled_by"`
	Reason      string    `json:"reason"`
}

func NewActivityCancelledEvent(
	activity *ActivityGroup,
	cancelledBy uuid.UUID,
	reason string,
) *ActivityCancelledEvent {
	return &ActivityCancelledEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivityGroupCancelled,
			OccurredAt:  time.Now().UTC(),
			AggregateID: activity.ID(),
		},
		CancelledBy: cancelledBy,
		Reason:      reason,
	}
}

type ActivityDeletedEvent struct {
	domainevent.BaseEvent
	DeletedBy uuid.UUID `json:"deleted_by"`
}

func NewActivityGroupDeletedEvent(
	activity *ActivityGroup,
	deletedBy uuid.UUID,
) *ActivityDeletedEvent {
	return &ActivityDeletedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivityGroupDeleted,
			OccurredAt:  time.Now().UTC(),
			AggregateID: activity.ID(),
		},
		DeletedBy: deletedBy,
	}
}

type ActivityStartedEvent struct {
	domainevent.BaseEvent
	StartedBy uuid.UUID `json:"started_by"`
}

func NewActivityStartedEvent(activity *ActivityGroup, startedBy uuid.UUID) *ActivityStartedEvent {
	return &ActivityStartedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivityGroupStarted,
			OccurredAt:  time.Now().UTC(),
			AggregateID: activity.ID(),
		},
		StartedBy: startedBy,
	}
}

type ActivityCompletedEvent struct {
	domainevent.BaseEvent
	CompletedBy uuid.UUID `json:"completed_by"`
}

func NewActivityCompletedEvent(activity *ActivityGroup, completedBy uuid.UUID) *ActivityCompletedEvent {
	return &ActivityCompletedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivityGroupCompleted,
			OccurredAt:  time.Now().UTC(),
			AggregateID: activity.ID(),
		},
		CompletedBy: completedBy,
	}
}

type MemberJoinRequestedEvent struct {
	domainevent.BaseEvent
	ActivityGroupID uuid.UUID `json:"activity_id"`
	UserID          uuid.UUID `json:"user_id"`
	// ManagerUserIDs are the group's confirmed admins/creator at request time -
	// carried on the event so consumers (e.g. notifications) can resolve who
	// to notify without needing to query activity's own schema.
	ManagerUserIDs []uuid.UUID `json:"manager_user_ids"`
}

func NewMemberJoinRequestedEvent(
	member *Member,
	managerUserIDs []uuid.UUID,
) *MemberJoinRequestedEvent {
	return &MemberJoinRequestedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivityGroupMemberJoinRequested,
			OccurredAt:  time.Now().UTC(),
			AggregateID: member.ActivityGroupID(),
		},
		ActivityGroupID: member.ActivityGroupID(),
		UserID:          member.UserID(),
		ManagerUserIDs:  managerUserIDs,
	}
}

type MemberJoinedEvent struct {
	domainevent.BaseEvent
	ActivityGroupID uuid.UUID `json:"activity_id"`
	UserID          uuid.UUID `json:"user_id"`
	ViaInvite       bool      `json:"via_invite"`
}

func NewMemberJoinedEvent(
	m *Member,
	viaInvite bool,
) *MemberJoinedEvent {
	return &MemberJoinedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivityGroupMemberJoined,
			OccurredAt:  time.Now().UTC(),
			AggregateID: m.ActivityGroupID(),
		},
		ActivityGroupID: m.ActivityGroupID(),
		UserID:          m.UserID(),
		ViaInvite:       viaInvite,
	}
}

type MemberApprovedEvent struct {
	domainevent.BaseEvent
	ActivityID uuid.UUID `json:"activity_id"`
	ApprovedBy uuid.UUID `json:"approved_by"`
	UserID     uuid.UUID `json:"user_id"`
}

func NewMemberApprovedEvent(
	Member *Member,
	approvedBy uuid.UUID,
) *MemberApprovedEvent {
	return &MemberApprovedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivityGroupMemberApproved,
			OccurredAt:  time.Now().UTC(),
			AggregateID: Member.ActivityGroupID(),
		},
		ActivityID: Member.ActivityGroupID(),
		ApprovedBy: approvedBy,
		UserID:     Member.UserID(),
	}
}

type MemberRejectedEvent struct {
	domainevent.BaseEvent
	ActivityID uuid.UUID `json:"activity_id"`
	RejectedBy uuid.UUID `json:"rejected_by"`
	UserID     uuid.UUID `json:"user_id"`
}

func NewMemberRejectedEvent(
	Member *Member,
	rejectedBy uuid.UUID,
) *MemberRejectedEvent {
	return &MemberRejectedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivityGroupMemberRejected,
			OccurredAt:  time.Now().UTC(),
			AggregateID: Member.ActivityGroupID(),
		},
		ActivityID: Member.ActivityGroupID(),
		RejectedBy: rejectedBy,
		UserID:     Member.UserID(),
	}
}

type MemberLeftEvent struct {
	domainevent.BaseEvent
	ActivityID uuid.UUID `json:"activity_id"`
	UserID     uuid.UUID `json:"user_id"`
}

func NewMemberLeftEvent(Member *Member) *MemberLeftEvent {
	return &MemberLeftEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivityGroupMemberLeft,
			OccurredAt:  time.Now().UTC(),
			AggregateID: Member.ActivityGroupID(),
		},
		ActivityID: Member.ActivityGroupID(),
		UserID:     Member.UserID(),
	}
}

type MemberRemovedEvent struct {
	domainevent.BaseEvent
	ActivityID uuid.UUID `json:"activity_id"`
	RemovedBy  uuid.UUID `json:"removed_by"`
	UserID     uuid.UUID `json:"user_id"`
}

func NewMemberRemovedEvent(
	Member *Member,
	removedBy uuid.UUID,
) *MemberRemovedEvent {
	return &MemberRemovedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivityGroupMemberRemoved,
			OccurredAt:  time.Now().UTC(),
			AggregateID: Member.ActivityGroupID(),
		},
		ActivityID: Member.ActivityGroupID(),
		RemovedBy:  removedBy,
		UserID:     Member.UserID(),
	}
}

type MemberRoleChangedEvent struct {
	domainevent.BaseEvent
	ActivityID uuid.UUID `json:"activity_id"`
	ChangedBy  uuid.UUID `json:"changed_by"`
	UserID     uuid.UUID `json:"user_id"`
	NewRole    string    `json:"new_role"`
	OldRole    string    `json:"old_role"`
}

func NewMemberRoleChangedEvent(
	Member *Member,
	changedBy uuid.UUID,
	newRole MemberRole,
	oldRole MemberRole,
) *MemberRoleChangedEvent {
	return &MemberRoleChangedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivityGroupMemberRoleChanged,
			OccurredAt:  time.Now().UTC(),
			AggregateID: Member.ActivityGroupID(),
		},
		ActivityID: Member.ActivityGroupID(),
		ChangedBy:  changedBy,
		UserID:     Member.UserID(),
		NewRole:    newRole.String(),
		OldRole:    oldRole.String(),
	}
}

type MemberPrioritySetEvent struct {
	domainevent.BaseEvent
	ActivityID uuid.UUID `json:"activity_id"`
	SetBy      uuid.UUID `json:"set_by"`
	UserID     uuid.UUID `json:"user_id"`
}

func NewMemberPrioritySetEvent(
	Member *Member,
	setBy uuid.UUID,
) *MemberPrioritySetEvent {
	return &MemberPrioritySetEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivityGroupMemberPrioritySet,
			OccurredAt:  time.Now().UTC(),
			AggregateID: Member.ActivityGroupID(),
		},
		ActivityID: Member.ActivityGroupID(),
		SetBy:      setBy,
		UserID:     Member.UserID(),
	}
}

type MemberPriorityRemovedEvent struct {
	domainevent.BaseEvent
	ActivityID uuid.UUID `json:"activity_id"`
	RemovedBy  uuid.UUID `json:"removed_by"`
	UserID     uuid.UUID `json:"user_id"`
}

func NewMemberPriorityRemovedEvent(
	Member *Member,
	removedBy uuid.UUID,
) *MemberPriorityRemovedEvent {
	return &MemberPriorityRemovedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivityGroupMemberPriorityRemoved,
			OccurredAt:  time.Now().UTC(),
			AggregateID: Member.ActivityGroupID(),
		},
		ActivityID: Member.ActivityGroupID(),
		RemovedBy:  removedBy,
		UserID:     Member.UserID(),
	}
}

type InviteSentEvent struct {
	domainevent.BaseEvent
	ActivityID    uuid.UUID `json:"activity_id"`
	InvitedBy     uuid.UUID `json:"invited_by"`
	InvitedUserID uuid.UUID `json:"invited_user_id"`
	ExpiresAt     time.Time `json:"expires_at"`
}

func NewInviteSentEvent(invite *GroupInvite) *InviteSentEvent {
	return &InviteSentEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivityInviteSent,
			OccurredAt:  time.Now().UTC(),
			AggregateID: invite.ID(),
		},
		ActivityID:    invite.ActivityGroupID(),
		InvitedBy:     invite.InvitedByID(),
		InvitedUserID: invite.InvitedUserID(),
		ExpiresAt:     invite.ExpiresAt(),
	}
}

type InviteAcceptedEvent struct {
	domainevent.BaseEvent
	ActivityID    uuid.UUID `json:"activity_id"`
	InvitedBy     uuid.UUID `json:"invited_by"`
	InvitedUserID uuid.UUID `json:"invited_user_id"`
}

func NewInviteAcceptedEvent(invite *GroupInvite) *InviteAcceptedEvent {
	return &InviteAcceptedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivityInviteAccepted,
			OccurredAt:  time.Now().UTC(),
			AggregateID: invite.ID(),
		},
		ActivityID:    invite.ActivityGroupID(),
		InvitedBy:     invite.InvitedByID(),
		InvitedUserID: invite.InvitedUserID(),
	}
}

type InviteDeclinedEvent struct {
	domainevent.BaseEvent
	ActivityID    uuid.UUID `json:"activity_id"`
	InvitedBy     uuid.UUID `json:"invited_by"`
	InvitedUserID uuid.UUID `json:"invited_user_id"`
}

func NewInviteDeclinedEvent(invite *GroupInvite) *InviteDeclinedEvent {
	return &InviteDeclinedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivityInviteDeclined,
			OccurredAt:  time.Now().UTC(),
			AggregateID: invite.ID(),
		},
		ActivityID:    invite.ActivityGroupID(),
		InvitedBy:     invite.InvitedByID(),
		InvitedUserID: invite.InvitedUserID(),
	}
}

type InviteExpiredEvent struct {
	domainevent.BaseEvent
	ActivityID    uuid.UUID `json:"activity_id"`
	InvitedBy     uuid.UUID `json:"invited_by"`
	InvitedUserID uuid.UUID `json:"invited_user_id"`
}

func NewInviteExpiredEvent(invite *GroupInvite) *InviteExpiredEvent {
	return &InviteExpiredEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivityInviteExpired,
			OccurredAt:  time.Now().UTC(),
			AggregateID: invite.ID(),
		},
		InvitedBy:     invite.InvitedByID(),
		ActivityID:    invite.ActivityGroupID(),
		InvitedUserID: invite.InvitedUserID(),
	}
}

type SessionInviteSentEvent struct {
	domainevent.BaseEvent
	SessionID     uuid.UUID `json:"session_id"`
	InvitedBy     uuid.UUID `json:"invited_by"`
	InvitedUserID uuid.UUID `json:"invited_user_id"`
	ExpiresAt     time.Time `json:"expires_at"`
}

func NewSessionInviteSentEvent(invite *SessionInvite) *SessionInviteSentEvent {
	return &SessionInviteSentEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivitySessionInviteSent,
			OccurredAt:  time.Now().UTC(),
			AggregateID: invite.ID(),
		},
		SessionID:     invite.SessionID(),
		InvitedBy:     invite.InvitedByID(),
		InvitedUserID: invite.InvitedUserID(),
		ExpiresAt:     invite.ExpiresAt(),
	}
}

type SessionInviteAcceptedEvent struct {
	domainevent.BaseEvent
	SessionID     uuid.UUID `json:"session_id"`
	InvitedBy     uuid.UUID `json:"invited_by"`
	InvitedUserID uuid.UUID `json:"invited_user_id"`
}

func NewSessionInviteAcceptedEvent(invite *SessionInvite) *SessionInviteAcceptedEvent {
	return &SessionInviteAcceptedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivitySessionInviteAccepted,
			OccurredAt:  time.Now().UTC(),
			AggregateID: invite.ID(),
		},
		SessionID:     invite.SessionID(),
		InvitedBy:     invite.InvitedByID(),
		InvitedUserID: invite.InvitedUserID(),
	}
}

type SessionInviteDeclinedEvent struct {
	domainevent.BaseEvent
	SessionID     uuid.UUID `json:"session_id"`
	InvitedBy     uuid.UUID `json:"invited_by"`
	InvitedUserID uuid.UUID `json:"invited_user_id"`
}

func NewSessionInviteDeclinedEvent(invite *SessionInvite) *SessionInviteDeclinedEvent {
	return &SessionInviteDeclinedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivitySessionInviteDeclined,
			OccurredAt:  time.Now().UTC(),
			AggregateID: invite.ID(),
		},
		SessionID:     invite.SessionID(),
		InvitedBy:     invite.InvitedByID(),
		InvitedUserID: invite.InvitedUserID(),
	}
}

type SessionInviteExpiredEvent struct {
	domainevent.BaseEvent
	SessionID     uuid.UUID `json:"session_id"`
	InvitedBy     uuid.UUID `json:"invited_by"`
	InvitedUserID uuid.UUID `json:"invited_user_id"`
}

func NewSessionInviteExpiredEvent(invite *SessionInvite) *SessionInviteExpiredEvent {
	return &SessionInviteExpiredEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivitySessionInviteExpired,
			OccurredAt:  time.Now().UTC(),
			AggregateID: invite.ID(),
		},
		SessionID:     invite.SessionID(),
		InvitedBy:     invite.InvitedByID(),
		InvitedUserID: invite.InvitedUserID(),
	}
}

type InviteLinkCreatedEvent struct {
	domainevent.BaseEvent
	GroupID   uuid.UUID `json:"group_id"`
	CreatedBy uuid.UUID `json:"created_by"`
	ExpiresAt time.Time `json:"expires_at"`
}

func NewInviteLinkCreatedEvent(link *InviteLink) *InviteLinkCreatedEvent {
	return &InviteLinkCreatedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivityInviteLinkCreated,
			OccurredAt:  time.Now().UTC(),
			AggregateID: link.ID(),
		},
		GroupID:   link.groupID,
		CreatedBy: link.createdBy,
		ExpiresAt: link.expiresAt,
	}
}

type InviteLinkUsedEvent struct {
	domainevent.BaseEvent
	GroupID uuid.UUID `json:"group_id"`
	UsedBy  uuid.UUID `json:"used_by"`
}

func NewInviteLinkUsedEvent(
	il *InviteLink,
	usedBy uuid.UUID,
) *InviteLinkUsedEvent {
	return &InviteLinkUsedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivityInviteLinkUsed,
			OccurredAt:  time.Now().UTC(),
			AggregateID: il.ID(),
		},
		GroupID: il.GroupID(),
		UsedBy:  usedBy,
	}
}

type InviteLinkRevokedEvent struct {
	domainevent.BaseEvent
	GroupID   uuid.UUID `json:"group_id"`
	RevokedBy uuid.UUID `json:"revoked_by"`
}

func NewInviteLinkRevokedEvent(
	il *InviteLink,
	revokedBy uuid.UUID,
) *InviteLinkRevokedEvent {
	return &InviteLinkRevokedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivityInviteLinkRevoked,
			OccurredAt:  time.Now().UTC(),
			AggregateID: il.ID(),
		},
		GroupID:   il.GroupID(),
		RevokedBy: revokedBy,
	}
}

type InviteLinkExpiredEvent struct {
	domainevent.BaseEvent
	GroupID uuid.UUID `json:"group_id"`
}

func NewInviteLinkExpiredEvent(
	il *InviteLink,
) *InviteLinkExpiredEvent {
	return &InviteLinkExpiredEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivityInviteLinkExpired,
			OccurredAt:  time.Now().UTC(),
			AggregateID: il.ID(),
		},
		GroupID: il.GroupID(),
	}
}

type SessionCreatedEvent struct {
	domainevent.BaseEvent
	ActivityID  *uuid.UUID `json:"activity_id"`
	CreatedBy   uuid.UUID  `json:"created_by"`
	StartTime   time.Time  `json:"start_time"`
	EndTime     *time.Time `json:"end_time"`
	IsRecurring bool       `json:"is_recurring"`
}

func NewSessionCreatedEvent(
	session *Session,
	createdBy uuid.UUID,
) *SessionCreatedEvent {
	return &SessionCreatedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivitySessionCreated,
			OccurredAt:  time.Now().UTC(),
			AggregateID: session.ID(),
		},
		ActivityID:  session.ActivityGroupID(),
		CreatedBy:   createdBy,
		StartTime:   session.Schedule().StartTime(),
		EndTime:     session.Schedule().EndTime(),
		IsRecurring: session.IsRecurring(),
	}
}

type SessionAttendeeAutoConfirmedEvent struct {
	domainevent.BaseEvent
	ActivityID *uuid.UUID `json:"activity_id"`
	UserID     uuid.UUID  `json:"user_id"`
}

func NewSessionAttendeeAutoConfirmedEvent(
	sessionID uuid.UUID,
	activityID *uuid.UUID,
	userID uuid.UUID,
) *SessionAttendeeAutoConfirmedEvent {
	return &SessionAttendeeAutoConfirmedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivitySessionAttendeeAutoConfirmed,
			OccurredAt:  time.Now().UTC(),
			AggregateID: sessionID,
		},
		ActivityID: activityID,
		UserID:     userID,
	}
}

type SessionAttendeeAutoPendingEvent struct {
	domainevent.BaseEvent
	ActivityID *uuid.UUID `json:"activity_id"`
	UserID     uuid.UUID  `json:"user_id"`
}

func NewSessionAttendeeAutoPendingEvent(
	sessionID uuid.UUID,
	activityID *uuid.UUID,
	userID uuid.UUID,
) *SessionAttendeeAutoPendingEvent {
	return &SessionAttendeeAutoPendingEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivitySessionAttendeeAutoPending,
			OccurredAt:  time.Now().UTC(),
			AggregateID: sessionID,
		},
		UserID:     userID,
		ActivityID: activityID,
	}
}

type SessionUpdatedEvent struct {
	domainevent.BaseEvent
	ActivityID *uuid.UUID `json:"activity_id"`
	UpdatedBy  uuid.UUID  `json:"updated_by"`
}

func NewSessionUpdatedEvent(
	session *Session,
	updatedBy uuid.UUID,
) *SessionUpdatedEvent {
	return &SessionUpdatedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivitySessionUpdated,
			OccurredAt:  time.Now().UTC(),
			AggregateID: session.ID(),
		},
		ActivityID: session.ActivityGroupID(),
		UpdatedBy:  updatedBy,
	}
}

type SessionVisibilityChangedEvent struct {
	domainevent.BaseEvent
	ActivityID    *uuid.UUID `json:"activity_id"`
	ChangedBy     uuid.UUID  `json:"changed_by"`
	OldVisibility string     `json:"old_visibility"`
	Visibility    string     `json:"visibility"`
}

func NewSessionVisibilityChangedEvent(
	session *Session,
	oldVisibility SessionVisibility,
	changedBy uuid.UUID,
) *SessionVisibilityChangedEvent {
	return &SessionVisibilityChangedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivitySessionVisibilityChanged,
			OccurredAt:  time.Now().UTC(),
			AggregateID: session.ID(),
		},
		ActivityID:    session.ActivityGroupID(),
		ChangedBy:     changedBy,
		Visibility:    session.Visibility().String(),
		OldVisibility: oldVisibility.String(),
	}
}

// SessionCancelledEvent carries AttendeeUserIDs (the session's attendees at
// cancellation time, resolved by the application layer) so the notifications
// service can fan out a notification to each of them without needing to
// query activity's own schema - mirrors the ManagerUserIDs convention used by
// MemberJoinRequestedEvent.
type SessionCancelledEvent struct {
	domainevent.BaseEvent
	ActivityID         *uuid.UUID  `json:"activity_id"`
	CancelledBy        uuid.UUID   `json:"cancelled_by"`
	CancellationReason string      `json:"cancellation_reason"`
	AttendeeUserIDs    []uuid.UUID `json:"attendee_user_ids"`
}

func NewSessionCancelledEvent(
	session *Session,
	cancelledBy uuid.UUID,
	cancellationReason string,
	attendeeUserIDs []uuid.UUID,
) *SessionCancelledEvent {
	return &SessionCancelledEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivitySessionCancelled,
			OccurredAt:  time.Now().UTC(),
			AggregateID: session.ID(),
		},
		ActivityID:         session.ActivityGroupID(),
		CancelledBy:        cancelledBy,
		CancellationReason: cancellationReason,
		AttendeeUserIDs:    attendeeUserIDs,
	}
}

type SessionStartedEvent struct {
	domainevent.BaseEvent
	ActivityID *uuid.UUID `json:"activity_id"`
	StartedBy  uuid.UUID  `json:"started_by"`
}

func NewSessionStartedEvent(
	session *Session,
	startedBy uuid.UUID,
) *SessionStartedEvent {
	return &SessionStartedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivitySessionStarted,
			OccurredAt:  time.Now().UTC(),
			AggregateID: session.ID(),
		},
		ActivityID: session.ActivityGroupID(),
		StartedBy:  startedBy,
	}
}

type SessionCompletedEvent struct {
	domainevent.BaseEvent
	ActivityID    *uuid.UUID `json:"activity_id"`
	CompletedByID uuid.UUID  `json:"completed_by_id"`
	IsRecurring   bool       `json:"is_recurring"`
}

func NewSessionCompletedEvent(
	session *Session,
	completedByID uuid.UUID,
) *SessionCompletedEvent {
	return &SessionCompletedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivitySessionCompleted,
			OccurredAt:  time.Now().UTC(),
			AggregateID: session.ID(),
		},
		ActivityID:    session.ActivityGroupID(),
		CompletedByID: completedByID,
		IsRecurring:   session.IsRecurring(),
	}
}

// SessionExpiredEvent marks a session auto-completed by the cron sweep once its
// end time has passed - no actor, unlike SessionCompletedEvent, since nothing
// user-initiated caused this transition.
type SessionExpiredEvent struct {
	domainevent.BaseEvent
	ActivityID  *uuid.UUID `json:"activity_id"`
	IsRecurring bool       `json:"is_recurring"`
}

func NewSessionExpiredEvent(session *Session) *SessionExpiredEvent {
	return &SessionExpiredEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivitySessionExpired,
			OccurredAt:  time.Now().UTC(),
			AggregateID: session.ID(),
		},
		ActivityID:  session.ActivityGroupID(),
		IsRecurring: session.IsRecurring(),
	}
}

type AttendeeRSVPAutoPendingEvent struct {
	domainevent.BaseEvent
	ActivityID *uuid.UUID `json:"activity_id"`
	UserID     uuid.UUID  `json:"user_id"`
	SessionID  uuid.UUID  `json:"session_id"`
}

func NewAttendeeRSVPAutoPendingEvent(
	a *Attendee,
	s *Session,
) *AttendeeRSVPAutoPendingEvent {
	return &AttendeeRSVPAutoPendingEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivitySessionAttendeeRSVPAutoPending,
			OccurredAt:  time.Now().UTC(),
			AggregateID: uuid.Nil,
		},
		ActivityID: s.ActivityGroupID(),
		UserID:     a.UserID(),
		SessionID:  s.ID(),
	}
}

type AttendeeRSVPGoingEvent struct {
	domainevent.BaseEvent
	ActivityID *uuid.UUID `json:"activity_id"`
	UserID     uuid.UUID  `json:"user_id"`
	SessionID  uuid.UUID  `json:"session_id"`
}

func NewAttendeeRSVPGoingEvent(
	a *Attendee,
	s *Session,
) *AttendeeRSVPGoingEvent {
	return &AttendeeRSVPGoingEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivitySessionAttendeeRSVPGoing,
			OccurredAt:  time.Now().UTC(),
			AggregateID: a.ID(),
		},
		ActivityID: s.ActivityGroupID(),
		UserID:     a.UserID(),
		SessionID:  s.ID(),
	}
}

type AttendeeRSVPNotGoingEvent struct {
	domainevent.BaseEvent
	ActivityID *uuid.UUID `json:"activity_id"`
	UserID     uuid.UUID  `json:"user_id"`
	SessionID  uuid.UUID  `json:"session_id"`
	HoldSpot   bool       `json:"hold_spot"`
}

func NewAttendeeRSVPNotGoingEvent(
	a *Attendee,
	s *Session,
	holdSpot bool,
) *AttendeeRSVPNotGoingEvent {
	return &AttendeeRSVPNotGoingEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivitySessionAttendeeRSVPNotGoing,
			OccurredAt:  time.Now().UTC(),
			AggregateID: a.ID(),
		},
		ActivityID: s.ActivityGroupID(),
		UserID:     a.UserID(),
		SessionID:  s.ID(),
		HoldSpot:   holdSpot,
	}
}

type AttendeeRSVPMaybeEvent struct {
	domainevent.BaseEvent
	ActivityID *uuid.UUID `json:"activity_id"`
	UserID     uuid.UUID  `json:"user_id"`
	SessionID  uuid.UUID  `json:"session_id"`
	HoldSpot   bool       `json:"hold_spot"`
}

func NewAttendeeRSVPMaybeEvent(
	a *Attendee,
	s *Session,
	holdSpot bool,
) *AttendeeRSVPMaybeEvent {
	return &AttendeeRSVPMaybeEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivitySessionAttendeeRSVPMaybe,
			OccurredAt:  time.Now().UTC(),
			AggregateID: a.ID(),
		},
		ActivityID: s.ActivityGroupID(),
		UserID:     a.UserID(),
		SessionID:  s.ID(),
		HoldSpot:   holdSpot,
	}
}

type AttendeePromotedEvent struct {
	domainevent.BaseEvent
	SessionID uuid.UUID `json:"session_id"`
	UserID    uuid.UUID `json:"user_id"`
}

func NewAttendeePromotedEvent(a *Attendee) *AttendeePromotedEvent {
	return &AttendeePromotedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivitySessionAttendeePromoted,
			OccurredAt:  time.Now().UTC(),
			AggregateID: a.ID(),
		},
		SessionID: a.SessionID(),
		UserID:    a.UserID(),
	}
}

// AttendeeJoinRequestedEvent is raised when a user RSVPs "going" on a session
// that requires creator/admin approval - carries ManagerUserIDs directly
// (resolved at request time) so the notifications service never needs to
// query membership data itself, mirroring MemberJoinRequestedEvent.
type AttendeeJoinRequestedEvent struct {
	domainevent.BaseEvent
	ActivityID     *uuid.UUID  `json:"activity_id"`
	SessionID      uuid.UUID   `json:"session_id"`
	UserID         uuid.UUID   `json:"user_id"`
	ManagerUserIDs []uuid.UUID `json:"manager_user_ids"`
}

func NewAttendeeJoinRequestedEvent(a *Attendee, s *Session, managerUserIDs []uuid.UUID) *AttendeeJoinRequestedEvent {
	return &AttendeeJoinRequestedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivitySessionAttendeeJoinRequested,
			OccurredAt:  time.Now().UTC(),
			AggregateID: a.ID(),
		},
		ActivityID:     s.ActivityGroupID(),
		SessionID:      s.ID(),
		UserID:         a.UserID(),
		ManagerUserIDs: managerUserIDs,
	}
}

type AttendeeJoinApprovedEvent struct {
	domainevent.BaseEvent
	ActivityID *uuid.UUID `json:"activity_id"`
	SessionID  uuid.UUID  `json:"session_id"`
	UserID     uuid.UUID  `json:"user_id"`
	ApprovedBy uuid.UUID  `json:"approved_by"`
}

func NewAttendeeJoinApprovedEvent(a *Attendee, s *Session, approvedBy uuid.UUID) *AttendeeJoinApprovedEvent {
	return &AttendeeJoinApprovedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivitySessionAttendeeJoinApproved,
			OccurredAt:  time.Now().UTC(),
			AggregateID: a.ID(),
		},
		ActivityID: s.ActivityGroupID(),
		SessionID:  s.ID(),
		UserID:     a.UserID(),
		ApprovedBy: approvedBy,
	}
}

type AttendeeJoinRejectedEvent struct {
	domainevent.BaseEvent
	ActivityID *uuid.UUID `json:"activity_id"`
	SessionID  uuid.UUID  `json:"session_id"`
	UserID     uuid.UUID  `json:"user_id"`
	RejectedBy uuid.UUID  `json:"rejected_by"`
}

func NewAttendeeJoinRejectedEvent(a *Attendee, s *Session, rejectedBy uuid.UUID) *AttendeeJoinRejectedEvent {
	return &AttendeeJoinRejectedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivitySessionAttendeeJoinRejected,
			OccurredAt:  time.Now().UTC(),
			AggregateID: a.ID(),
		},
		ActivityID: s.ActivityGroupID(),
		SessionID:  s.ID(),
		UserID:     a.UserID(),
		RejectedBy: rejectedBy,
	}
}

type AttendeeRemovedEvent struct {
	domainevent.BaseEvent
	ActivityID *uuid.UUID `json:"activity_id"`
	SessionID  uuid.UUID  `json:"session_id"`
	UserID     uuid.UUID  `json:"user_id"`
	RemovedBy  uuid.UUID  `json:"removed_by"`
}

func NewAttendeeRemovedEvent(a *Attendee, s *Session, removedBy uuid.UUID) *AttendeeRemovedEvent {
	return &AttendeeRemovedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivitySessionAttendeeRemoved,
			OccurredAt:  time.Now().UTC(),
			AggregateID: a.ID(),
		},
		ActivityID: s.ActivityGroupID(),
		SessionID:  s.ID(),
		UserID:     a.UserID(),
		RemovedBy:  removedBy,
	}
}

type SessionTemplateCreatedEvent struct {
	domainevent.BaseEvent
	ActivityGroupID uuid.UUID `json:"activity_group_id"`
	CreatedBy       uuid.UUID `json:"created_by"`
	Title           string    `json:"title"`
	IsActive        bool      `json:"is_active"`
}

func NewSessionTemplateCreatedEvent(st *SessionTemplate) *SessionTemplateCreatedEvent {
	return &SessionTemplateCreatedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivitySessionTemplateCreated,
			OccurredAt:  time.Now().UTC(),
			AggregateID: st.ID(),
		},
		ActivityGroupID: st.ActivityGroupID(),
		CreatedBy:       st.CreatedByID(),
		Title:           st.Title(),
		IsActive:        st.IsActive(),
	}
}

type SessionTemplateActivatedEvent struct {
	domainevent.BaseEvent
	ActivityGroupID uuid.UUID `json:"activity_group_id"`
}

func NewSessionTemplateActivatedEvent(st *SessionTemplate) *SessionTemplateActivatedEvent {
	return &SessionTemplateActivatedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivitySessionTemplateActivated,
			OccurredAt:  time.Now().UTC(),
			AggregateID: st.ID(),
		},
		ActivityGroupID: st.ActivityGroupID(),
	}
}

type SessionTemplateDeactivatedEvent struct {
	domainevent.BaseEvent
	ActivityGroupID uuid.UUID `json:"activity_group_id"`
}

func NewSessionTemplateDeactivatedEvent(st *SessionTemplate) *SessionTemplateDeactivatedEvent {
	return &SessionTemplateDeactivatedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivitySessionTemplateDeactivated,
			OccurredAt:  time.Now().UTC(),
			AggregateID: st.ID(),
		},
		ActivityGroupID: st.ActivityGroupID(),
	}
}

type SessionTemplateDeletedEvent struct {
	domainevent.BaseEvent
	ActivityGroupID uuid.UUID `json:"activity_group_id"`
}

func NewSessionTemplateDeletedEvent(st *SessionTemplate) *SessionTemplateDeletedEvent {
	return &SessionTemplateDeletedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivitySessionTemplateDeleted,
			OccurredAt:  time.Now().UTC(),
			AggregateID: st.ID(),
		},
		ActivityGroupID: st.ActivityGroupID(),
	}
}

type SessionTemplateUpdatedEvent struct {
	domainevent.BaseEvent
	ActivityGroupID uuid.UUID `json:"activity_group_id"`
}

func NewSessionTemplateUpdatedEvent(st *SessionTemplate) *SessionTemplateUpdatedEvent {
	return &SessionTemplateUpdatedEvent{
		BaseEvent: domainevent.BaseEvent{
			EventID:     uuid.New(),
			EventType:   EventActivitySessionTemplateUpdated,
			OccurredAt:  time.Now().UTC(),
			AggregateID: st.ID(),
		},
		ActivityGroupID: st.ActivityGroupID(),
	}
}
