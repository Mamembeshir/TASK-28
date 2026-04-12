package analyticsrepo

import (
	"context"
	"time"

	"github.com/eduexchange/eduexchange/internal/model"
	"github.com/google/uuid"
)

// AnalyticsFilter holds filter criteria for audit log queries.
type AnalyticsFilter struct {
	ActorID    *uuid.UUID
	EntityType string
	Action     string
	From       *time.Time
	To         *time.Time
	Page       int
	PageSize   int
}

// AnalyticsRepository defines the data access interface for analytics.
type AnalyticsRepository interface {
	// AnalyticsSummary
	UpsertAnalyticsSummary(ctx context.Context, summary *model.AnalyticsSummary) error
	ListAnalyticsSummaries(ctx context.Context, metricType string) ([]model.AnalyticsSummary, error)
	DeleteOldSummaries(ctx context.Context, before time.Time) error

	// Computed queries for analytics
	GetUtilizationPeaks(ctx context.Context) ([]model.AnalyticsSummary, error)  // resource views by hour
	GetDemandHotspots(ctx context.Context) ([]model.AnalyticsSummary, error)    // top categories by resource count
	GetApprovalCycleTime(ctx context.Context) (float64, error)                  // avg hours PENDING_REVIEW→APPROVED
	GetViolationRate(ctx context.Context) (float64, error)                      // takedowns/published
	GetCancellationRate(ctx context.Context) (float64, error)                   // cancelled/total orders
	GetTotalCounts(ctx context.Context) (resources, users, orders int, err error)

	// ScheduledReport
	CreateScheduledReport(ctx context.Context, report *model.ScheduledReport) error
	GetScheduledReport(ctx context.Context, id uuid.UUID) (*model.ScheduledReport, error)
	ListScheduledReports(ctx context.Context, page, pageSize int) ([]model.ScheduledReport, int, error)
	UpdateScheduledReport(ctx context.Context, report *model.ScheduledReport) error

	// Audit log queries
	ListAuditLogs(ctx context.Context, filter AnalyticsFilter) ([]model.AuditLogEntry, int, error)
	ExportAuditLogs(ctx context.Context, filter AnalyticsFilter) ([]model.AuditLogEntry, error)
}
