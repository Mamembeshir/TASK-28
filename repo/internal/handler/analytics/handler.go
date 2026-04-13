package analyticshandler

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/eduexchange/eduexchange/internal/audit"
	"github.com/eduexchange/eduexchange/internal/middleware"
	analyticsrepo "github.com/eduexchange/eduexchange/internal/repository/analytics"
	analyticsservice "github.com/eduexchange/eduexchange/internal/service/analytics"
	analyticspages "github.com/eduexchange/eduexchange/internal/templ/pages/analytics"
	adminpages "github.com/eduexchange/eduexchange/internal/templ/pages/admin"
)

// Handler serves all analytics routes.
type Handler struct {
	analyticsSvc *analyticsservice.AnalyticsService
	auditSvc     *audit.Service
}

// New constructs an analytics Handler.
func New(analyticsSvc *analyticsservice.AnalyticsService, auditSvc *audit.Service) *Handler {
	return &Handler{
		analyticsSvc: analyticsSvc,
		auditSvc:     auditSvc,
	}
}

// GetDashboard handles GET /analytics/dashboard
func (h *Handler) GetDashboard(c *gin.Context) {
	authUser := middleware.GetAuthUser(c)

	var roles []string
	if authUser != nil {
		roles = authUser.Roles
	}

	metrics, err := h.analyticsSvc.GetDashboard(c.Request.Context(), roles)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	isAdmin := containsRole(roles, "ADMIN")
	isReviewer := containsRole(roles, "REVIEWER")

	data := analyticspages.DashboardData{
		Metrics:    metrics,
		AuthUser:   authUser,
		IsAdmin:    isAdmin,
		IsReviewer: isReviewer,
	}

	username := ""
	if authUser != nil {
		username = authUser.Username
	}
	_ = username

	if c.GetHeader("Accept") == "application/json" {
		c.JSON(http.StatusOK, gin.H{"metrics": metrics})
		return
	}

	c.Header("Content-Type", "text/html")
	if err := analyticspages.DashboardPage(data).Render(c.Request.Context(), c.Writer); err != nil {
		c.Status(http.StatusInternalServerError)
	}
}

// PostGenerateReport handles POST /analytics/reports/generate (Admin only)
func (h *Handler) PostGenerateReport(c *gin.Context) {
	authUser := middleware.GetAuthUser(c)
	if authUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	reportType := c.PostForm("report_type")
	if reportType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "report_type is required"})
		return
	}

	params := map[string]string{}
	for key, vals := range c.Request.PostForm {
		if key != "report_type" && len(vals) > 0 {
			params[key] = vals[0]
		}
	}

	report, err := h.analyticsSvc.GenerateReport(c.Request.Context(), authUser.ID, reportType, params)
	if err != nil {
		log.Printf("analytics handler: GenerateReport error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":          report.ID.String(),
		"report_type": report.ReportType,
		"status":      report.Status.String(),
		"file_path":   report.FilePath,
	})
}

// GetReportDownload handles GET /analytics/reports/:id/download (Admin only)
func (h *Handler) GetReportDownload(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid report id"})
		return
	}

	report, err := h.analyticsSvc.GetReport(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "report not found"})
		return
	}

	c.File(report.FilePath)
}

// GetReportList handles GET /analytics/reports (Admin only)
func (h *Handler) GetReportList(c *gin.Context) {
	authUser := middleware.GetAuthUser(c)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	reports, total, err := h.analyticsSvc.ListReports(c.Request.Context(), page, pageSize)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	data := analyticspages.ReportListData{
		Reports:  reports,
		Total:    total,
		AuthUser: authUser,
	}

	c.Header("Content-Type", "text/html")
	if err := analyticspages.ReportListPage(data).Render(c.Request.Context(), c.Writer); err != nil {
		c.Status(http.StatusInternalServerError)
	}
}

// GetAuditLogs handles GET /admin/audit-logs (Admin only)
func (h *Handler) GetAuditLogs(c *gin.Context) {
	authUser := middleware.GetAuthUser(c)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))

	filter := analyticsrepo.AnalyticsFilter{
		EntityType: c.Query("entity_type"),
		Action:     c.Query("action"),
		Page:       page,
		PageSize:   pageSize,
	}

	if actorStr := c.Query("actor_id"); actorStr != "" {
		if id, err := uuid.Parse(actorStr); err == nil {
			filter.ActorID = &id
		}
	}
	if fromStr := c.Query("from"); fromStr != "" {
		if t, err := time.Parse("2006-01-02", fromStr); err == nil {
			filter.From = &t
		}
	}
	if toStr := c.Query("to"); toStr != "" {
		if t, err := time.Parse("2006-01-02", toStr); err == nil {
			end := t.Add(24*time.Hour - time.Second)
			filter.To = &end
		}
	}

	entries, total, err := h.analyticsSvc.GetAuditLogs(c.Request.Context(), filter)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	data := adminpages.AuditLogData{
		Entries:  entries,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
		AuthUser: authUser,
	}

	if c.GetHeader("Accept") == "application/json" {
		c.JSON(http.StatusOK, gin.H{"entries": entries, "total": total})
		return
	}

	c.Header("Content-Type", "text/html")
	if err := adminpages.AuditLogPage(data).Render(c.Request.Context(), c.Writer); err != nil {
		c.Status(http.StatusInternalServerError)
	}
}

// PostExportAuditLog handles POST /admin/audit-logs/export (Admin only)
func (h *Handler) PostExportAuditLog(c *gin.Context) {
	authUser := middleware.GetAuthUser(c)
	if authUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	filter := analyticsrepo.AnalyticsFilter{
		EntityType: c.PostForm("entity_type"),
		Action:     c.PostForm("action"),
		Page:       1,
		PageSize:   100000,
	}

	if actorStr := c.PostForm("actor_id"); actorStr != "" {
		if id, err := uuid.Parse(actorStr); err == nil {
			filter.ActorID = &id
		}
	}
	if fromStr := c.PostForm("from"); fromStr != "" {
		if t, err := time.Parse("2006-01-02", fromStr); err == nil {
			filter.From = &t
		}
	}
	if toStr := c.PostForm("to"); toStr != "" {
		if t, err := time.Parse("2006-01-02", toStr); err == nil {
			end := t.Add(24*time.Hour - time.Second)
			filter.To = &end
		}
	}

	filePath, err := h.analyticsSvc.ExportAuditLog(c.Request.Context(), authUser.ID, filter)
	if err != nil {
		log.Printf("analytics handler: ExportAuditLog error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"file_path": filePath})
}

// containsRole checks whether a role string exists in a slice.
func containsRole(roles []string, role string) bool {
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}
