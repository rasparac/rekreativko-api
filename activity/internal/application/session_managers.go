package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/activity/internal/infrastructure/persistence"
)

// sessionManagerIDs returns who can manage the session: its creator and, for a
// grouped session, the group's confirmed admins and creator. Without
// duplicates or uuid.Nil.
func sessionManagerIDs(ctx context.Context, memberRepo MemberRepository, session *domain.Session) ([]uuid.UUID, error) {
	candidates := []uuid.UUID{session.CreatedByID()}

	if groupID := session.ActivityGroupID(); groupID != nil {
		confirmed := domain.MemberStatusConfirmed
		members, _, err := memberRepo.ListMembers(ctx, persistence.MemberFilter{
			ActivityGroupID: groupID,
			Status:          &confirmed,
		})
		if err != nil {
			return nil, fmt.Errorf("list confirmed members: %w", err)
		}
		for _, m := range members {
			if m.Role().CanManageMembers() {
				candidates = append(candidates, m.UserID())
			}
		}
	}

	seen := make(map[uuid.UUID]struct{}, len(candidates))
	managers := make([]uuid.UUID, 0, len(candidates))
	for _, id := range candidates {
		if id == uuid.Nil {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		managers = append(managers, id)
	}

	return managers, nil
}
