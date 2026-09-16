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
