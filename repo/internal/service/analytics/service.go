package analyticsservice

import (
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/eduexchange/eduexchange/internal/audit"
	"github.com/eduexchange/eduexchange/internal/model"
	analyticsrepo "github.com/eduexchange/eduexchange/internal/repository/analytics"
	"github.com/google/uuid"
)

// AnalyticsService handles analytics business logic.
type AnalyticsService struct {
	repo      analyticsrepo.AnalyticsRepository
	auditSvc  *audit.Service
	exportDir string // e.g., "data/exports/reports"
}

// NewAnalyticsService constructs an AnalyticsService.
func NewAnalyticsService(repo analyticsrepo.AnalyticsRepository, auditSvc *audit.Service, exportDir string) *AnalyticsService {
	return &AnalyticsService{
		repo:      repo,
		auditSvc:  auditSvc,
		exportDir: exportDir,
	}
}

// RefreshMetrics computes all analytics metrics and stores them in the analytics_summary table.
func (s *AnalyticsService) RefreshMetrics(ctx context.Context) error {
	now := time.Now()

	// Utilization peaks
	peaks, err := s.repo.GetUtilizationPeaks(ctx)
	if err != nil {
		log.Printf("analytics: GetUtilizationPeaks error: %v", err)
	} else {
		for i := range peaks {
			if upsertErr := s.repo.UpsertAnalyticsSummary(ctx, &peaks[i]); upsertErr != nil {
				log.Printf("analytics: UpsertAnalyticsSummary peaks error: %v", upsertErr)
			}
		}
	}

	// Demand hotspots
	hotspots, err := s.repo.GetDemandHotspots(ctx)
	if err != nil {
		log.Printf("analytics: GetDemandHotspots error: %v", err)
	} else {
		for i := range hotspots {
			if upsertErr := s.repo.UpsertAnalyticsSummary(ctx, &hotspots[i]); upsertErr != nil {
				log.Printf("analytics: UpsertAnalyticsSummary hotspots error: %v", upsertErr)
			}
		}
	}

	// Approval cycle time
	cycleTime, err := s.repo.GetApprovalCycleTime(ctx)
	if err != nil {
		log.Printf("analytics: GetApprovalCycleTime error: %v", err)
	} else {
		if upsertErr := s.repo.UpsertAnalyticsSummary(ctx, &model.AnalyticsSummary{
			ID:          uuid.New(),
			MetricType:  "APPROVAL_CYCLE_TIME",
			MetricKey:   "avg",
			MetricValue: cycleTime,
			MetricLabel: fmt.Sprintf("%.1f hours avg", cycleTime),
			ComputedAt:  now,
			PeriodStart: now.Add(-30 * 24 * time.Hour),
			PeriodEnd:   now,
		}); upsertErr != nil {
			log.Printf("analytics: UpsertAnalyticsSummary cycle time error: %v", upsertErr)
		}
	}

	// Violation rate
	violationRate, err := s.repo.GetViolationRate(ctx)
	if err != nil {
		log.Printf("analytics: GetViolationRate error: %v", err)
	} else {
		if upsertErr := s.repo.UpsertAnalyticsSummary(ctx, &model.AnalyticsSummary{
			ID:          uuid.New(),
			MetricType:  "VIOLATION_RATE",
			MetricKey:   "rate",
			MetricValue: violationRate,
			MetricLabel: fmt.Sprintf("%.1f%%", violationRate),
			ComputedAt:  now,
			PeriodStart: now.Add(-30 * 24 * time.Hour),
			PeriodEnd:   now,
		}); upsertErr != nil {
			log.Printf("analytics: UpsertAnalyticsSummary violation rate error: %v", upsertErr)
		}
	}

	// Cancellation rate
	cancelRate, err := s.repo.GetCancellationRate(ctx)
	if err != nil {
		log.Printf("analytics: GetCancellationRate error: %v", err)
	} else {
		if upsertErr := s.repo.UpsertAnalyticsSummary(ctx, &model.AnalyticsSummary{
			ID:          uuid.New(),
			MetricType:  "CANCELLATION_RATE",
			MetricKey:   "rate",
			MetricValue: cancelRate,
			MetricLabel: fmt.Sprintf("%.1f%%", cancelRate),
			ComputedAt:  now,
			PeriodStart: now.Add(-30 * 24 * time.Hour),
			PeriodEnd:   now,
		}); upsertErr != nil {
			log.Printf("analytics: UpsertAnalyticsSummary cancellation rate error: %v", upsertErr)
		}
	}

	return nil
}

// GetDashboard returns role-filtered metrics.
// Admin sees all. Reviewer sees moderation stats. Regular user sees basic counts and hotspots.
func (s *AnalyticsService) GetDashboard(ctx context.Context, roles []string) (*model.DashboardMetrics, error) {
	isAdmin := containsRole(roles, "ADMIN")
	isReviewer := containsRole(roles, "REVIEWER")
	isSupplier := containsRole(roles, "SUPPLIER")

	metrics := &model.DashboardMetrics{
		RefreshedAt: time.Now(),
	}

	resources, users, orders, err := s.repo.GetTotalCounts(ctx)
	if err != nil {
		log.Printf("analytics: GetTotalCounts error: %v", err)
	}
	metrics.TotalResources = resources
	metrics.TotalUsers = users
	metrics.TotalOrders = orders

	// Demand hotspots — visible to all roles
	hotspots, err := s.repo.GetDemandHotspots(ctx)
	if err != nil {
		log.Printf("analytics: GetDemandHotspots error: %v", err)
	} else {
		metrics.DemandHotspots = hotspots
	}

	if isAdmin || isReviewer {
		cycleTime, err := s.repo.GetApprovalCycleTime(ctx)
		if err != nil {
			log.Printf("analytics: GetApprovalCycleTime error: %v", err)
		} else {
			metrics.ApprovalCycleTime = cycleTime
		}

		violationRate, err := s.repo.GetViolationRate(ctx)
		if err != nil {
			log.Printf("analytics: GetViolationRate error: %v", err)
		} else {
			metrics.ViolationRate = violationRate
		}
	}

	if isAdmin {
		peaks, err := s.repo.GetUtilizationPeaks(ctx)
		if err != nil {
			log.Printf("analytics: GetUtilizationPeaks error: %v", err)
		} else {
			metrics.UtilizationPeaks = peaks
		}

		cancelRate, err := s.repo.GetCancellationRate(ctx)
		if err != nil {
			log.Printf("analytics: GetCancellationRate error: %v", err)
		} else {
			metrics.CancellationRate = cancelRate
		}
	}

	// Supplier: only show orders (already populated via GetTotalCounts)
	_ = isSupplier

	return metrics, nil
}

// GenerateReport creates a CSV report file and a ScheduledReport record.
func (s *AnalyticsService) GenerateReport(ctx context.Context, actorID uuid.UUID, reportType string, params map[string]string) (*model.ScheduledReport, error) {
	if err := os.MkdirAll(s.exportDir, 0755); err != nil {
		return nil, fmt.Errorf("analytics: create export dir: %w", err)
	}

	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("%s_%s.csv", reportType, timestamp)
	filePath := filepath.Join(s.exportDir, filename)

	f, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("analytics: create file: %w", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	switch reportType {
	case "ANALYTICS":
		if err := w.Write([]string{"MetricType", "MetricKey", "MetricValue", "MetricLabel", "ComputedAt"}); err != nil {
			return nil, err
		}
		for _, mt := range []string{"UTILIZATION_PEAKS", "DEMAND_HOTSPOTS", "APPROVAL_CYCLE_TIME", "VIOLATION_RATE", "CANCELLATION_RATE"} {
			summaries, err := s.repo.ListAnalyticsSummaries(ctx, mt)
			if err != nil {
				log.Printf("analytics: ListAnalyticsSummaries %s error: %v", mt, err)
				continue
			}
			for _, sum := range summaries {
				if err := w.Write([]string{
					sum.MetricType,
					sum.MetricKey,
					fmt.Sprintf("%.4f", sum.MetricValue),
					sum.MetricLabel,
					sum.ComputedAt.Format(time.RFC3339),
				}); err != nil {
					return nil, err
				}
			}
		}
	case "AUDIT_LOG":
		filter := analyticsrepo.AnalyticsFilter{Page: 1, PageSize: 10000}
		entries, _, err := s.repo.ListAuditLogs(ctx, filter)
		if err != nil {
			return nil, fmt.Errorf("analytics: list audit logs: %w", err)
		}
		if err := w.Write([]string{"ID", "ActorID", "Username", "Action", "EntityType", "EntityID", "IPAddress", "Source", "Reason", "Timestamp"}); err != nil {
			return nil, err
		}
		for _, e := range entries {
			if err := w.Write([]string{
				e.ID.String(), e.ActorID.String(), e.ActorUsername,
				e.Action, e.EntityType, e.EntityID.String(),
				e.IPAddress, e.Source, e.Reason,
				e.Timestamp.Format(time.RFC3339),
			}); err != nil {
				return nil, err
			}
		}
	case "SUPPLIER_KPI":
		if err := w.Write([]string{"Metric", "Value"}); err != nil {
			return nil, err
		}
		cancelRate, err := s.repo.GetCancellationRate(ctx)
		if err != nil {
			log.Printf("analytics: GetCancellationRate: %v", err)
		}
		_, _, orders, err := s.repo.GetTotalCounts(ctx)
		if err != nil {
			log.Printf("analytics: GetTotalCounts: %v", err)
		}
		_ = w.Write([]string{"CancellationRate", fmt.Sprintf("%.2f%%", cancelRate)})
		_ = w.Write([]string{"TotalOrders", fmt.Sprintf("%d", orders)})
	default:
		if err := w.Write([]string{"ReportType", "GeneratedAt"}); err != nil {
			return nil, err
		}
		_ = w.Write([]string{reportType, time.Now().Format(time.RFC3339)})
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("analytics: csv flush: %w", err)
	}

	now := time.Now()
	report := &model.ScheduledReport{
		ID:          uuid.New(),
		ReportType:  reportType,
		Parameters:  params,
		FilePath:    filePath,
		Status:      model.ScheduledReportStatusGenerated,
		GeneratedAt: &now,
		RequestedBy: actorID,
	}

	if err := s.repo.CreateScheduledReport(ctx, report); err != nil {
		return nil, fmt.Errorf("analytics: create scheduled report: %w", err)
	}

	_ = s.auditSvc.Record(ctx, audit.Entry{
		ActorID:    actorID,
		Action:     "REPORT_GENERATED",
		EntityType: "scheduled_report",
		EntityID:   report.ID,
		AfterData:  map[string]interface{}{"report_type": reportType, "file_path": filePath},
	})

	return report, nil
}

// GetReport returns a scheduled report by ID.
func (s *AnalyticsService) GetReport(ctx context.Context, id uuid.UUID) (*model.ScheduledReport, error) {
	return s.repo.GetScheduledReport(ctx, id)
}

// ListReports returns a paginated list of generated reports.
func (s *AnalyticsService) ListReports(ctx context.Context, page, pageSize int) ([]model.ScheduledReport, int, error) {
	return s.repo.ListScheduledReports(ctx, page, pageSize)
}

// ExportAuditLog generates a CSV export of audit logs and returns the file path.
func (s *AnalyticsService) ExportAuditLog(ctx context.Context, actorID uuid.UUID, filter analyticsrepo.AnalyticsFilter) (string, error) {
	entries, err := s.repo.ExportAuditLogs(ctx, filter)
	if err != nil {
		return "", fmt.Errorf("analytics: export audit logs: %w", err)
	}

	if err := os.MkdirAll(s.exportDir, 0755); err != nil {
		return "", fmt.Errorf("analytics: create export dir: %w", err)
	}

	timestamp := time.Now().Format("20060102_150405")
	filePath := filepath.Join(s.exportDir, fmt.Sprintf("audit_log_%s.csv", timestamp))

	f, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("analytics: create file: %w", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	if err := w.Write([]string{"ID", "ActorID", "Username", "Action", "EntityType", "EntityID", "IPAddress", "Source", "Reason", "Timestamp"}); err != nil {
		return "", err
	}

	for _, e := range entries {
		if err := w.Write([]string{
			e.ID.String(), e.ActorID.String(), e.ActorUsername,
			e.Action, e.EntityType, e.EntityID.String(),
			e.IPAddress, e.Source, e.Reason,
			e.Timestamp.Format(time.RFC3339),
		}); err != nil {
			return "", err
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return "", fmt.Errorf("analytics: csv flush: %w", err)
	}

	_ = s.auditSvc.Record(ctx, audit.Entry{
		ActorID:    actorID,
		Action:     "AUDIT_LOG_EXPORTED",
		EntityType: "audit_log",
		AfterData:  map[string]interface{}{"file_path": filePath, "count": len(entries)},
	})

	return filePath, nil
}

// GetAuditLogs returns paginated audit log entries.
func (s *AnalyticsService) GetAuditLogs(ctx context.Context, filter analyticsrepo.AnalyticsFilter) ([]model.AuditLogEntry, int, error) {
	return s.repo.ListAuditLogs(ctx, filter)
}

// containsRole checks if a role string is present in a slice of role strings.
func containsRole(roles []string, role string) bool {
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}
