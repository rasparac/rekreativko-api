package domain

import (
	"time"

	"github.com/google/uuid"
)

type AcitivityStatistics struct {
	activityID     uuid.UUID
	totalConfirmed int            // confirmed members only
	totalPending   int            // awaiting approval
	totalLeft      int            // members who left the activity or removed
	totalRejected  int            // rejected join requests
	membersByRole  map[string]int // creator/admin/member count
	updatedAt      time.Time
}

func NewActivityStatistics(activityID uuid.UUID) *AcitivityStatistics {
	return &AcitivityStatistics{
		activityID:     activityID,
		totalConfirmed: 1, // Creator is automatically confirmed
		totalPending:   0,
		totalLeft:      0,
		totalRejected:  0,
		membersByRole: map[string]int{
			MemberRoleCreator.String(): 1,
			MemberRoleAdmin.String():   0, // Creator is an admin
			MemberRoleMember.String():  0,
		},
		updatedAt: time.Now().UTC(),
	}
}

func (as *AcitivityStatistics) ActivityID() uuid.UUID {
	return as.activityID
}

func (as *AcitivityStatistics) TotalConfirmed() int {
	return as.totalConfirmed
}

func (as *AcitivityStatistics) TotalPending() int {
	return as.totalPending
}

func (as *AcitivityStatistics) TotalLeft() int {
	return as.totalLeft
}

func (as *AcitivityStatistics) TotalRejected() int {
	return as.totalRejected
}

func (as *AcitivityStatistics) MembersByRole() map[string]int {
	return as.membersByRole
}

func (as *AcitivityStatistics) UpdatedAt() time.Time {
	return as.updatedAt
}

func (as *AcitivityStatistics) touch() {
	as.updatedAt = time.Now().UTC()
}

func (as *AcitivityStatistics) OnMemberJoined() {
	as.totalPending++
	as.touch()
}

func (as *AcitivityStatistics) OnMemberApproved() {
	as.totalPending--
	as.totalConfirmed++
	as.membersByRole[MemberRoleMember.String()]++
	as.touch()
}

func (as *AcitivityStatistics) OnMemberLeft(role MemberRole) {
	as.totalConfirmed--
	as.totalLeft++
	as.membersByRole[role.String()]--
	as.touch()
}

func (as *AcitivityStatistics) OnMemberRejected() {
	as.totalPending--
	as.totalRejected++
	as.touch()
}

func (as *AcitivityStatistics) OnMemberRoleChanged(oldRole, newRole MemberRole) {
	if oldRole == newRole {
		return
	}
	as.membersByRole[oldRole.String()]--
	as.membersByRole[newRole.String()]++
	as.touch()
}

func (as *AcitivityStatistics) OnMemberRemoved(role MemberRole) {
	as.totalConfirmed--
	as.totalLeft++
	as.membersByRole[role.String()]--
	as.touch()
}

func (as *AcitivityStatistics) OnMemberInviteAccepted() {
	as.totalPending--
	as.totalConfirmed++
	as.membersByRole[MemberRoleMember.String()]++
	as.touch()
}

func (as *AcitivityStatistics) OnMemberLinkUsed() {
	as.totalPending--
	as.totalConfirmed++
	as.membersByRole[MemberRoleMember.String()]++
	as.touch()
}

type SessionStatistics struct {
	sessionID     uuid.UUID
	activityID    uuid.UUID
	totalGoing    int // confirmed members only
	totalpending  int // waitlisted
	totalNotGoing int // members who declined
	totalMaybe    int // soft interest
	totalPromoted int // members who were promoted (from waitlist)
	updatedAt     time.Time
}

func NewSessionStatistics(
	sessionID uuid.UUID,
	activityID uuid.UUID,
	autoConfirmed int,
	autoPending int,
) *SessionStatistics {
	return &SessionStatistics{
		sessionID:     sessionID,
		activityID:    activityID,
		totalGoing:    autoConfirmed,
		totalpending:  autoPending,
		totalNotGoing: 0,
		totalMaybe:    0,
		updatedAt:     time.Now().UTC(),
	}
}

func (ss *SessionStatistics) SessionID() uuid.UUID {
	return ss.sessionID
}

func (ss *SessionStatistics) ActivityID() uuid.UUID {
	return ss.activityID
}

func (ss *SessionStatistics) TotalGoing() int {
	return ss.totalGoing
}

func (ss *SessionStatistics) TotalPending() int {
	return ss.totalpending
}

func (ss *SessionStatistics) TotalNotGoing() int {
	return ss.totalNotGoing
}

func (ss *SessionStatistics) TotalMaybe() int {
	return ss.totalMaybe
}

func (ss *SessionStatistics) UpdatedAt() time.Time {
	return ss.updatedAt
}

func (ss *SessionStatistics) OnAttendeeRSCPGoing() {
	ss.totalpending--
	ss.totalGoing++
	ss.touch()
}

func (ss *SessionStatistics) OnAttendeeRSVPPending() {
	ss.totalpending++
	ss.touch()
}

func (ss *SessionStatistics) OnAttendeeRSVPNotGoing(heldSpot bool) {
	if heldSpot {
		ss.totalGoing--
	}
	ss.totalNotGoing++
	ss.touch()
}

func (ss *SessionStatistics) OnAttendeeRSVPMaybe(heldSpot bool) {
	if heldSpot {
		ss.totalGoing--
	}
	ss.totalMaybe++
	ss.touch()
}

// OnAttendeePromoted updates stats when a pending attendeee gets a spot
func (ss *SessionStatistics) OnAttendeePromoted() {
	ss.totalpending--
	ss.totalGoing++
	ss.totalPromoted++
	ss.touch()
}

// OnAttendeeAutoConfirmed updates stats for azto.confirmed attendees
// Called when processing auot-attend list after session creation
func (ss *SessionStatistics) OnAttendeeAutoConfirmed() {
	ss.totalpending--
	ss.totalGoing++
	ss.touch()
}

func (ss *SessionStatistics) OnAttendeeAutoPending() {
	ss.totalpending++
	ss.touch()
}

func (ss *SessionStatistics) touch() {
	ss.updatedAt = time.Now().UTC()
}
