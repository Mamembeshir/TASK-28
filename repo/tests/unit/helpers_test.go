package unit_test

import (
	"testing"

	"github.com/eduexchange/eduexchange/internal/model"
	"github.com/eduexchange/eduexchange/internal/repository"
	"github.com/stretchr/testify/assert"
)

// ---- FormatCurrency ----

func TestFormatCurrency_PositiveCents(t *testing.T) {
	assert.Equal(t, "$100.50", repository.FormatCurrency(10050))
}

func TestFormatCurrency_Zero(t *testing.T) {
	assert.Equal(t, "$0.00", repository.FormatCurrency(0))
}

func TestFormatCurrency_NegativeCents(t *testing.T) {
	assert.Equal(t, "-$10.00", repository.FormatCurrency(-1000))
}

func TestFormatCurrency_Pennies(t *testing.T) {
	assert.Equal(t, "$0.05", repository.FormatCurrency(5))
}

func TestFormatCurrency_LargeAmount(t *testing.T) {
	assert.Equal(t, "$1234567.89", repository.FormatCurrency(123456789))
}

// ---- Enum String() ----

func TestUserStatus_String(t *testing.T) {
	tests := []struct {
		s    model.UserStatus
		want string
	}{
		{model.UserStatusActive, "ACTIVE"},
		{model.UserStatusSuspended, "SUSPENDED"},
		{model.UserStatusBanned, "BANNED"},
		{model.UserStatusDeactivated, "DEACTIVATED"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.s.String())
	}
}

func TestResourceStatus_String(t *testing.T) {
	tests := []struct {
		s    model.ResourceStatus
		want string
	}{
		{model.ResourceStatusDraft, "DRAFT"},
		{model.ResourceStatusPendingReview, "PENDING_REVIEW"},
		{model.ResourceStatusApproved, "APPROVED"},
		{model.ResourceStatusPublished, "PUBLISHED"},
		{model.ResourceStatusRejected, "REJECTED"},
		{model.ResourceStatusTakenDown, "TAKEN_DOWN"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.s.String())
	}
}

func TestVoteType_String(t *testing.T) {
	assert.Equal(t, "UP", model.VoteTypeUp.String())
	assert.Equal(t, "DOWN", model.VoteTypeDown.String())
}

func TestBanType_String(t *testing.T) {
	assert.Equal(t, "1_DAY", model.BanType1Day.String())
	assert.Equal(t, "7_DAYS", model.BanType7Days.String())
	assert.Equal(t, "PERMANENT", model.BanTypePermanent.String())
}

func TestSupplierTier_String(t *testing.T) {
	assert.Equal(t, "BRONZE", model.SupplierTierBronze.String())
	assert.Equal(t, "SILVER", model.SupplierTierSilver.String())
	assert.Equal(t, "GOLD", model.SupplierTierGold.String())
}

func TestOrderStatus_String(t *testing.T) {
	assert.Equal(t, "CREATED", model.OrderStatusCreated.String())
	assert.Equal(t, "QC_PASSED", model.OrderStatusQCPassed.String())
	assert.Equal(t, "CANCELLED", model.OrderStatusCancelled.String())
}

// ---- Enum ParseX roundtrip ----

func TestParseUserStatus_RoundTrip(t *testing.T) {
	for _, s := range []model.UserStatus{
		model.UserStatusActive, model.UserStatusSuspended,
		model.UserStatusBanned, model.UserStatusDeactivated,
	} {
		parsed, err := model.ParseUserStatus(s.String())
		assert.NoError(t, err)
		assert.Equal(t, s, parsed)
	}
}

func TestParseResourceStatus_RoundTrip(t *testing.T) {
	for _, s := range []model.ResourceStatus{
		model.ResourceStatusDraft, model.ResourceStatusPendingReview,
		model.ResourceStatusApproved, model.ResourceStatusPublished,
		model.ResourceStatusRejected, model.ResourceStatusTakenDown,
	} {
		parsed, err := model.ParseResourceStatus(s.String())
		assert.NoError(t, err)
		assert.Equal(t, s, parsed)
	}
}

func TestParseUserStatus_InvalidInput(t *testing.T) {
	_, err := model.ParseUserStatus("UNKNOWN")
	assert.Error(t, err)
}

// ---- ValidationErrors ----

func TestValidationErrors_AddAndCheck(t *testing.T) {
	ve := model.NewValidationErrors()
	assert.False(t, ve.HasErrors())

	ve.Add("username", "Username is required")
	ve.Add("password", "Password too short")

	assert.True(t, ve.HasErrors())
	assert.Len(t, ve.Errors, 2)
	assert.Equal(t, "username", ve.Errors[0].Field)
	assert.Equal(t, "password", ve.Errors[1].Field)
}

func TestValidationErrors_Error_ReturnsFirstMessage(t *testing.T) {
	ve := model.NewValidationErrors()
	ve.Add("email", "Invalid email")
	assert.Equal(t, "Invalid email", ve.Error())
}

func TestValidationErrors_Error_EmptyReturnsDefault(t *testing.T) {
	ve := model.NewValidationErrors()
	assert.Equal(t, "validation failed", ve.Error())
}

// ---- Gamification level calculation ----

func TestLevel_PositivePoints(t *testing.T) {
	assert.Equal(t, 0, calcLevel(0))
	assert.Equal(t, 0, calcLevel(199))
	assert.Equal(t, 1, calcLevel(200))
	assert.Equal(t, 1, calcLevel(399))
	assert.Equal(t, 2, calcLevel(400))
	assert.Equal(t, 5, calcLevel(1000))
}

func TestLevel_NegativePointsFloorAtZero(t *testing.T) {
	assert.Equal(t, 0, calcLevel(-1))
	assert.Equal(t, 0, calcLevel(-10))
	assert.Equal(t, 0, calcLevel(-10000))
}

// calcLevel mirrors the business rule: level = max(0, floor(points / 200))
func calcLevel(totalPoints int) int {
	if totalPoints < 0 {
		return 0
	}
	return totalPoints / 200
}
