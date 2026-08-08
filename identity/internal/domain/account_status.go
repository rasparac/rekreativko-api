package domain

type AccountStatus string

const (
	AccountStatusActive    AccountStatus = "active"
	AccountStatusPending   AccountStatus = "pending"
	AccountStatusSuspended AccountStatus = "suspended"
	AccountStatusDeleted   AccountStatus = "deleted"
)

func (as AccountStatus) IsValid() bool {
	switch as {
	case AccountStatusActive, AccountStatusPending, AccountStatusSuspended, AccountStatusDeleted:
		return true
	default:
		return false
	}
}

func (as AccountStatus) String() string {
	return string(as)
}

func (as AccountStatus) CanLogin() bool {
	return as == AccountStatusActive
}

func (as AccountStatus) CanBeActivated() bool {
	return as == AccountStatusPending
}

func (as AccountStatus) CanBeSuspended() bool {
	return as == AccountStatusActive
}

func (as AccountStatus) CanBeDeleted() bool {
	return as == AccountStatusActive || as == AccountStatusSuspended
}
