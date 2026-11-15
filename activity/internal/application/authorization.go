package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
)

// requireCanManageGroup checks that requesterID is a confirmed member of
// activityGroupID with a role that can manage the group (creator or admin).
// Each failure case wraps its own specific sentinel error (domain.ErrMemberNotFound,
// domain.ErrNotConfirmed, domain.ErrInsufficientRole) together with domain.ErrUnauthorized,
// so callers can tell exactly which check failed via errors.Is on the specific
// sentinel, while error_mapper.go's errors.Is(err, domain.ErrUnauthorized) check
// still matches regardless of which one it was, mapping to the same HTTP response.
// visibleGroup returns the group if requesterID may see it, and
// domain.ErrActivityGroupNotFound if not: a private group doesn't exist for
// anyone but its creator and confirmed members - not just access-denied, so a
// stranger can't even confirm it exists by id. Its templates and members are
// hidden the same way.
func visibleGroup(
	ctx context.Context,
	groupRepo ActivityGroupRepository,
	memberRepo MemberRepository,
	activityGroupID uuid.UUID,
	requesterID uuid.UUID,
) (*domain.ActivityGroup, error) {
	group, err := groupRepo.GetActivityGroupByID(ctx, activityGroupID)
	if err != nil {
		return nil, err
	}

	if !group.IsVisibleTo(requesterID, func() bool { return isConfirmedMember(ctx, memberRepo, activityGroupID, requesterID) }) {
		return nil, domain.ErrActivityGroupNotFound
	}

	return group, nil
}

// isConfirmedMember answers loosely: any error (not-found or otherwise) is
// "not a confirmed member", since callers only need a yes/no visibility signal.
func isConfirmedMember(ctx context.Context, memberRepo MemberRepository, activityGroupID, userID uuid.UUID) bool {
	member, err := memberRepo.GetMemberByGroupAndUser(ctx, activityGroupID, userID)
	if err != nil || member == nil {
		return false
	}

	return member.Status() == domain.MemberStatusConfirmed
}

func requireCanManageGroup(
	ctx context.Context,
	memberRepo MemberRepository,
	activityGroupID uuid.UUID,
	requesterID uuid.UUID,
) error {
	member, err := memberRepo.GetMemberByGroupAndUser(ctx, activityGroupID, requesterID)
	switch {
	case err != nil:
		return fmt.Errorf("%w: %w", domain.ErrMemberNotFound, domain.ErrUnauthorized)
	case !member.IsConfirmed():
		return fmt.Errorf("%w: %w", domain.ErrNotConfirmed, domain.ErrUnauthorized)
	case !member.Role().CanManageMembers():
		return fmt.Errorf("role %q: %w: %w", member.Role(), domain.ErrInsufficientRole, domain.ErrUnauthorized)
	default:
		return nil
	}
}
