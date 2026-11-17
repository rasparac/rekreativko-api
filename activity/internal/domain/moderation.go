package domain

import (
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// ReportTargetType is what a report is about.
type ReportTargetType string

const (
	ReportTargetUser    ReportTargetType = "user"
	ReportTargetGroup   ReportTargetType = "group"
	ReportTargetSession ReportTargetType = "session"
)

func (t ReportTargetType) IsValid() bool {
	return t == ReportTargetUser || t == ReportTargetGroup || t == ReportTargetSession
}

// ReportReason is why the reporter thinks the target breaks the rules.
type ReportReason string

const (
	ReportReasonSpam          ReportReason = "spam"
	ReportReasonHarassment    ReportReason = "harassment"
	ReportReasonHate          ReportReason = "hate"
	ReportReasonViolence      ReportReason = "violence"
	ReportReasonInappropriate ReportReason = "inappropriate"
	ReportReasonOther         ReportReason = "other"
)

// ReportReasons are the accepted reasons, in the order clients show them.
var ReportReasons = []ReportReason{
	ReportReasonSpam, ReportReasonHarassment, ReportReasonHate,
	ReportReasonViolence, ReportReasonInappropriate, ReportReasonOther,
}

func (r ReportReason) IsValid() bool {
	for _, valid := range ReportReasons {
		if r == valid {
			return true
		}
	}
	return false
}

// MaxReportNoteLength caps the optional note, in characters.
const MaxReportNoteLength = 1000

// Report is a user's report of a user, group or session, waiting for an
// operator's review. The note is free text the reporter typed: never log it.
type Report struct {
	id         uuid.UUID
	reporterID uuid.UUID
	targetType ReportTargetType
	targetID   uuid.UUID
	reason     ReportReason
	note       string
	createdAt  time.Time
}

func NewReport(reporterID uuid.UUID, targetType ReportTargetType, targetID uuid.UUID, reason ReportReason, note string) (*Report, error) {
	if !targetType.IsValid() || targetID == uuid.Nil {
		return nil, ErrInvalidReportTarget
	}

	if !reason.IsValid() {
		return nil, ErrInvalidReportReason
	}

	if utf8.RuneCountInString(note) > MaxReportNoteLength {
		return nil, ErrReportNoteTooLong
	}

	if targetType == ReportTargetUser && targetID == reporterID {
		return nil, ErrCannotReportYourself
	}

	return &Report{
		id:         uuid.New(),
		reporterID: reporterID,
		targetType: targetType,
		targetID:   targetID,
		reason:     reason,
		note:       note,
		createdAt:  time.Now().UTC(),
	}, nil
}

func (r *Report) ID() uuid.UUID                { return r.id }
func (r *Report) ReporterID() uuid.UUID        { return r.reporterID }
func (r *Report) TargetType() ReportTargetType { return r.targetType }
func (r *Report) TargetID() uuid.UUID          { return r.targetID }
func (r *Report) Reason() ReportReason         { return r.reason }
func (r *Report) Note() string                 { return r.note }
func (r *Report) CreatedAt() time.Time         { return r.createdAt }

// AccountBlock is blockerID having blocked blockedID: they can't invite each
// other, and blockedID's groups and sessions are hidden from blockerID's
// discover and browse lists.
type AccountBlock struct {
	blockerID uuid.UUID
	blockedID uuid.UUID
	createdAt time.Time
}

func NewAccountBlock(blockerID, blockedID uuid.UUID) (*AccountBlock, error) {
	if blockerID == blockedID {
		return nil, ErrCannotBlockYourself
	}

	return &AccountBlock{blockerID: blockerID, blockedID: blockedID, createdAt: time.Now().UTC()}, nil
}

func ReconstructAccountBlock(blockerID, blockedID uuid.UUID, createdAt time.Time) *AccountBlock {
	return &AccountBlock{blockerID: blockerID, blockedID: blockedID, createdAt: createdAt}
}

func (b *AccountBlock) BlockerID() uuid.UUID { return b.blockerID }
func (b *AccountBlock) BlockedID() uuid.UUID { return b.blockedID }
func (b *AccountBlock) CreatedAt() time.Time { return b.createdAt }
