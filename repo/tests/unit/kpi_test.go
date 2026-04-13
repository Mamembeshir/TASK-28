package unit_test

import (
	"testing"
	"time"

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

// TestGetTierBenefits_Gold — Gold → 48h window (universal SLA), escalate after 2 misses
func TestGetTierBenefits_Gold(t *testing.T) {
	benefits := supplierservice.GetTierBenefits(model.SupplierTierGold)
	assert.Equal(t, 48, benefits.ConfirmWindowHours)
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

// ── Stockout rate: computed by order lines, not by order count ─────────────────

func TestComputeStockoutRate_ByOrderLines(t *testing.T) {
	// One order with 3 lines, one cancelled order with 1 line → 1/4 = 25%
	orders := []model.SupplierOrder{
		{
			ID:         uuid.New(),
			SupplierID: uuid.New(),
			Status:     model.OrderStatusClosed,
			OrderLines: []model.OrderLine{
				{SKU: "A", Quantity: 1},
				{SKU: "B", Quantity: 1},
				{SKU: "C", Quantity: 1},
			},
		},
		{
			ID:         uuid.New(),
			SupplierID: uuid.New(),
			Status:     model.OrderStatusCancelled,
			OrderLines: []model.OrderLine{
				{SKU: "D", Quantity: 1},
			},
		},
	}
	rate := supplierservice.ComputeStockoutRate(orders)
	assert.InDelta(t, 25.0, rate, 0.01, "1 cancelled line out of 4 total = 25%%")
}

func TestComputeStockoutRate_NoLines_FallsBackToOrderCount(t *testing.T) {
	// Orders with no OrderLines — each counts as 1 line
	orders := []model.SupplierOrder{
		{ID: uuid.New(), Status: model.OrderStatusClosed},
		{ID: uuid.New(), Status: model.OrderStatusCancelled},
	}
	rate := supplierservice.ComputeStockoutRate(orders)
	assert.InDelta(t, 50.0, rate, 0.01, "1 cancelled out of 2 = 50%% when no line detail")
}

func TestComputeStockoutRate_NoOrders(t *testing.T) {
	assert.Equal(t, 0.0, supplierservice.ComputeStockoutRate(nil))
}

// ── Return rate: computed over 30-day window only ─────────────────────────────

func TestComputeReturnRate_ExcludesOldQC(t *testing.T) {
	now := time.Now().UTC()
	oldQC := now.Add(-31 * 24 * time.Hour) // 31 days ago — outside window
	recentQC := now.Add(-5 * 24 * time.Hour)

	orders := []model.SupplierOrder{
		{
			ID:     uuid.New(),
			Status: model.OrderStatusClosed,
			QCResult: &model.SupplierQCResult{
				ID:          uuid.New(),
				Result:      model.QCResultFail,
				SubmittedAt: oldQC, // should be excluded
			},
		},
		{
			ID:     uuid.New(),
			Status: model.OrderStatusQCPassed,
			QCResult: &model.SupplierQCResult{
				ID:          uuid.New(),
				Result:      model.QCResultPass,
				SubmittedAt: recentQC, // included
			},
		},
	}

	rate := supplierservice.ComputeReturnRate(orders, now)
	// Only the recent QC is in window, and it passed — return rate = 0%
	assert.Equal(t, 0.0, rate, "old QC failure must not count in 30-day window")
}

func TestComputeReturnRate_WithinWindow(t *testing.T) {
	now := time.Now().UTC()
	recent := now.Add(-10 * 24 * time.Hour)

	orders := []model.SupplierOrder{
		{
			ID: uuid.New(), Status: model.OrderStatusQCFailed,
			QCResult: &model.SupplierQCResult{ID: uuid.New(), Result: model.QCResultFail, SubmittedAt: recent},
		},
		{
			ID: uuid.New(), Status: model.OrderStatusQCPassed,
			QCResult: &model.SupplierQCResult{ID: uuid.New(), Result: model.QCResultPass, SubmittedAt: recent},
		},
	}

	rate := supplierservice.ComputeReturnRate(orders, now)
	assert.InDelta(t, 50.0, rate, 0.01, "1 fail out of 2 QC results = 50%%")
}

func TestComputeReturnRate_NoQC(t *testing.T) {
	assert.Equal(t, 0.0, supplierservice.ComputeReturnRate(nil, time.Now()))
}
