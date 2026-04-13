package supplierservice

import (
	"context"
	"log"
	"time"

	"github.com/eduexchange/eduexchange/internal/audit"
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
	repo     supplierrepo.SupplierRepository
	auditSvc *audit.Service
}

// NewKPIService creates a new KPIService.
func NewKPIService(repo supplierrepo.SupplierRepository, auditSvc *audit.Service) *KPIService {
	return &KPIService{repo: repo, auditSvc: auditSvc}
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
// All tiers share the universal 48h confirmation window (SLA requirement).
// Escalation threshold varies: Gold escalates after 2 misses, Silver after 1, Bronze immediately.
func GetTierBenefits(tier model.SupplierTier) TierBenefits {
	switch tier {
	case model.SupplierTierGold:
		return TierBenefits{ConfirmWindowHours: 48, EscalationThreshold: 2}
	case model.SupplierTierSilver:
		return TierBenefits{ConfirmWindowHours: 48, EscalationThreshold: 1}
	default: // Bronze
		return TierBenefits{ConfirmWindowHours: 48, EscalationThreshold: 0}
	}
}

// ComputeStockoutRate returns cancelled_lines / total_lines * 100.
// Stockout is measured by order lines, not by order count.
func ComputeStockoutRate(orders []model.SupplierOrder) float64 {
	totalLines, cancelledLines := 0, 0
	for _, o := range orders {
		lines := len(o.OrderLines)
		if lines == 0 {
			lines = 1 // treat order as one line when no detail stored
		}
		totalLines += lines
		if o.Status == model.OrderStatusCancelled {
			cancelledLines += lines
		}
	}
	if totalLines == 0 {
		return 0
	}
	return float64(cancelledLines) / float64(totalLines) * 100
}

// ComputeOnTimeDeliveryRate returns on-time shipments / eligible orders * 100.
func ComputeOnTimeDeliveryRate(orders []model.SupplierOrder) float64 {
	eligible, onTime := 0, 0
	for _, o := range orders {
		if o.Status == model.OrderStatusCancelled || o.DeliveryDateConfirmed == nil {
			continue
		}
		eligible++
		if o.ASN != nil && !o.ASN.ShippedAt.IsZero() && !o.ASN.ShippedAt.After(*o.DeliveryDateConfirmed) {
			onTime++
		}
	}
	if eligible == 0 {
		return 0
	}
	return float64(onTime) / float64(eligible) * 100
}

// ComputeReturnRate returns QC failures / QC completions within the last 30 days * 100.
func ComputeReturnRate(orders []model.SupplierOrder, now time.Time) float64 {
	thirtyDaysAgo := now.Add(-30 * 24 * time.Hour)
	completed, failed := 0, 0
	for _, o := range orders {
		if o.QCResult != nil && !o.QCResult.SubmittedAt.Before(thirtyDaysAgo) {
			completed++
			if o.QCResult.Result == model.QCResultFail {
				failed++
			}
		}
	}
	if completed == 0 {
		return 0
	}
	return float64(failed) / float64(completed) * 100
}

// ComputeDefectRate returns sum(defective_units) / sum(inspected_units) * 100.
func ComputeDefectRate(orders []model.SupplierOrder) float64 {
	totalInspected, totalDefective := 0, 0
	for _, o := range orders {
		if o.QCResult != nil {
			totalInspected += o.QCResult.InspectedUnits
			totalDefective += o.QCResult.DefectiveUnits
		}
	}
	if totalInspected == 0 {
		return 0
	}
	return float64(totalDefective) / float64(totalInspected) * 100
}

// RecalculateKPIs calculates KPIs for a supplier over the rolling 90-day window.
// actorID is the operator who triggered the recalculation (uuid.Nil for cron-initiated).
func (s *KPIService) RecalculateKPIs(ctx context.Context, actorID, supplierID uuid.UUID) (*model.SupplierKPI, error) {
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

	kpi.StockoutRatePct = ComputeStockoutRate(orders)
	kpi.OnTimeDeliveryPct = ComputeOnTimeDeliveryRate(orders)
	kpi.ReturnRatePct = ComputeReturnRate(orders, now)
	kpi.DefectRatePct = ComputeDefectRate(orders)

	kpi.TierAssigned = AssignTier(*kpi)

	if err := s.repo.SaveKPI(ctx, kpi); err != nil {
		return nil, err
	}

	// Update supplier tier
	if err := s.repo.UpdateSupplierTier(ctx, supplierID, kpi.TierAssigned); err != nil {
		log.Printf("kpi: failed to update supplier tier for %s: %v", supplierID, err)
	}

	if s.auditSvc != nil {
		_ = s.auditSvc.Record(ctx, audit.Entry{
			ActorID:    actorID,
			Action:     "supplier.kpi.recalculate",
			EntityType: "supplier",
			EntityID:   supplierID,
			AfterData: map[string]interface{}{
				"tier":              string(kpi.TierAssigned),
				"on_time_pct":       kpi.OnTimeDeliveryPct,
				"stockout_pct":      kpi.StockoutRatePct,
				"return_rate_pct":   kpi.ReturnRatePct,
				"defect_rate_pct":   kpi.DefectRatePct,
			},
			Source: "supplier",
			Reason: "kpi recalculation",
		})
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
		if _, err := s.RecalculateKPIs(ctx, uuid.Nil, supplier.ID); err != nil {
			log.Printf("kpi: failed to recalculate KPIs for supplier %s: %v", supplier.ID, err)
		}
	}
	return nil
}
