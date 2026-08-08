package domain

import (
	"regexp"
	"strings"
)

type Email struct {
	value string
}

func NewEmail(value string) (*Email, error) {
	if value == "" {
		return nil, nil
	}

	email := strings.TrimSpace(strings.ToLower(value))

	if !isValidEmail(email) {
		return nil, ErrInvalidEmailFormat
	}

	return &Email{value: email}, nil

}

func (e *Email) String() string {
	if e == nil {
		return ""
	}
	return e.value
}

func (e *Email) IsEmpty() bool {
	return e == nil || e.value == ""
}

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

func isValidEmail(email string) bool {
	if len(email) < 3 || len(email) > 254 {
		return false
	}

	return emailRegex.MatchString(email)
}

type PhoneNumber struct {
	value string
}

func NewPhoneNumber(value string) (*PhoneNumber, error) {
	if value == "" {
		return nil, nil
	}

	phone := strings.TrimSpace(value)

	if !isValidPhoneNumber(phone) {
		return nil, ErrInvalidPhoneNumberFormat
	}

	return &PhoneNumber{value: phone}, nil
}

func (p *PhoneNumber) String() string {
	if p == nil {
		return ""
	}
	return p.value
}

func (p *PhoneNumber) IsEmpty() bool {
	return p == nil || p.value == ""
}

var phoneNumberRegex = regexp.MustCompile(`^\+?[0-9]{7,15}$`)

func isValidPhoneNumber(phone string) bool {
	return phoneNumberRegex.MatchString(phone)
}

type Password struct {
	value string
	hash  string
}

func NewPassword(value string) (*Password, error) {
	err := ValidatePassword(value)
	if err != nil {
		return nil, err
	}

	return &Password{}, nil
}

func NewPasswordFromHash(hash string) *Password {
	return &Password{hash: hash}
}

func (p *Password) String() string {
	return p.hash
}

func ValidatePassword(password string) error {
	if len(password) < 8 {
		return &PasswordRequirementError{
			Requirements: []string{"at least 8 characters long"},
		}
	}

	var (
		hasUpper bool
		hasLower bool
		hasDigit bool
		hasSpec  bool
	)

	for _, c := range password {
		switch {
		case 'A' <= c && c <= 'Z':
			hasUpper = true
		case 'a' <= c && c <= 'z':
			hasLower = true
		case '0' <= c && c <= '9':
			hasDigit = true
		case strings.ContainsRune("!@#$%^&*()-_=+[]{}|;:',.<>?/", c):
			hasSpec = true
		}
	}

	var requirements []string

	if !hasUpper {
		requirements = append(requirements, "at least one uppercase letter")
	}
	if !hasLower {
		requirements = append(requirements, "at least one lowercase letter")
	}
	if !hasDigit {
		requirements = append(requirements, "at least one digit")
	}
	if !hasSpec {
		requirements = append(requirements, "at least one special character")
	}

	if len(requirements) > 0 {
		return &PasswordRequirementError{
			Requirements: requirements,
		}
	}

	return nil
}
