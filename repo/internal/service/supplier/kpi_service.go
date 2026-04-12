package supplierservice

import (
	"context"
	"log"
	"time"

	"github.com/eduexchange/eduexchange/internal/model"
	supplierrepo "github.com/eduexchange/eduexchange/internal/repository/supplier"
	"github.com/google/uuid"
)

// TierBenefits describes the confirmation window and escalation threshold for a tier.
type TierBenefits struct {
	ConfirmWindowHours  int
	EscalationThreshold int // number of misses before escalation; 0 = immediate
}

// KPIService handles KPI calculation and tier assignment.
type KPIService struct {
	repo supplierrepo.SupplierRepository
}

// NewKPIService creates a new KPIService.
func NewKPIService(repo supplierrepo.SupplierRepository) *KPIService {
	return &KPIService{repo: repo}
}

// AssignTier determines supplier tier based on KPI metrics.
// Pure function - testable without any dependencies.
// Gold = all 4 KPIs meet targets; Silver = 3 of 4; Bronze = < 3
func AssignTier(kpi model.SupplierKPI) model.SupplierTier {
	met := 0
	if kpi.OnTimeDeliveryPct >= 95 {
		met++
	}
	if kpi.StockoutRatePct <= 2 {
		met++
	}
	if kpi.ReturnRatePct <= 1 {
		met++
	}
	if kpi.DefectRatePct <= 0.5 {
		met++
	}

	switch {
	case met == 4:
		return model.SupplierTierGold
	case met == 3:
		return model.SupplierTierSilver
	default:
		return model.SupplierTierBronze
	}
}

// GetTierBenefits returns the tier-specific benefits.
// Gold: 72h confirmation window, escalate after 2 misses
// Silver: 48h, escalate after 1 miss
// Bronze: 48h, immediate escalation
func GetTierBenefits(tier model.SupplierTier) TierBenefits {
	switch tier {
	case model.SupplierTierGold:
		return TierBenefits{ConfirmWindowHours: 72, EscalationThreshold: 2}
	case model.SupplierTierSilver:
		return TierBenefits{ConfirmWindowHours: 48, EscalationThreshold: 1}
	default: // Bronze
		return TierBenefits{ConfirmWindowHours: 48, EscalationThreshold: 0}
	}
}

// RecalculateKPIs calculates KPIs for a supplier over the rolling 90-day window.
func (s *KPIService) RecalculateKPIs(ctx context.Context, supplierID uuid.UUID) (*model.SupplierKPI, error) {
	now := time.Now().UTC()
	periodStart := now.Add(-90 * 24 * time.Hour)

	orders, err := s.repo.GetOrdersInPeriod(ctx, supplierID, periodStart, now)
	if err != nil {
		return nil, err
	}

	kpi := &model.SupplierKPI{
		ID:          uuid.New(),
		SupplierID:  supplierID,
		PeriodStart: periodStart,
		PeriodEnd:   now,
		ComputedAt:  now,
	}

	total := len(orders)
	if total == 0 {
		kpi.TierAssigned = model.SupplierTierBronze
		if err := s.repo.SaveKPI(ctx, kpi); err != nil {
			return nil, err
		}
		return kpi, nil
	}

	// Stockout: cancelled / total
	cancelled := 0
	for _, o := range orders {
		if o.Status == model.OrderStatusCancelled {
			cancelled++
		}
	}
	kpi.StockoutRatePct = float64(cancelled) / float64(total) * 100

	// OTD: orders with confirmed delivery date and ASN where shipped_at <= delivery_date_confirmed
	// Only count non-cancelled orders with delivery_date_confirmed set
	otdEligible := 0
	otdOnTime := 0
	for _, o := range orders {
		if o.Status == model.OrderStatusCancelled {
			continue
		}
		if o.DeliveryDateConfirmed == nil {
			continue
		}
		otdEligible++
		if o.ASN != nil && !o.ASN.ShippedAt.IsZero() {
			if !o.ASN.ShippedAt.After(*o.DeliveryDateConfirmed) {
				otdOnTime++
			}
		}
	}
	if otdEligible > 0 {
		kpi.OnTimeDeliveryPct = float64(otdOnTime) / float64(otdEligible) * 100
	}

	// Return rate: QC_FAILED / qc-completed
	qcCompleted := 0
	qcFailed := 0
	for _, o := range orders {
		if o.Status == model.OrderStatusQCPassed || o.Status == model.OrderStatusQCFailed ||
			o.Status == model.OrderStatusClosed {
			qcCompleted++
			if o.Status == model.OrderStatusQCFailed {
				qcFailed++
			}
			// Also check the QC result for closed orders
			if o.Status == model.OrderStatusClosed && o.QCResult != nil {
				if o.QCResult.Result == model.QCResultFail {
					qcFailed++
					qcFailed-- // already counted above if status-based
				}
			}
		}
	}
	// More accurate: use QC result
	qcCompleted = 0
	qcFailed = 0
	totalInspected := 0
	totalDefective := 0
	for _, o := range orders {
		if o.QCResult != nil {
			qcCompleted++
			if o.QCResult.Result == model.QCResultFail {
				qcFailed++
			}
			totalInspected += o.QCResult.InspectedUnits
			totalDefective += o.QCResult.DefectiveUnits
		}
	}
	if qcCompleted > 0 {
		kpi.ReturnRatePct = float64(qcFailed) / float64(qcCompleted) * 100
	}

	// Defect rate: sum(defective) / sum(inspected) * 100
	if totalInspected > 0 {
		kpi.DefectRatePct = float64(totalDefective) / float64(totalInspected) * 100
	}

	kpi.TierAssigned = AssignTier(*kpi)

	if err := s.repo.SaveKPI(ctx, kpi); err != nil {
		return nil, err
	}

	// Update supplier tier
	if err := s.repo.UpdateSupplierTier(ctx, supplierID, kpi.TierAssigned); err != nil {
		log.Printf("kpi: failed to update supplier tier for %s: %v", supplierID, err)
	}

	return kpi, nil
}

// RecalculateAllKPIs recalculates KPIs for all active suppliers.
func (s *KPIService) RecalculateAllKPIs(ctx context.Context) error {
	suppliers, err := s.repo.GetAllActiveSuppliers(ctx)
	if err != nil {
		return err
	}

	for _, supplier := range suppliers {
		if _, err := s.RecalculateKPIs(ctx, supplier.ID); err != nil {
			log.Printf("kpi: failed to recalculate KPIs for supplier %s: %v", supplier.ID, err)
		}
	}
	return nil
}
