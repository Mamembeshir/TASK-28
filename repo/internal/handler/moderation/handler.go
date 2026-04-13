package moderationhandler

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/eduexchange/eduexchange/internal/middleware"
	"github.com/eduexchange/eduexchange/internal/model"
	moderationservice "github.com/eduexchange/eduexchange/internal/service/moderation"
	moderationpages "github.com/eduexchange/eduexchange/internal/templ/pages/moderation"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Handler handles moderation HTTP endpoints.
type Handler struct {
	svc *moderationservice.ModerationService
}

// New creates a new moderation Handler.
func New(svc *moderationservice.ModerationService) *Handler {
	return &Handler{svc: svc}
}

func respondError(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{"error": msg})
}

// internalError logs the real error server-side and returns a safe generic message to the client.
func internalError(c *gin.Context, err error) {
	log.Printf("moderation handler: internal error: %v", err)
	respondError(c, http.StatusInternalServerError, "internal server error")
}

// handleServiceError translates service-layer errors to appropriate HTTP responses.
func handleServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, model.ErrValidation):
		respondError(c, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, model.ErrForbidden):
		respondError(c, http.StatusForbidden, "forbidden")
	case errors.Is(err, model.ErrNotFound):
		respondError(c, http.StatusNotFound, "not found")
	case errors.Is(err, model.ErrConflict), errors.Is(err, model.ErrStaleVersion):
		respondError(c, http.StatusConflict, err.Error())
	default:
		internalError(c, err)
	}
}

func isHTMLRequest(c *gin.Context) bool {
	return c.GetHeader("HX-Request") == "true" || strings.Contains(c.GetHeader("Accept"), "text/html")
}

// ── Reports ───────────────────────────────────────────────────────────────────

// CreateReport handles POST /reports
func (h *Handler) CreateReport(c *gin.Context) {
	user := middleware.GetAuthUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	resourceIDStr := c.PostForm("resource_id")
	if resourceIDStr == "" {
		resourceIDStr = c.Param("resource_id")
	}
	reasonType := c.PostForm("reason_type")
	description := c.PostForm("description")

	resourceID, err := uuid.Parse(resourceIDStr)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid resource_id")
		return
	}

	report, err := h.svc.CreateReport(c.Request.Context(), user.ID, resourceID, reasonType, description)
	if err != nil {
		if err == model.ErrNotFound {
			respondError(c, http.StatusNotFound, "resource not found")
			return
		}
		internalError(c, err)
		return
	}

	c.JSON(http.StatusCreated, report)
}

// ListReports handles GET /moderation/reports
func (h *Handler) ListReports(c *gin.Context) {
	user := middleware.GetAuthUser(c)
	statusFilter := c.Query("status")
	page := 1
	pageSize := 20

	reports, _, err := h.svc.ListReports(c.Request.Context(), statusFilter, page, pageSize)
	if err != nil {
		internalError(c, err)
		return
	}

	if isHTMLRequest(c) {
		data := moderationpages.ReportQueueData{
			Reports:      reports,
			StatusFilter: statusFilter,
			AuthUser:     user,
		}
		c.Status(http.StatusOK)
		_ = moderationpages.ReportQueuePage(data).Render(c.Request.Context(), c.Writer)
		return
	}

	c.JSON(http.StatusOK, gin.H{"reports": reports})
}

// GetReportDetail handles GET /moderation/reports/:id
func (h *Handler) GetReportDetail(c *gin.Context) {
	user := middleware.GetAuthUser(c)
	reportID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid report id")
		return
	}

	report, err := h.svc.GetReport(c.Request.Context(), reportID)
	if err != nil {
		if err == model.ErrNotFound {
			respondError(c, http.StatusNotFound, "report not found")
			return
		}
		internalError(c, err)
		return
	}

	actions, _ := h.svc.ListModerationActions(c.Request.Context(), "RESOURCE", report.ResourceID)

	if isHTMLRequest(c) {
		data := moderationpages.ReportDetailData{
			Report:   report,
			Actions:  actions,
			AuthUser: user,
		}
		c.Status(http.StatusOK)
		_ = moderationpages.ReportDetailPage(data).Render(c.Request.Context(), c.Writer)
		return
	}

	c.JSON(http.StatusOK, gin.H{"report": report, "actions": actions})
}

// AssignReport handles POST /moderation/reports/:id/assign
func (h *Handler) AssignReport(c *gin.Context) {
	user := middleware.GetAuthUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	reportID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid report id")
		return
	}

	if err := h.svc.AssignReport(c.Request.Context(), reportID, user.ID); err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "assigned"})
}

// ResolveReport handles POST /moderation/reports/:id/resolve
func (h *Handler) ResolveReport(c *gin.Context) {
	user := middleware.GetAuthUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	reportID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid report id")
		return
	}

	actionType := c.PostForm("action_type")
	if actionType == "" {
		actionType = "APPROVE"
	}
	notes := c.PostForm("notes")
	evidence := c.PostForm("evidence")

	if err := h.svc.ResolveReport(c.Request.Context(), reportID, user.ID, actionType, notes, evidence); err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "resolved"})
}

// DismissReport handles POST /moderation/reports/:id/dismiss
func (h *Handler) DismissReport(c *gin.Context) {
	user := middleware.GetAuthUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	reportID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid report id")
		return
	}

	notes := c.PostForm("notes")

	if err := h.svc.DismissReport(c.Request.Context(), reportID, user.ID, notes); err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "dismissed"})
}

// ── Resources ─────────────────────────────────────────────────────────────────

// TakedownResource handles POST /moderation/resources/:id/takedown
func (h *Handler) TakedownResource(c *gin.Context) {
	user := middleware.GetAuthUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	resourceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid resource id")
		return
	}

	evidence := c.PostForm("evidence")

	if err := h.svc.TakedownResource(c.Request.Context(), resourceID, user.ID, evidence); err != nil {
		if err == model.ErrNotFound {
			respondError(c, http.StatusNotFound, "resource not found")
			return
		}
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "taken_down"})
}

// RestoreResource handles POST /moderation/resources/:id/restore
func (h *Handler) RestoreResource(c *gin.Context) {
	user := middleware.GetAuthUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	resourceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid resource id")
		return
	}

	if err := h.svc.RestoreResource(c.Request.Context(), resourceID, user.ID); err != nil {
		if err == model.ErrNotFound {
			respondError(c, http.StatusNotFound, "resource not found")
			return
		}
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "restored"})
}

// ── Users ─────────────────────────────────────────────────────────────────────

// BanUser handles POST /moderation/users/:id/ban
func (h *Handler) BanUser(c *gin.Context) {
	user := middleware.GetAuthUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	targetUserID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid user id")
		return
	}

	banType := c.PostForm("ban_type")
	reason := c.PostForm("reason")

	if err := h.svc.BanUser(c.Request.Context(), targetUserID, user.ID, banType, reason); err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "banned"})
}

// UnbanUser handles POST /moderation/users/:id/unban
func (h *Handler) UnbanUser(c *gin.Context) {
	user := middleware.GetAuthUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	targetUserID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid user id")
		return
	}

	if err := h.svc.UnbanUser(c.Request.Context(), targetUserID, user.ID); err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "unbanned"})
}

// ── Anomalies ─────────────────────────────────────────────────────────────────

// ListAnomalyFlags handles GET /moderation/anomalies
func (h *Handler) ListAnomalyFlags(c *gin.Context) {
	user := middleware.GetAuthUser(c)
	statusFilter := c.Query("status")

	flags, err := h.svc.ListAnomalyFlags(c.Request.Context(), statusFilter)
	if err != nil {
		internalError(c, err)
		return
	}

	if isHTMLRequest(c) {
		data := moderationpages.AnomalyListData{
			Flags:    flags,
			AuthUser: user,
		}
		c.Status(http.StatusOK)
		_ = moderationpages.AnomalyListPage(data).Render(c.Request.Context(), c.Writer)
		return
	}

	c.JSON(http.StatusOK, gin.H{"flags": flags})
}

// ReviewAnomaly handles POST /moderation/anomalies/:id/review
func (h *Handler) ReviewAnomaly(c *gin.Context) {
	user := middleware.GetAuthUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	flagID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid flag id")
		return
	}

	decision := c.PostForm("decision")

	if err := h.svc.ReviewAnomaly(c.Request.Context(), flagID, user.ID, decision); err != nil {
		if err == model.ErrNotFound {
			respondError(c, http.StatusNotFound, "anomaly flag not found")
			return
		}
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": decision})
}
