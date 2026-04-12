package unit_test

import (
	"testing"

	"github.com/eduexchange/eduexchange/internal/model"
	supplierservice "github.com/eduexchange/eduexchange/internal/service/supplier"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func makeKPI(otd, stockout, returnRate, defect float64) model.SupplierKPI {
	return model.SupplierKPI{
		ID:                uuid.New(),
		SupplierID:        uuid.New(),
		OnTimeDeliveryPct: otd,
		StockoutRatePct:   stockout,
		ReturnRatePct:     returnRate,
		DefectRatePct:     defect,
	}
}

// TestAssignTier_Gold — all 4 KPIs meet thresholds → Gold
func TestAssignTier_Gold(t *testing.T) {
	kpi := makeKPI(96.0, 1.5, 0.8, 0.4)
	tier := supplierservice.AssignTier(kpi)
	assert.Equal(t, model.SupplierTierGold, tier, "all 4 met => Gold")
}

// TestAssignTier_Silver — 3 of 4 meet → Silver
func TestAssignTier_Silver(t *testing.T) {
	// OTD below target, rest meet
	kpi := makeKPI(90.0, 1.5, 0.8, 0.4)
	tier := supplierservice.AssignTier(kpi)
	assert.Equal(t, model.SupplierTierSilver, tier, "3 of 4 met => Silver")
}

// TestAssignTier_Bronze — 2 of 4 meet → Bronze
func TestAssignTier_Bronze(t *testing.T) {
	// OTD and stockout below target
	kpi := makeKPI(90.0, 3.0, 0.8, 0.4)
	tier := supplierservice.AssignTier(kpi)
	assert.Equal(t, model.SupplierTierBronze, tier, "2 of 4 met => Bronze")
}

// TestAssignTier_NegativePcts — 0/4 meet → Bronze
func TestAssignTier_NegativePcts(t *testing.T) {
	kpi := makeKPI(80.0, 5.0, 3.0, 1.0)
	tier := supplierservice.AssignTier(kpi)
	assert.Equal(t, model.SupplierTierBronze, tier, "0 of 4 met => Bronze")
}

// TestAssignTier_ExactBoundary — exactly at thresholds → Gold
func TestAssignTier_ExactBoundary(t *testing.T) {
	kpi := makeKPI(95.0, 2.0, 1.0, 0.5)
	tier := supplierservice.AssignTier(kpi)
	assert.Equal(t, model.SupplierTierGold, tier, "exactly at thresholds => Gold")
}

// TestGetTierBenefits_Gold — Gold → 72h window
func TestGetTierBenefits_Gold(t *testing.T) {
	benefits := supplierservice.GetTierBenefits(model.SupplierTierGold)
	assert.Equal(t, 72, benefits.ConfirmWindowHours)
	assert.Equal(t, 2, benefits.EscalationThreshold)
}

// TestGetTierBenefits_Silver — Silver → 48h window
func TestGetTierBenefits_Silver(t *testing.T) {
	benefits := supplierservice.GetTierBenefits(model.SupplierTierSilver)
	assert.Equal(t, 48, benefits.ConfirmWindowHours)
	assert.Equal(t, 1, benefits.EscalationThreshold)
}

// TestGetTierBenefits_Bronze — Bronze → 48h window, 0 escalation threshold
func TestGetTierBenefits_Bronze(t *testing.T) {
	benefits := supplierservice.GetTierBenefits(model.SupplierTierBronze)
	assert.Equal(t, 48, benefits.ConfirmWindowHours)
	assert.Equal(t, 0, benefits.EscalationThreshold)
}
