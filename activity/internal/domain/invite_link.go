package domain

import (
	"crypto/rand"
	"encoding/base64"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
)

const (
	InviteLinkExpiryDuration = 7 * 24 * time.Hour // 7 days
	inviteLinkTokenBytes     = 32                 // 32 random bytes = 43 characters in base64 encoding
)

type InviteLinkStatus string

const (
	InviteLinkStatusActive  InviteLinkStatus = "active"
	InviteLinkStatusExpired InviteLinkStatus = "expired"
	InviteLinkStatusRevoked InviteLinkStatus = "revoked"
)

type InviteLinkUsageStatus string

const (
	InviteLinkUsageStatusConfirmed InviteLinkUsageStatus = "confirmed" // The user has confirmed, member created
	InviteLinkUsageStatusExpired   InviteLinkUsageStatus = "expired"   // The invite link has expired
	InviteLinkUsageStatusRevoked   InviteLinkUsageStatus = "revoked"   // The invite link has been revoked
	InviteLinkUsageStatusConsumed  InviteLinkUsageStatus = "consumed"  // The invite link has been used by the user
)

type InviteLinkUsage struct {
	id           uuid.UUID
	inviteLinkId uuid.UUID
	userID       uuid.UUID
	status       InviteLinkUsageStatus
	usedAt       time.Time
}

func newLinkUsage(
	inviteLinkId uuid.UUID,
	userID uuid.UUID,
	status InviteLinkUsageStatus,
) InviteLinkUsage {
	return InviteLinkUsage{
		id:           uuid.New(),
		inviteLinkId: inviteLinkId,
		userID:       userID,
		status:       status,
		usedAt:       time.Now().UTC(),
	}
}

type InviteLink struct {
	id        uuid.UUID
	groupID   uuid.UUID
	createdBy uuid.UUID
	token     string
	status    InviteLinkStatus
	createdAt time.Time
	expiresAt time.Time
	revokedAt *time.Time

	usages []InviteLinkUsage

	events []domainevent.Event
}

func NewInviteLink(
	groupID uuid.UUID,
	createdBy uuid.UUID,
	creatorRole MemberRole,
) (*InviteLink, error) {
	if !creatorRole.CanManageMembers() {
		return nil, ErrUnauthorized
	}

	token, err := generateSecureToken()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()

	link := &InviteLink{
		id:        uuid.New(),
		groupID:   groupID,
		createdBy: createdBy,
		token:     token,
		status:    InviteLinkStatusActive,
		createdAt: now,
		expiresAt: now.Add(InviteLinkExpiryDuration),
	}

	link.addEvent(
		NewInviteLinkCreatedEvent(link),
	)

	return link, nil

}

func (il *InviteLink) ID() uuid.UUID {
	return il.id
}

func (il *InviteLink) GroupID() uuid.UUID {
	return il.groupID
}

func (il *InviteLink) CreatedBy() uuid.UUID {
	return il.createdBy
}

func (il *InviteLink) Token() string {
	return il.token
}

func (il *InviteLink) Status() InviteLinkStatus {
	return il.status
}

func (il *InviteLink) CreatedAt() time.Time {
	return il.createdAt
}

func (il *InviteLink) ExpiresAt() time.Time {
	return il.expiresAt
}

func (il *InviteLink) RevokedAt() *time.Time {
	return il.revokedAt
}

func (il *InviteLink) Usages() []InviteLinkUsage {
	return il.usages
}

func (il *InviteLink) IsRevoked() bool {
	return il.status == InviteLinkStatusRevoked
}

func (il *InviteLink) IsExpired() bool {
	return time.Now().UTC().After(il.ExpiresAt())
}

// Use attempts to use the invite link for the given user ID.
// It returns an InviteLinkUsage indicating the result of the attempt.
// Auto-confirms user as member on success  - Member create at service layer
func (il *InviteLink) Use(userID uuid.UUID) (InviteLinkUsage, error) {
	if il.hasSuccessfulUsage() {
		usage := newLinkUsage(
			il.ID(),
			userID,
			InviteLinkUsageStatusConsumed,
		)
		il.usages = append(il.usages, usage)
		return usage, ErrInviteLinkAlreadyUsed
	}

	if il.IsRevoked() {
		usage := newLinkUsage(
			il.ID(),
			userID,
			InviteLinkUsageStatusRevoked,
		)
		il.usages = append(il.usages, usage)
		return usage, ErrInviteLinkRevoked
	}

	if il.IsExpired() {
		il.status = InviteLinkStatusExpired
		usage := newLinkUsage(
			il.ID(),
			userID,
			InviteLinkUsageStatusExpired,
		)
		il.usages = append(il.usages, usage)
		return usage, ErrInviteLinkExpired
	}

	usage := newLinkUsage(
		il.ID(),
		userID,
		InviteLinkUsageStatusConfirmed,
	)

	il.usages = append(il.usages, usage)

	il.addEvent(
		NewInviteLinkUsedEvent(il, userID),
	)

	return usage, nil
}

func (il *InviteLink) Revoke(
	requestedBy uuid.UUID,
	requesterRole MemberRole,
) error {
	if !requesterRole.CanManageMembers() {
		return ErrUnauthorized
	}

	if il.IsRevoked() {
		return ErrInviteLinkRevoked
	}

	if il.IsExpired() {
		return ErrInviteLinkExpired
	}

	now := time.Now().UTC()
	il.status = InviteLinkStatusRevoked
	il.revokedAt = &now

	il.addEvent(
		NewInviteLinkRevokedEvent(il, requestedBy),
	)

	return nil
}

func (il *InviteLink) Expire() error {
	if !il.IsExpired() {
		return ErrInviteLinkHasNotExpiredYet
	}

	il.status = InviteLinkStatusExpired

	il.addEvent(
		NewInviteLinkExpiredEvent(il),
	)

	return nil
}

func (il *InviteLink) IsActive() bool {
	return il.status == InviteLinkStatusActive
}

func (il *InviteLink) IsUsable() bool {
	return il.IsActive() && !il.IsExpired() && !il.hasSuccessfulUsage()
}

func (il *InviteLink) addEvent(event domainevent.Event) {
	il.events = append(il.events, event)
}

func (il *InviteLink) hasSuccessfulUsage() bool {
	for _, usage := range il.usages {
		if usage.status == InviteLinkUsageStatusConfirmed {
			return true
		}
	}
	return false
}

func generateSecureToken() (string, error) {
	bytes := make([]byte, inviteLinkTokenBytes)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(bytes), nil
}
