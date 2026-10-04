package events

import (
	"context"

	"github.com/rasparac/rekreativko-api/shared/events"
	"github.com/rasparac/rekreativko-api/shared/logger"
)

// identityNotifier sends the actual email/SMS for an identity account
// lifecycle event - implemented by shared/notification.Service. Moved here
// from gateway, which had no business owning delivery-channel concerns.
type identityNotifier interface {
	HandleAccountVerified(ctx context.Context, payload []byte) error
	HandleAccountLocked(ctx context.Context, payload []byte) error
	HandlePasswordChanged(ctx context.Context, payload []byte) error
	HandleVerificationCodeGenerated(ctx context.Context, payload []byte) error
}

// Subscriber wires every notification-producing domain event this service
// cares about to its handler. Each Subscribe call creates its own durable
// JetStream consumer (named after this service + the subject), independent
// of any other service subscribed to the same event.
type Subscriber struct {
	broker                       events.MessageBroker
	identityNotifier             identityNotifier
	memberJoinRequestedHandler   *memberJoinRequestedHandler
	memberApprovedHandler        *memberApprovedHandler
	memberRejectedHandler        *memberRejectedHandler
	attendeePromotedHandler      *attendeePromotedHandler
	attendeeJoinRequestedHandler *attendeeJoinRequestedHandler
	attendeeJoinApprovedHandler  *attendeeJoinApprovedHandler
	attendeeJoinRejectedHandler  *attendeeJoinRejectedHandler
	attendeeRemovedHandler       *attendeeRemovedHandler
	memberRemovedHandler         *memberRemovedHandler
	sessionCancelledHandler      *sessionCancelledHandler
	inviteSentHandler            *inviteSentHandler
	inviteAcceptedHandler        *inviteAcceptedHandler
	inviteDeclinedHandler        *inviteDeclinedHandler
	inviteExpiredHandler         *inviteExpiredHandler
	sessionInviteSentHandler     *sessionInviteSentHandler
	sessionInviteAcceptedHandler *sessionInviteAcceptedHandler
	sessionInviteDeclinedHandler *sessionInviteDeclinedHandler
	sessionInviteExpiredHandler  *sessionInviteExpiredHandler
	draftStartedHandler          *draftStartedHandler
	draftTurnChangedHandler      *draftTurnChangedHandler
	draftPausedHandler           *draftPausedHandler
	draftCompletedHandler        *draftCompletedHandler
	draftCancelledHandler        *draftCancelledHandler
	votingOpenedHandler          *votingOpenedHandler
	votingClosedHandler          *votingClosedHandler
	votingCancelledHandler       *votingCancelledHandler
	votingTiedHandler            *votingTiedHandler
	teamsResetHandler            *teamsResetHandler
	attendeeTeamChangedHandler   *attendeeTeamChangedHandler
}

func NewSubscriber(
	broker events.MessageBroker,
	notifications notificationCreator,
	identityNotifier identityNotifier,
	logger *logger.Logger,
) *Subscriber {
	return &Subscriber{
		broker:                       broker,
		identityNotifier:             identityNotifier,
		memberJoinRequestedHandler:   &memberJoinRequestedHandler{notifications: notifications, logger: logger},
		memberApprovedHandler:        &memberApprovedHandler{notifications: notifications, logger: logger},
		memberRejectedHandler:        &memberRejectedHandler{notifications: notifications, logger: logger},
		attendeePromotedHandler:      &attendeePromotedHandler{notifications: notifications, logger: logger},
		attendeeJoinRequestedHandler: &attendeeJoinRequestedHandler{notifications: notifications, logger: logger},
		attendeeJoinApprovedHandler:  &attendeeJoinApprovedHandler{notifications: notifications, logger: logger},
		attendeeJoinRejectedHandler:  &attendeeJoinRejectedHandler{notifications: notifications, logger: logger},
		attendeeRemovedHandler:       &attendeeRemovedHandler{notifications: notifications, logger: logger},
		memberRemovedHandler:         &memberRemovedHandler{notifications: notifications, logger: logger},
		sessionCancelledHandler:      &sessionCancelledHandler{notifications: notifications, logger: logger},
		inviteSentHandler:            &inviteSentHandler{notifications: notifications, logger: logger},
		inviteAcceptedHandler:        &inviteAcceptedHandler{notifications: notifications, logger: logger},
		inviteDeclinedHandler:        &inviteDeclinedHandler{notifications: notifications, logger: logger},
		inviteExpiredHandler:         &inviteExpiredHandler{notifications: notifications, logger: logger},
		sessionInviteSentHandler:     &sessionInviteSentHandler{notifications: notifications, logger: logger},
		sessionInviteAcceptedHandler: &sessionInviteAcceptedHandler{notifications: notifications, logger: logger},
		sessionInviteDeclinedHandler: &sessionInviteDeclinedHandler{notifications: notifications, logger: logger},
		sessionInviteExpiredHandler:  &sessionInviteExpiredHandler{notifications: notifications, logger: logger},
		draftStartedHandler:          &draftStartedHandler{notifications: notifications, logger: logger},
		draftTurnChangedHandler:      &draftTurnChangedHandler{notifications: notifications, logger: logger},
		draftPausedHandler:           &draftPausedHandler{notifications: notifications, logger: logger},
		draftCompletedHandler:        &draftCompletedHandler{notifications: notifications, logger: logger},
		draftCancelledHandler:        &draftCancelledHandler{notifications: notifications, logger: logger},
		votingOpenedHandler:          &votingOpenedHandler{notifications: notifications, logger: logger},
		votingClosedHandler:          &votingClosedHandler{notifications: notifications, logger: logger},
		votingCancelledHandler:       &votingCancelledHandler{notifications: notifications, logger: logger},
		votingTiedHandler:            &votingTiedHandler{notifications: notifications, logger: logger},
		teamsResetHandler:            &teamsResetHandler{notifications: notifications, logger: logger},
		attendeeTeamChangedHandler:   &attendeeTeamChangedHandler{notifications: notifications, logger: logger},
	}
}

func (s *Subscriber) Subscribe(ctx context.Context) error {
	subscriptions := []struct {
		topic   string
		handler events.MessageHandler
	}{
		{"identity.account.verified", s.identityNotifier.HandleAccountVerified},
		{"identity.account.locked", s.identityNotifier.HandleAccountLocked},
		{"identity.account.password.changed", s.identityNotifier.HandlePasswordChanged},
		{"identity.verification_code.created", s.identityNotifier.HandleVerificationCodeGenerated},
		{"activity.member.join_requested", s.memberJoinRequestedHandler.Handle},
		{"activity.member.approved", s.memberApprovedHandler.Handle},
		{"activity.member.rejected", s.memberRejectedHandler.Handle},
		{"activity.session.attendee.promoted", s.attendeePromotedHandler.Handle},
		{"activity.session.attendee.join_requested", s.attendeeJoinRequestedHandler.Handle},
		{"activity.session.attendee.join_approved", s.attendeeJoinApprovedHandler.Handle},
		{"activity.session.attendee.join_rejected", s.attendeeJoinRejectedHandler.Handle},
		{"activity.session.attendee.removed", s.attendeeRemovedHandler.Handle},
		{"activity.member.removed", s.memberRemovedHandler.Handle},
		{"activity.session.cancelled", s.sessionCancelledHandler.Handle},
		{"activity.invite.sent", s.inviteSentHandler.Handle},
		{"activity.invite.accepted", s.inviteAcceptedHandler.Handle},
		{"activity.invite.declined", s.inviteDeclinedHandler.Handle},
		{"activity.invite.expired", s.inviteExpiredHandler.Handle},
		{"activity.session_invite.sent", s.sessionInviteSentHandler.Handle},
		{"activity.session_invite.accepted", s.sessionInviteAcceptedHandler.Handle},
		{"activity.session_invite.declined", s.sessionInviteDeclinedHandler.Handle},
		{"activity.session_invite.expired", s.sessionInviteExpiredHandler.Handle},
		{"activity.session.draft.started", s.draftStartedHandler.Handle},
		{"activity.session.draft.turn_changed", s.draftTurnChangedHandler.Handle},
		{"activity.session.draft.paused", s.draftPausedHandler.Handle},
		{"activity.session.draft.completed", s.draftCompletedHandler.Handle},
		{"activity.session.draft.cancelled", s.draftCancelledHandler.Handle},
		{"activity.session.voting.opened", s.votingOpenedHandler.Handle},
		{"activity.session.voting.closed", s.votingClosedHandler.Handle},
		{"activity.session.voting.cancelled", s.votingCancelledHandler.Handle},
		{"activity.session.voting.tied", s.votingTiedHandler.Handle},
		{"activity.session.teams_reset", s.teamsResetHandler.Handle},
		{"activity.session.attendee.team_changed", s.attendeeTeamChangedHandler.Handle},
	}

	for _, sub := range subscriptions {
		if err := s.broker.Subscribe(ctx, sub.topic, sub.handler); err != nil {
			return err
		}
	}

	return nil
}
