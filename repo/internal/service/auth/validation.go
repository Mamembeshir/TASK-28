package authservice

import (
	"fmt"
	"regexp"
	"unicode"

	"github.com/eduexchange/eduexchange/internal/model"
)

var usernameRe = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

// ValidatePassword enforces AUTH-01: min 12, max 128 chars, 1 upper, 1 lower, 1 digit, 1 special.
func ValidatePassword(password string) *model.ValidationErrors {
	errs := model.NewValidationErrors()

	if len(password) < 12 {
		errs.Add("password", "Password must be at least 12 characters.")
		return errs // no point checking further
	}
	if len(password) > 128 {
		errs.Add("password", "Password must be at most 128 characters.")
		return errs
	}

	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, ch := range password {
		switch {
		case unicode.IsUpper(ch):
			hasUpper = true
		case unicode.IsLower(ch):
			hasLower = true
		case unicode.IsDigit(ch):
			hasDigit = true
		case isSpecial(ch):
			hasSpecial = true
		}
	}

	if !hasUpper {
		errs.Add("password", "Password must contain at least one uppercase letter.")
	}
	if !hasLower {
		errs.Add("password", "Password must contain at least one lowercase letter.")
	}
	if !hasDigit {
		errs.Add("password", "Password must contain at least one digit.")
	}
	if !hasSpecial {
		errs.Add("password", "Password must contain at least one special character (!@#$%^&* etc).")
	}

	return errs
}

func isSpecial(ch rune) bool {
	return !unicode.IsLetter(ch) && !unicode.IsDigit(ch) && !unicode.IsSpace(ch)
}

// ValidateUsername enforces: 3–100 chars, alphanumeric + . _ -
func ValidateUsername(username string) *model.ValidationErrors {
	errs := model.NewValidationErrors()
	if len(username) < 3 {
		errs.Add("username", "Username must be at least 3 characters.")
	} else if len(username) > 100 {
		errs.Add("username", "Username must be at most 100 characters.")
	} else if !usernameRe.MatchString(username) {
		errs.Add("username", "Username may only contain letters, numbers, dots, underscores, and hyphens.")
	}
	return errs
}

// ValidateEmail performs a basic email format check.
func ValidateEmail(email string) *model.ValidationErrors {
	errs := model.NewValidationErrors()
	re := regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	if !re.MatchString(email) {
		errs.Add("email", "Email address is invalid.")
	}
	return errs
}

// ValidStatusTransition returns nil when the transition is allowed.
// Enforces PRD section 9.1 (User Status state machine).
func ValidStatusTransition(from, to model.UserStatus) error {
	allowed := map[model.UserStatus][]model.UserStatus{
		model.UserStatusActive: {
			model.UserStatusSuspended,
			model.UserStatusBanned,
			model.UserStatusDeactivated,
		},
		model.UserStatusSuspended: {
			model.UserStatusActive,
			model.UserStatusDeactivated,
		},
		model.UserStatusBanned: {
			model.UserStatusActive,
		},
		model.UserStatusDeactivated: {}, // terminal — no outbound transitions
	}

	targets, ok := allowed[from]
	if !ok {
		return fmt.Errorf("unknown status: %s", from)
	}
	for _, t := range targets {
		if t == to {
			return nil
		}
	}
	return fmt.Errorf(
		"invalid status transition: %s → %s is not permitted",
		from, to,
	)
}
