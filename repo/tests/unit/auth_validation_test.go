package unit_test

import (
	"testing"

	"github.com/eduexchange/eduexchange/internal/model"
	authservice "github.com/eduexchange/eduexchange/internal/service/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── ValidatePassword (AUTH-01) ───────────────────────────────────────────────

func TestValidatePassword_TooShort(t *testing.T) {
	errs := authservice.ValidatePassword("Short1!")
	assert.True(t, errs.HasErrors())
	assert.Equal(t, "password", errs.Errors[0].Field)
}

func TestValidatePassword_TooLong(t *testing.T) {
	long := make([]byte, 129)
	for i := range long {
		long[i] = 'a'
	}
	errs := authservice.ValidatePassword(string(long))
	assert.True(t, errs.HasErrors())
}

func TestValidatePassword_MissingUpper(t *testing.T) {
	errs := authservice.ValidatePassword("nouppercase1!")
	assert.True(t, errs.HasErrors())
	found := false
	for _, e := range errs.Errors {
		if e.Field == "password" {
			found = true
		}
	}
	assert.True(t, found)
}

func TestValidatePassword_MissingLower(t *testing.T) {
	errs := authservice.ValidatePassword("NOLOWERCASE1!")
	assert.True(t, errs.HasErrors())
}

func TestValidatePassword_MissingDigit(t *testing.T) {
	errs := authservice.ValidatePassword("NoDigitHere!!")
	assert.True(t, errs.HasErrors())
}

func TestValidatePassword_MissingSpecial(t *testing.T) {
	errs := authservice.ValidatePassword("NoSpecialChar1")
	assert.True(t, errs.HasErrors())
}

func TestValidatePassword_Valid(t *testing.T) {
	errs := authservice.ValidatePassword("SecurePass1!")
	assert.False(t, errs.HasErrors())
}

func TestValidatePassword_ValidLong(t *testing.T) {
	// exactly 128 chars — should pass
	pw := "Aa1!" + string(make([]byte, 124))
	for i := 4; i < 128; i++ {
		pw = pw[:i] + "x" + pw[i+1:]
	}
	errs := authservice.ValidatePassword(pw)
	assert.False(t, errs.HasErrors())
}

// ─── ValidateUsername ────────────────────────────────────────────────────────

func TestValidateUsername_TooShort(t *testing.T) {
	errs := authservice.ValidateUsername("ab")
	assert.True(t, errs.HasErrors())
}

func TestValidateUsername_TooLong(t *testing.T) {
	long := make([]byte, 101)
	for i := range long {
		long[i] = 'a'
	}
	errs := authservice.ValidateUsername(string(long))
	assert.True(t, errs.HasErrors())
}

func TestValidateUsername_InvalidChars(t *testing.T) {
	errs := authservice.ValidateUsername("user name!")
	assert.True(t, errs.HasErrors())
}

func TestValidateUsername_Valid(t *testing.T) {
	for _, username := range []string{"abc", "user_1", "User.Name-2", "a23456789012345"} {
		errs := authservice.ValidateUsername(username)
		assert.False(t, errs.HasErrors(), "expected valid for %q", username)
	}
}

// ─── ValidateEmail ────────────────────────────────────────────────────────────

func TestValidateEmail_Invalid(t *testing.T) {
	for _, email := range []string{"notanemail", "missing@dot", "@nodomain.com", "no-at-sign"} {
		errs := authservice.ValidateEmail(email)
		assert.True(t, errs.HasErrors(), "expected invalid for %q", email)
	}
}

func TestValidateEmail_Valid(t *testing.T) {
	errs := authservice.ValidateEmail("user@example.com")
	assert.False(t, errs.HasErrors())
}

// ─── ValidStatusTransition (PRD 9.1) ────────────────────────────────────────

func TestValidStatusTransition_ActiveToSuspended(t *testing.T) {
	err := authservice.ValidStatusTransition(model.UserStatusActive, model.UserStatusSuspended)
	require.NoError(t, err)
}

func TestValidStatusTransition_ActiveToBanned(t *testing.T) {
	err := authservice.ValidStatusTransition(model.UserStatusActive, model.UserStatusBanned)
	require.NoError(t, err)
}

func TestValidStatusTransition_ActiveToDeactivated(t *testing.T) {
	err := authservice.ValidStatusTransition(model.UserStatusActive, model.UserStatusDeactivated)
	require.NoError(t, err)
}

func TestValidStatusTransition_SuspendedToActive(t *testing.T) {
	err := authservice.ValidStatusTransition(model.UserStatusSuspended, model.UserStatusActive)
	require.NoError(t, err)
}

func TestValidStatusTransition_SuspendedToDeactivated(t *testing.T) {
	err := authservice.ValidStatusTransition(model.UserStatusSuspended, model.UserStatusDeactivated)
	require.NoError(t, err)
}

func TestValidStatusTransition_BannedToActive(t *testing.T) {
	err := authservice.ValidStatusTransition(model.UserStatusBanned, model.UserStatusActive)
	require.NoError(t, err)
}

func TestValidStatusTransition_BannedToSuspended_Invalid(t *testing.T) {
	err := authservice.ValidStatusTransition(model.UserStatusBanned, model.UserStatusSuspended)
	assert.Error(t, err)
}

func TestValidStatusTransition_DeactivatedToAnything_Invalid(t *testing.T) {
	for _, to := range []model.UserStatus{model.UserStatusActive, model.UserStatusSuspended, model.UserStatusBanned} {
		err := authservice.ValidStatusTransition(model.UserStatusDeactivated, to)
		assert.Error(t, err, "expected error for DEACTIVATED → %s", to)
	}
}

func TestValidStatusTransition_ActiveToActive_Invalid(t *testing.T) {
	err := authservice.ValidStatusTransition(model.UserStatusActive, model.UserStatusActive)
	assert.Error(t, err)
}

func TestValidStatusTransition_SuspendedToBanned_Invalid(t *testing.T) {
	err := authservice.ValidStatusTransition(model.UserStatusSuspended, model.UserStatusBanned)
	assert.Error(t, err)
}
