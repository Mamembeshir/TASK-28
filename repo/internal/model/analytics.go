package model

import (
	"time"

	"github.com/google/uuid"
)

// AnalyticsSummary holds pre-computed analytics metrics refreshed on a schedule.
type AnalyticsSummary struct {
	ID          uuid.UUID
	MetricType  string    // e.g., "UTILIZATION_PEAKS", "DEMAND_HOTSPOTS", "APPROVAL_CYCLE_TIME", "VIOLATION_RATE", "CANCELLATION_RATE"
	MetricKey   string    // sub-key (e.g., hour of day for peaks, category name for hotspots)
	MetricValue float64   // the metric value
	MetricLabel string    // human-readable label
	ComputedAt  time.Time
	PeriodStart time.Time
	PeriodEnd   time.Time
}

// ScheduledReport tracks generated report files.
type ScheduledReport struct {
	ID          uuid.UUID
	ReportType  string            // "ANALYTICS", "AUDIT_LOG", "SUPPLIER_KPI"
	Parameters  map[string]string // JSON-stored parameters
	FilePath    string            // path to generated CSV
	Status      ScheduledReportStatus
	GeneratedAt *time.Time
	RequestedBy uuid.UUID
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// DashboardMetrics is a computed view of analytics for a given role.
type DashboardMetrics struct {
	UtilizationPeaks  []AnalyticsSummary // resources viewed per hour
	DemandHotspots    []AnalyticsSummary // most-requested categories
	ApprovalCycleTime float64            // avg hours from PENDING_REVIEW to APPROVED
	ViolationRate     float64            // takedowns / total published (percentage)
	CancellationRate  float64            // cancelled orders / total orders (percentage)
	TotalResources    int
	TotalUsers        int
	TotalOrders       int
	RefreshedAt       time.Time
}

// AuditLogEntry for display (from audit_logs table).
type AuditLogEntry struct {
	ID            uuid.UUID
	ActorID       uuid.UUID
	ActorUsername string
	Action        string
	EntityType    string
	EntityID      uuid.UUID
	BeforeData    map[string]interface{}
	AfterData     map[string]interface{}
	IPAddress     string
	Source        string
	Reason        string
	Timestamp     time.Time
}
