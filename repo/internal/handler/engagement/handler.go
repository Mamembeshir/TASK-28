package engagementhandler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/eduexchange/eduexchange/internal/middleware"
	"github.com/eduexchange/eduexchange/internal/model"
	engagementservice "github.com/eduexchange/eduexchange/internal/service/engagement"
	engagementpages "github.com/eduexchange/eduexchange/internal/templ/pages/engagement"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Handler handles engagement HTTP endpoints.
type Handler struct {
	svc *engagementservice.EngagementService
}

func New(svc *engagementservice.EngagementService) *Handler {
	return &Handler{svc: svc}
}

// ── Votes ─────────────────────────────────────────────────────────────────────

// PostVote handles POST /resources/:id/vote
// Body: {"vote_type": "UP"|"DOWN"}
func (h *Handler) PostVote(c *gin.Context) {
	actor := middleware.GetAuthUser(c)
	resourceID, err := parseUUID(c, "id")
	if err != nil {
		return
	}

	var body struct {
		VoteType string `json:"vote_type" form:"vote_type"`
	}
	if err := c.ShouldBind(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "vote_type required"})
		return
	}
	vt := model.VoteType(body.VoteType)
	if vt != model.VoteTypeUp && vt != model.VoteTypeDown {
		c.JSON(http.StatusBadRequest, gin.H{"error": "vote_type must be UP or DOWN"})
		return
	}

	counts, err := h.svc.CastVote(c.Request.Context(), actor.ID, resourceID, vt)
	if err != nil {
		renderError(c, err)
		return
	}
	c.JSON(http.StatusOK, counts)
}

// DeleteVote handles DELETE /resources/:id/vote
func (h *Handler) DeleteVote(c *gin.Context) {
	actor := middleware.GetAuthUser(c)
	resourceID, err := parseUUID(c, "id")
	if err != nil {
		return
	}

	counts, err := h.svc.RetractVote(c.Request.Context(), actor.ID, resourceID)
	if err != nil {
		renderError(c, err)
		return
	}
	c.JSON(http.StatusOK, counts)
}

// ── Favorites ─────────────────────────────────────────────────────────────────

// PostFavorite handles POST /resources/:id/favorite (toggle)
func (h *Handler) PostFavorite(c *gin.Context) {
	actor := middleware.GetAuthUser(c)
	resourceID, err := parseUUID(c, "id")
	if err != nil {
		return
	}

	added, err := h.svc.ToggleFavorite(c.Request.Context(), actor.ID, resourceID)
	if err != nil {
		renderError(c, err)
		return
	}

	status := "removed"
	if added {
		status = "added"
	}

	if c.GetHeader("HX-Request") == "true" {
		c.Header("HX-Trigger", "favoriteToggled")
	}
	c.JSON(http.StatusOK, gin.H{"status": status, "favorited": added})
}

// GetFavorites handles GET /favorites — list user's favorited resources.
func (h *Handler) GetFavorites(c *gin.Context) {
	actor := middleware.GetAuthUser(c)
	page := queryInt(c, "page", 1)
	pageSize := queryInt(c, "page_size", 20)

	resources, total, err := h.svc.ListFavorites(c.Request.Context(), actor.ID, page, pageSize)
	if err != nil {
		renderError(c, err)
		return
	}

	if c.GetHeader("HX-Request") == "true" || strings.Contains(c.GetHeader("Accept"), "text/html") {
		comp := engagementpages.FavoritesPage(engagementpages.FavoritesData{
			Resources: resources,
			Total:     total,
			Page:      page,
			AuthUser:  actor,
		})
		c.Status(http.StatusOK)
		_ = comp.Render(c.Request.Context(), c.Writer)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data": resources,
		"meta": gin.H{"total": total, "page": page, "page_size": pageSize},
	})
}

// ── Follows ───────────────────────────────────────────────────────────────────

// PostFollow handles POST /follows (toggle follow)
// Body: {"target_type": "AUTHOR"|"TOPIC", "target_id": "uuid"}
func (h *Handler) PostFollow(c *gin.Context) {
	actor := middleware.GetAuthUser(c)

	var body struct {
		TargetType string `json:"target_type" form:"target_type"`
		TargetID   string `json:"target_id" form:"target_id"`
	}
	if err := c.ShouldBind(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target_type and target_id required"})
		return
	}

	targetType := model.FollowTargetType(body.TargetType)
	if targetType != model.FollowTargetAuthor && targetType != model.FollowTargetTopic {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target_type must be AUTHOR or TOPIC"})
		return
	}

	targetID, err := uuid.Parse(body.TargetID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid target_id"})
		return
	}

	following, err := h.svc.ToggleFollow(c.Request.Context(), actor.ID, targetType, targetID)
	if err != nil {
		renderError(c, err)
		return
	}

	status := "unfollowed"
	if following {
		status = "following"
	}
	c.JSON(http.StatusOK, gin.H{"status": status, "following": following})
}

// ── helpers ───────────────────────────────────────────────────────────────────

func parseUUID(c *gin.Context, param string) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Param(param))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid " + param})
	}
	return id, err
}

func queryInt(c *gin.Context, key string, def int) int {
	v := c.DefaultQuery(key, "")
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func renderError(c *gin.Context, err error) {
	switch e := err.(type) {
	case *model.ValidationErrors:
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"status": "error",
			"error":  gin.H{"code": "VALIDATION_ERROR", "details": e.Errors},
		})
	default:
		if err == model.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		} else if err == model.ErrForbidden {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		}
	}
}
