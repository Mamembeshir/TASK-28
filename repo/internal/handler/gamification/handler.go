package gamificationhandler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/eduexchange/eduexchange/internal/middleware"
	"github.com/eduexchange/eduexchange/internal/model"
	gamificationservice "github.com/eduexchange/eduexchange/internal/service/gamification"
	gamificationpages "github.com/eduexchange/eduexchange/internal/templ/pages/gamification"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Handler handles gamification HTTP endpoints.
type Handler struct {
	pointsSvc *gamificationservice.PointsService
}

func New(pointsSvc *gamificationservice.PointsService) *Handler {
	return &Handler{pointsSvc: pointsSvc}
}

// GetUserPoints handles GET /users/:id/points
func (h *Handler) GetUserPoints(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	up, err := h.pointsSvc.GetUserPoints(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed"})
		return
	}

	badges, err := h.pointsSvc.ListUserBadges(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed"})
		return
	}

	if c.GetHeader("HX-Request") == "true" || strings.Contains(c.GetHeader("Accept"), "text/html") {
		authUser := middleware.GetAuthUser(c)
		comp := gamificationpages.UserStatsPage(gamificationpages.UserStatsData{
			Points:   up,
			Badges:   badges,
			AuthUser: authUser,
		})
		c.Status(http.StatusOK)
		_ = comp.Render(c.Request.Context(), c.Writer)
		return
	}
	c.JSON(http.StatusOK, gin.H{"points": up, "badges": badges})
}

// GetUserBadges handles GET /users/:id/badges
func (h *Handler) GetUserBadges(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	badges, err := h.pointsSvc.ListUserBadges(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": badges})
}

// GetLeaderboard handles GET /leaderboard
func (h *Handler) GetLeaderboard(c *gin.Context) {
	limit := 20
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	entries, err := h.pointsSvc.GetLeaderboard(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed"})
		return
	}

	if c.GetHeader("HX-Request") == "true" || strings.Contains(c.GetHeader("Accept"), "text/html") {
		authUser := middleware.GetAuthUser(c)
		comp := gamificationpages.LeaderboardPage(gamificationpages.LeaderboardData{
			Entries:  entries,
			AuthUser: authUser,
		})
		c.Status(http.StatusOK)
		_ = comp.Render(c.Request.Context(), c.Writer)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": entries})
}

// GetPointRules handles GET /admin/point-rules
func (h *Handler) GetPointRules(c *gin.Context) {
	rules, err := h.pointsSvc.ListPointRules(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed"})
		return
	}

	if c.GetHeader("HX-Request") == "true" || strings.Contains(c.GetHeader("Accept"), "text/html") {
		authUser := middleware.GetAuthUser(c)
		comp := gamificationpages.PointRulesPage(gamificationpages.PointRulesData{
			Rules:    rules,
			AuthUser: authUser,
		})
		c.Status(http.StatusOK)
		_ = comp.Render(c.Request.Context(), c.Writer)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rules})
}

// PutPointRule handles PUT /admin/point-rules/:id
func (h *Handler) PutPointRule(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var body struct {
		Points      int    `json:"points"      form:"points"`
		Description string `json:"description" form:"description"`
		IsActive    bool   `json:"is_active"   form:"is_active"`
	}
	if err := c.ShouldBind(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}

	rule := &model.PointRule{
		ID:          id,
		Points:      body.Points,
		Description: body.Description,
		IsActive:    body.IsActive,
	}
	if err := h.pointsSvc.UpdatePointRule(c.Request.Context(), rule); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "updated"})
}
