package dtos

import (
	"time"

	"github.com/google/uuid"
)

// InviteMemberRequest is a request to invite a user to an activity group
type InviteMemberRequest struct {
	UserID uuid.UUID `json:"user_id" validate:"required" example:"123e4567-e89b-12d3-a456-426655440000"`
}

// UpdateMemberRoleRequest is a request to update a member's role
type UpdateMemberRoleRequest struct {
	Role string `json:"role" validate:"required,oneof=admin member" example:"admin"`
}

// MemberResponse is a response containing member data
type MemberResponse struct {
	ID              uuid.UUID  `json:"id" example:"123e4567-e89b-12d3-a456-426655440000"`
	ActivityGroupID uuid.UUID  `json:"activity_group_id" example:"123e4567-e89b-12d3-a456-426655440000"`
	UserID          uuid.UUID  `json:"user_id" example:"123e4567-e89b-12d3-a456-426655440000"`
	Role            string     `json:"role" example:"member"`
	Status          string     `json:"status" example:"confirmed"`
	JoinedAt        time.Time  `json:"joined_at" example:"2024-01-01T00:00:00Z"`
	DecidedAt       *time.Time `json:"decided_at,omitempty" example:"2024-01-01T00:00:00Z"`
}

// InviteMemberResponse is a response after inviting a member
type InviteMemberResponse struct {
	ID uuid.UUID `json:"id" example:"123e4567-e89b-12d3-a456-426655440000"`
}

// RequestToJoinGroupResponse is a response after requesting to join an activity group
type RequestToJoinGroupResponse struct {
	ID uuid.UUID `json:"id" example:"123e4567-e89b-12d3-a456-426655440000"`
}
