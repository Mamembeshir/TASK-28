package frontend_test

import (
	"testing"
	"time"

	"github.com/eduexchange/eduexchange/internal/middleware"
	"github.com/eduexchange/eduexchange/internal/model"
	moderationpages "github.com/eduexchange/eduexchange/internal/templ/pages/moderation"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

var (
	testReportID   = uuid.New()
	testReporterID = uuid.New()
	testResourceID2 = uuid.New()
	testModeratorID = uuid.New()
	testFlagID     = uuid.New()

	sampleReport = model.Report{
		ID:            testReportID,
		ReporterID:    testReporterID,
		ResourceID:    testResourceID2,
		ReasonType:    "SPAM",
		Description:   "This resource is spam",
		Status:        "OPEN",
		Notes:         "",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		ResourceTitle: "Test Resource",
		ReporterName:  "reporter_user",
		ReviewerName:  "",
	}

	sampleAction = model.ModerationAction{
		ID:          uuid.New(),
		ModeratorID: testModeratorID,
		ActionType:  "TAKEDOWN",
		TargetType:  "RESOURCE",
		TargetID:    testResourceID2,
		Notes:       "Resource taken down for violations",
		EvidenceJSON: map[string]interface{}{
			"evidence": "User reported spam",
		},
		CreatedAt:     time.Now(),
		ModeratorName: "mod_user",
	}

	sampleFlag = model.AnomalyFlag{
		ID:       testFlagID,
		FlagType: "LIKE_RING",
		UserIDs:  []uuid.UUID{uuid.New(), uuid.New()},
		EvidenceJSON: map[string]interface{}{
			"mutual_votes_24h": 18,
		},
		Status:    "OPEN",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
)

// ── ReportQueuePage ───────────────────────────────────────────────────────────

func TestReportQueuePage_RendersReports(t *testing.T) {
	html := render(t, moderationpages.ReportQueuePage(moderationpages.ReportQueueData{
		Reports:      []model.Report{sampleReport},
		StatusFilter: "",
		AuthUser:     adminUser,
	}))

	assert.Contains(t, html, "Report Queue")
	assert.Contains(t, html, "Test Resource")
	assert.Contains(t, html, "reporter_user")
	assert.Contains(t, html, "SPAM")
	assert.Contains(t, html, "OPEN")
}

func TestReportQueuePage_FilterTabs(t *testing.T) {
	html := render(t, moderationpages.ReportQueuePage(moderationpages.ReportQueueData{
		Reports:      []model.Report{},
		StatusFilter: "OPEN",
		AuthUser:     adminUser,
	}))

	assert.Contains(t, html, "Open")
	assert.Contains(t, html, "Under Review")
	assert.Contains(t, html, "Resolved")
	assert.Contains(t, html, "Dismissed")
}

func TestReportQueuePage_EmptyState(t *testing.T) {
	html := render(t, moderationpages.ReportQueuePage(moderationpages.ReportQueueData{
		Reports:      []model.Report{},
		StatusFilter: "",
		AuthUser:     adminUser,
	}))

	assert.Contains(t, html, "No reports found")
}

// ── ReportDetailPage ──────────────────────────────────────────────────────────

func TestReportDetailPage_ShowsReportInfo(t *testing.T) {
	html := render(t, moderationpages.ReportDetailPage(moderationpages.ReportDetailData{
		Report:   &sampleReport,
		Actions:  []model.ModerationAction{sampleAction},
		AuthUser: adminUser,
	}))

	assert.Contains(t, html, "Report Detail")
	assert.Contains(t, html, "Test Resource")
	assert.Contains(t, html, "reporter_user")
	assert.Contains(t, html, "SPAM")
}

func TestReportDetailPage_ShowsEvidenceChain(t *testing.T) {
	html := render(t, moderationpages.ReportDetailPage(moderationpages.ReportDetailData{
		Report:   &sampleReport,
		Actions:  []model.ModerationAction{sampleAction},
		AuthUser: adminUser,
	}))

	assert.Contains(t, html, "Action History")
	assert.Contains(t, html, "TAKEDOWN")
	assert.Contains(t, html, "mod_user")
	assert.Contains(t, html, "Resource taken down for violations")
}

// ── AnomalyListPage ───────────────────────────────────────────────────────────

func TestAnomalyListPage_ShowsFlags(t *testing.T) {
	html := render(t, moderationpages.AnomalyListPage(moderationpages.AnomalyListData{
		Flags:    []model.AnomalyFlag{sampleFlag},
		AuthUser: adminUser,
	}))

	assert.Contains(t, html, "Anomaly Flags")
	assert.Contains(t, html, "LIKE_RING")
	assert.Contains(t, html, "OPEN")
}

func TestAnomalyListPage_EmptyState(t *testing.T) {
	html := render(t, moderationpages.AnomalyListPage(moderationpages.AnomalyListData{
		Flags:    []model.AnomalyFlag{},
		AuthUser: adminUser,
	}))

	assert.Contains(t, html, "No anomaly flags found")
}

// ── BanFormPage ───────────────────────────────────────────────────────────────

func TestBanFormPage_ShowsDurationOptions(t *testing.T) {
	html := render(t, moderationpages.BanFormPage(moderationpages.BanFormData{
		TargetUserID:   uuid.New(),
		TargetUsername: "target_user",
		AuthUser:       reviewerUser,
	}))

	assert.Contains(t, html, "Ban User")
	assert.Contains(t, html, "target_user")
	assert.Contains(t, html, "1 Day")
	assert.Contains(t, html, "7 Days")
}

func TestBanFormPage_PermanentOnlyForAdmin(t *testing.T) {
	// Admin sees permanent option
	adminHTML := render(t, moderationpages.BanFormPage(moderationpages.BanFormData{
		TargetUserID:   uuid.New(),
		TargetUsername: "target_user",
		AuthUser:       adminUser,
	}))
	assert.Contains(t, adminHTML, "Permanent")

	// Reviewer does NOT see permanent option
	reviewerHTML := render(t, moderationpages.BanFormPage(moderationpages.BanFormData{
		TargetUserID:   uuid.New(),
		TargetUsername: "target_user",
		AuthUser:       reviewerUser,
	}))
	assert.NotContains(t, reviewerHTML, "Permanent")
}

// ── EvidenceChain ─────────────────────────────────────────────────────────────

func TestEvidenceChain_RendersTimeline(t *testing.T) {
	action1 := model.ModerationAction{
		ID:            uuid.New(),
		ModeratorID:   testModeratorID,
		ActionType:    "APPROVE",
		TargetType:    "RESOURCE",
		TargetID:      testResourceID2,
		Notes:         "Approved after review",
		EvidenceJSON:  map[string]interface{}{},
		CreatedAt:     time.Now().Add(-2 * time.Hour),
		ModeratorName: "mod_one",
	}
	action2 := model.ModerationAction{
		ID:            uuid.New(),
		ModeratorID:   testModeratorID,
		ActionType:    "TAKEDOWN",
		TargetType:    "RESOURCE",
		TargetID:      testResourceID2,
		Notes:         "Taken down on second review",
		EvidenceJSON:  map[string]interface{}{"evidence": "spam"},
		CreatedAt:     time.Now(),
		ModeratorName: "mod_two",
	}

	html := render(t, moderationpages.EvidenceChain([]model.ModerationAction{action1, action2}))

	assert.Contains(t, html, "Action History")
	assert.Contains(t, html, "APPROVE")
	assert.Contains(t, html, "TAKEDOWN")
	assert.Contains(t, html, "mod_one")
	assert.Contains(t, html, "mod_two")
	assert.Contains(t, html, "Approved after review")
	assert.Contains(t, html, "Taken down on second review")
}

func TestEvidenceChain_EmptyState(t *testing.T) {
	html := render(t, moderationpages.EvidenceChain([]model.ModerationAction{}))

	assert.Contains(t, html, "No actions recorded yet")
}

// ── ReportRow ─────────────────────────────────────────────────────────────────

func TestReportRow_RendersAssignButton(t *testing.T) {
	html := render(t, moderationpages.ReportRow(sampleReport))

	assert.Contains(t, html, "Assign to me")
	assert.Contains(t, html, "View")
}

func TestReportRow_NoAssignForNonOpen(t *testing.T) {
	resolvedReport := sampleReport
	resolvedReport.Status = "RESOLVED"

	html := render(t, moderationpages.ReportRow(resolvedReport))

	assert.NotContains(t, html, "Assign to me")
	assert.Contains(t, html, "RESOLVED")
}

// Use existing auth user vars (adminUser, reviewerUser) from catalog_render_test.go
// but need a distinct middleware.AuthUser with REVIEWER role
var _ = &middleware.AuthUser{
	ID:       uuid.New(),
	Username: "test_reviewer",
	Roles:    []string{"REVIEWER"},
}
