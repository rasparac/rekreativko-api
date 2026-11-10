package domain

import (
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
)

// PickOrder decides which captain picks on each turn of a draft.
type PickOrder string

const (
	PickOrderSnake     PickOrder = "snake"     // A, B, B, A, A, B, ...
	PickOrderAlternate PickOrder = "alternate" // A, B, A, B, ...
)

func (o PickOrder) IsValid() bool {
	switch o {
	case PickOrderSnake, PickOrderAlternate:
		return true
	default:
		return false
	}
}

func (o PickOrder) String() string {
	return string(o)
}

// sideForTurn returns which side picks on the given 0-based turn. A pure
// function of the order, so more orders (or more captains) stay a local change.
func (o PickOrder) sideForTurn(turn int) DraftSide {
	side := DraftSide(turn % 2)
	if o == PickOrderSnake && (turn/2)%2 == 1 {
		return side.other()
	}
	return side
}

// DraftSide is a captain's side, and the position of the team it becomes:
// 0 = Team A (the first captain, who picks first), 1 = Team B.
type DraftSide int

const (
	DraftSideA DraftSide = 0
	DraftSideB DraftSide = 1
)

func (s DraftSide) IsValid() bool {
	return s == DraftSideA || s == DraftSideB
}

func (s DraftSide) other() DraftSide {
	return 1 - s
}

type DraftStatus string

const (
	DraftStatusActive    DraftStatus = "active"
	DraftStatusCompleted DraftStatus = "completed"
	DraftStatusCancelled DraftStatus = "cancelled"
)

type DraftCancelReason string

const (
	DraftCancelReasonOrganizer     DraftCancelReason = "organizer"      // creator/admin cancelled it
	DraftCancelReasonSessionEnded  DraftCancelReason = "session_ended"  // session was cancelled or completed mid-draft
	DraftCancelReasonRosterChanged DraftCancelReason = "roster_changed" // someone stopped going: team formation was reset
	DraftCancelReasonTeamsReset    DraftCancelReason = "teams_reset"    // an organizer reset team formation
)

// DraftPick is one player picked by a captain. PickNumber is the turn it was
// made on.
type DraftPick struct {
	pickNumber int
	side       DraftSide
	userID     uuid.UUID
	pickedAt   time.Time
}

func ReconstructDraftPick(pickNumber int, side DraftSide, userID uuid.UUID, pickedAt time.Time) DraftPick {
	return DraftPick{pickNumber: pickNumber, side: side, userID: userID, pickedAt: pickedAt}
}

func (p DraftPick) PickNumber() int     { return p.pickNumber }
func (p DraftPick) Side() DraftSide     { return p.side }
func (p DraftPick) UserID() uuid.UUID   { return p.userID }
func (p DraftPick) PickedAt() time.Time { return p.pickedAt }

// TeamDraft is a captain draft for one session: two captains take turns
// picking the people going until everyone is on a side. Picks live only in the
// draft - the session's teams are untouched until it completes
// (Session.ApplyDraftTeams), so cancelling never changes them. There is no
// turn timeout; the organizer cancels a stuck draft.
//
// Anyone who stops going resets team formation, which cancels the draft (see
// Reset and Session.ResetTeams).
//
// Every method that depends on who is going takes confirmed - the user IDs
// currently going/promoted - read by the caller under the session lock.
type TeamDraft struct {
	id              uuid.UUID
	sessionID       uuid.UUID
	activityGroupID *uuid.UUID
	pickOrder       PickOrder
	status          DraftStatus
	cancelReason    DraftCancelReason
	captains        [2]uuid.UUID // index = DraftSide
	teamConfig      TeamConfig   // always 2 teams; min players and colors as requested
	turn            int          // 0-based index of the next turn
	version         int          // bumped on every change, so clients can drop stale snapshots
	picks           []DraftPick
	startedBy       uuid.UUID
	startedAt       time.Time
	endedAt         *time.Time

	events []domainevent.Event
}

type TeamDraftInput struct {
	Session           *Session
	RequesterID       uuid.UUID
	RequesterRole     MemberRole
	CaptainIDs        [2]uuid.UUID // Team A's captain (picks first), Team B's captain
	PickOrder         PickOrder    // empty = snake
	MinPlayersPerTeam *int         // nil = no minimum (one per team)
	Colors            []string     // optional "#RRGGBB" for Team A, Team B
	Confirmed         []uuid.UUID  // people going right now
}

// NewTeamDraft starts a draft. It completes immediately when nobody besides the
// captains is going - callers check IsCompleted.
func NewTeamDraft(input TeamDraftInput) (*TeamDraft, error) {
	session := input.Session

	if !session.canManageSession(input.RequesterID, input.RequesterRole) {
		return nil, ErrUnauthorized
	}

	if err := session.requireTeamsChangeable(); err != nil {
		return nil, err
	}

	pickOrder := input.PickOrder
	if pickOrder == "" {
		pickOrder = PickOrderSnake
	}
	if !pickOrder.IsValid() {
		return nil, ErrInvalidPickOrder
	}

	teamCount := 2
	teamConfig, err := NewTeamConfig(&teamCount, input.MinPlayersPerTeam, input.Colors)
	if err != nil {
		return nil, err
	}

	if err := teamConfig.EnsureSupportedBy(session.ActivityType()); err != nil {
		return nil, err
	}

	captainA, captainB := input.CaptainIDs[0], input.CaptainIDs[1]
	if captainA == uuid.Nil || captainB == uuid.Nil || captainA == captainB {
		return nil, ErrInvalidDraftCaptains
	}

	if !slices.Contains(input.Confirmed, captainA) || !slices.Contains(input.Confirmed, captainB) {
		return nil, ErrDraftCaptainNotConfirmed
	}

	if !teamConfig.HasEnoughPlayers(len(input.Confirmed)) {
		return nil, ErrNotEnoughPlayers
	}

	now := time.Now().UTC()
	d := &TeamDraft{
		id:              uuid.New(),
		sessionID:       session.ID(),
		activityGroupID: session.ActivityGroupID(),
		pickOrder:       pickOrder,
		status:          DraftStatusActive,
		captains:        input.CaptainIDs,
		teamConfig:      teamConfig,
		startedBy:       input.RequesterID,
		startedAt:       now,
		version:         1,
	}

	d.addEvent(NewDraftStartedEvent(d))
	d.settle(input.Confirmed, now)

	return d, nil
}

// ReconstructTeamDraft rebuilds a draft from persisted data.
func ReconstructTeamDraft(
	id uuid.UUID,
	sessionID uuid.UUID,
	activityGroupID *uuid.UUID,
	pickOrder PickOrder,
	status DraftStatus,
	cancelReason DraftCancelReason,
	captains [2]uuid.UUID,
	teamConfig TeamConfig,
	turn int,
	version int,
	picks []DraftPick,
	startedBy uuid.UUID,
	startedAt time.Time,
	endedAt *time.Time,
) *TeamDraft {
	return &TeamDraft{
		id:              id,
		sessionID:       sessionID,
		activityGroupID: activityGroupID,
		pickOrder:       pickOrder,
		status:          status,
		cancelReason:    cancelReason,
		captains:        captains,
		teamConfig:      teamConfig,
		turn:            turn,
		version:         version,
		picks:           picks,
		startedBy:       startedBy,
		startedAt:       startedAt,
		endedAt:         endedAt,
	}
}

// Pick lets the captain whose turn it is take an available player.
func (d *TeamDraft) Pick(session *Session, captainID, userID uuid.UUID, confirmed []uuid.UUID) error {
	if err := d.requireActive(); err != nil {
		return err
	}

	if err := session.requireTeamsChangeable(); err != nil {
		return err
	}

	side, isCaptain := d.captainSide(captainID)
	if !isCaptain {
		return ErrNotDraftCaptain
	}

	if side != d.CurrentSide() {
		return ErrNotYourTurn
	}

	if !slices.Contains(d.Available(confirmed), userID) {
		return ErrPlayerNotAvailable
	}

	now := time.Now().UTC()
	pick := DraftPick{pickNumber: d.turn, side: side, userID: userID, pickedAt: now}
	d.picks = append(d.picks, pick)
	d.version++
	d.addEvent(NewDraftPlayerPickedEvent(d, pick, captainID))

	d.turn++
	d.settle(confirmed, now)

	return nil
}

// Cancel stops a running draft; the session's teams stay
// as they were. confirmed (the people going) is who gets told.
func (d *TeamDraft) Cancel(session *Session, requesterID uuid.UUID, requesterRole MemberRole, confirmed []uuid.UUID) error {
	if !session.canManageSession(requesterID, requesterRole) {
		return ErrUnauthorized
	}

	if !d.IsRunning() {
		return ErrDraftNotActive
	}

	d.version++
	d.cancel(DraftCancelReasonOrganizer, requesterID, confirmed, time.Now().UTC())

	return nil
}

// SessionEnded cancels a running draft because its session was cancelled or
// completed - nobody can pick any more, so it must not keep reporting a
// current captain. endedBy is uuid.Nil when the session was auto-completed.
func (d *TeamDraft) SessionEnded(endedBy uuid.UUID) {
	if !d.IsRunning() {
		return
	}

	d.version++
	d.cancel(DraftCancelReasonSessionEnded, endedBy, nil, time.Now().UTC())
}

// Reset cancels a running draft because team formation was reset (see
// Session.ResetTeams). reason is DraftCancelReasonRosterChanged or
// DraftCancelReasonTeamsReset; by is who caused it. Nobody is listed as a
// participant: the reset event tells everyone going.
func (d *TeamDraft) Reset(reason DraftCancelReason, by uuid.UUID) {
	if !d.IsRunning() {
		return
	}

	d.version++
	d.cancel(reason, by, nil, time.Now().UTC())
}

func (d *TeamDraft) cancel(reason DraftCancelReason, by uuid.UUID, participants []uuid.UUID, now time.Time) {
	d.status = DraftStatusCancelled
	d.cancelReason = reason
	d.endedAt = &now

	d.addEvent(NewDraftCancelledEvent(d, by, participants))
}

// settle completes the draft once nobody is left to pick; otherwise the next
// captain is told it's their turn.
func (d *TeamDraft) settle(confirmed []uuid.UUID, now time.Time) {
	if len(d.Available(confirmed)) == 0 {
		d.status = DraftStatusCompleted
		d.endedAt = &now
		d.addEvent(NewDraftCompletedEvent(d))
		return
	}

	d.addEvent(NewDraftTurnChangedEvent(d))
}

func (d *TeamDraft) requireActive() error {
	if d.status != DraftStatusActive {
		return ErrDraftNotActive
	}
	return nil
}

func (d *TeamDraft) captainSide(userID uuid.UUID) (DraftSide, bool) {
	switch userID {
	case d.captains[DraftSideA]:
		return DraftSideA, true
	case d.captains[DraftSideB]:
		return DraftSideB, true
	default:
		return 0, false
	}
}

// Available returns the people going nobody has picked yet (captains
// excluded), in the order given.
func (d *TeamDraft) Available(confirmed []uuid.UUID) []uuid.UUID {
	available := make([]uuid.UUID, 0, len(confirmed))
	for _, userID := range confirmed {
		if _, isCaptain := d.captainSide(userID); isCaptain {
			continue
		}
		if slices.ContainsFunc(d.picks, func(p DraftPick) bool { return p.userID == userID }) {
			continue
		}
		available = append(available, userID)
	}
	return available
}

// Roster returns a side's players: the captain first, then picks in order.
func (d *TeamDraft) Roster(side DraftSide) []uuid.UUID {
	roster := make([]uuid.UUID, 0, len(d.picks)+1)
	roster = append(roster, d.captains[side])
	for _, p := range d.picks {
		if p.side == side {
			roster = append(roster, p.userID)
		}
	}
	return roster
}

// TotalPicks is how many picks the draft will have in all: the turns taken so
// far plus one per person still available. It changes when people join or
// leave.
func (d *TeamDraft) TotalPicks(availableCount int) int {
	if !d.IsRunning() {
		return d.turn
	}
	return d.turn + availableCount
}

// TurnOrder is the side picking on every turn from the first to the last
// (TotalPicks long) - done, current (index Turn) and upcoming.
func (d *TeamDraft) TurnOrder(availableCount int) []DraftSide {
	total := d.TotalPicks(availableCount)
	order := make([]DraftSide, total)
	for turn := range order {
		order[turn] = d.pickOrder.sideForTurn(turn)
	}
	return order
}

// CurrentSide is the side whose turn it is (meaningful while running).
func (d *TeamDraft) CurrentSide() DraftSide {
	return d.pickOrder.sideForTurn(d.turn)
}

// CurrentCaptainID is the captain whose turn it is: nil once the draft ended.
func (d *TeamDraft) CurrentCaptainID() *uuid.UUID {
	if !d.IsRunning() {
		return nil
	}
	captain := d.captains[d.CurrentSide()]
	return &captain
}

// IsRunning reports whether the draft is still going - a running draft blocks
// manual team changes and a second draft.
func (d *TeamDraft) IsRunning() bool {
	return d.status == DraftStatusActive
}

func (d *TeamDraft) IsCompleted() bool { return d.status == DraftStatusCompleted }

func (d *TeamDraft) ID() uuid.UUID                    { return d.id }
func (d *TeamDraft) SessionID() uuid.UUID             { return d.sessionID }
func (d *TeamDraft) ActivityGroupID() *uuid.UUID      { return d.activityGroupID }
func (d *TeamDraft) PickOrder() PickOrder             { return d.pickOrder }
func (d *TeamDraft) Status() DraftStatus              { return d.status }
func (d *TeamDraft) CancelReason() DraftCancelReason  { return d.cancelReason }
func (d *TeamDraft) Captains() [2]uuid.UUID           { return d.captains }
func (d *TeamDraft) TeamConfig() TeamConfig           { return d.teamConfig }
func (d *TeamDraft) Turn() int                        { return d.turn }
func (d *TeamDraft) Version() int                     { return d.version }
func (d *TeamDraft) Picks() []DraftPick               { return d.picks }
func (d *TeamDraft) StartedBy() uuid.UUID             { return d.startedBy }
func (d *TeamDraft) StartedAt() time.Time             { return d.startedAt }
func (d *TeamDraft) EndedAt() *time.Time              { return d.endedAt }
func (d *TeamDraft) Events() []domainevent.Event      { return d.events }
func (d *TeamDraft) ClearEvents()                     { d.events = nil }
func (d *TeamDraft) addEvent(event domainevent.Event) { d.events = append(d.events, event) }
