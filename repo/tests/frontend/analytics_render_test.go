package frontend_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/eduexchange/eduexchange/internal/model"
	adminpages "github.com/eduexchange/eduexchange/internal/templ/pages/admin"
	analyticspages "github.com/eduexchange/eduexchange/internal/templ/pages/analytics"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnalyticsDashboardPage_AdminView(t *testing.T) {
	adminUser := testAuthUser
	metrics := &model.DashboardMetrics{
		ApprovalCycleTime: 4.2,
		ViolationRate:     2.1,
		CancellationRate:  5.3,
		TotalResources:    42,
		TotalUsers:        100,
		TotalOrders:       15,
		RefreshedAt:       time.Now(),
		DemandHotspots: []model.AnalyticsSummary{
			{MetricLabel: "Mathematics", MetricValue: 15},
			{MetricLabel: "Science", MetricValue: 10},
		},
		UtilizationPeaks: []model.AnalyticsSummary{
			{MetricLabel: "Hour 9:00", MetricValue: 42},
			{MetricLabel: "Hour 14:00", MetricValue: 35},
		},
	}

	data := analyticspages.DashboardData{
		Metrics:    metrics,
		AuthUser:   adminUser,
		IsAdmin:    true,
		IsReviewer: false,
	}

	var buf bytes.Buffer
	err := analyticspages.DashboardPage(data).Render(context.Background(), &buf)
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "Analytics Dashboard")
	assert.Contains(t, html, "42")   // total resources
	assert.Contains(t, html, "100")  // total users
	assert.Contains(t, html, "Mathematics")
}

func TestAnalyticsDashboardPage_RegularUserView(t *testing.T) {
	metrics := &model.DashboardMetrics{
		TotalResources: 42,
		TotalUsers:     100,
		DemandHotspots: []model.AnalyticsSummary{
			{MetricLabel: "Art", MetricValue: 5},
		},
		RefreshedAt: time.Now(),
	}

	data := analyticspages.DashboardData{
		Metrics:    metrics,
		AuthUser:   testAuthUser,
		IsAdmin:    false,
		IsReviewer: false,
	}

	var buf bytes.Buffer
	err := analyticspages.DashboardPage(data).Render(context.Background(), &buf)
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "Analytics Dashboard")
	assert.Contains(t, html, "Art")
}

func TestReportListPage_EmptyState(t *testing.T) {
	data := analyticspages.ReportListData{
		Reports:  []model.ScheduledReport{},
		Total:    0,
		AuthUser: testAuthUser,
	}

	var buf bytes.Buffer
	err := analyticspages.ReportListPage(data).Render(context.Background(), &buf)
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "Generated Reports")
}

func TestReportListPage_WithReports(t *testing.T) {
	generatedAt := time.Now()
	reports := []model.ScheduledReport{
		{
			ID:          uuid.New(),
			ReportType:  "ANALYTICS",
			Status:      model.ScheduledReportStatusGenerated,
			FilePath:    "data/exports/reports/ANALYTICS_20260412.csv",
			GeneratedAt: &generatedAt,
			RequestedBy: uuid.New(),
			CreatedAt:   time.Now(),
		},
	}

	data := analyticspages.ReportListData{
		Reports:  reports,
		Total:    1,
		AuthUser: testAuthUser,
	}

	var buf bytes.Buffer
	err := analyticspages.ReportListPage(data).Render(context.Background(), &buf)
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "ANALYTICS")
	assert.Contains(t, html, "Download")
}

func TestAuditLogPage_Renders(t *testing.T) {
	entries := []model.AuditLogEntry{
		{
			ID:            uuid.New(),
			ActorID:       uuid.New(),
			ActorUsername: "admin",
			Action:        "resource.approve",
			EntityType:    "resource",
			EntityID:      uuid.New(),
			IPAddress:     "127.0.0.1",
			Source:        "web",
			Reason:        "Looks good",
			Timestamp:     time.Now(),
		},
	}

	data := adminpages.AuditLogData{
		Entries:  entries,
		Total:    1,
		Page:     1,
		PageSize: 25,
		AuthUser: testAuthUser,
	}

	var buf bytes.Buffer
	err := adminpages.AuditLogPage(data).Render(context.Background(), &buf)
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "Audit Log")
	assert.Contains(t, html, "resource.approve")
	assert.Contains(t, html, "admin")
	assert.Contains(t, html, "Export")
}
