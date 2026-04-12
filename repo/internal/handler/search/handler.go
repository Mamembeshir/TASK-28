package searchhandler

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/eduexchange/eduexchange/internal/middleware"
	"github.com/eduexchange/eduexchange/internal/model"
	gamificationservice "github.com/eduexchange/eduexchange/internal/service/gamification"
	"github.com/eduexchange/eduexchange/internal/service/recommendations"
	searchservice "github.com/eduexchange/eduexchange/internal/service/search"
	searchpages "github.com/eduexchange/eduexchange/internal/templ/pages/search"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Handler handles search, rankings, and recommendations.
type Handler struct {
	searchSvc  *searchservice.SearchService
	rankSvc    *gamificationservice.RankingService
	recSvc     *recommendations.RecommendationService
}

func New(
	searchSvc *searchservice.SearchService,
	rankSvc *gamificationservice.RankingService,
	recSvc *recommendations.RecommendationService,
) *Handler {
	return &Handler{
		searchSvc: searchSvc,
		rankSvc:   rankSvc,
		recSvc:    recSvc,
	}
}

// ── Search ────────────────────────────────────────────────────────────────────

// GetSearch handles GET /search
func (h *Handler) GetSearch(c *gin.Context) {
	var userID *uuid.UUID
	if au := middleware.GetAuthUser(c); au != nil {
		userID = &au.ID
	}

	f := buildFilter(c, userID)
	result, err := h.searchSvc.Search(c.Request.Context(), f, userID)
	if err != nil {
		log.Printf("[SEARCH] search failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "search failed"})
		return
	}

	if c.GetHeader("HX-Request") == "true" {
		// Return partial results fragment.
		authUser := authUserFromCtx(c)
		comp := searchpages.SearchResultsFragment(searchpages.SearchData{
			Result:   result,
			AuthUser: authUser,
		})
		c.Status(http.StatusOK)
		_ = comp.Render(c.Request.Context(), c.Writer)
		return
	}

	authUser := authUserFromCtx(c)
	var history []model.UserSearchHistory
	if userID != nil {
		history, _ = h.searchSvc.GetHistory(c.Request.Context(), *userID)
	}

	comp := searchpages.SearchPage(searchpages.SearchData{
		Result:   result,
		History:  history,
		AuthUser: authUser,
	})
	c.Status(http.StatusOK)
	_ = comp.Render(c.Request.Context(), c.Writer)
}

// GetSuggest handles GET /search/suggest?q=prefix
func (h *Handler) GetSuggest(c *gin.Context) {
	prefix := c.Query("q")
	suggestions, err := h.searchSvc.TypeAhead(c.Request.Context(), prefix)
	if err != nil || len(suggestions) == 0 {
		c.JSON(http.StatusOK, gin.H{"suggestions": []string{}})
		return
	}

	if c.GetHeader("HX-Request") == "true" {
		comp := searchpages.SuggestionsDropdown(suggestions)
		c.Status(http.StatusOK)
		_ = comp.Render(c.Request.Context(), c.Writer)
		return
	}
	c.JSON(http.StatusOK, gin.H{"suggestions": suggestions})
}

// GetHistory handles GET /search/history
func (h *Handler) GetHistory(c *gin.Context) {
	actor := middleware.GetAuthUser(c)
	history, err := h.searchSvc.GetHistory(c.Request.Context(), actor.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": history})
}

// DeleteHistory handles DELETE /search/history
func (h *Handler) DeleteHistory(c *gin.Context) {
	actor := middleware.GetAuthUser(c)
	if err := h.searchSvc.ClearHistory(c.Request.Context(), actor.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "cleared"})
}

// ── Rankings ──────────────────────────────────────────────────────────────────

// GetBestsellers handles GET /rankings/bestsellers
func (h *Handler) GetBestsellers(c *gin.Context) {
	entries, err := h.rankSvc.GetBestsellers(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed"})
		return
	}
	if c.GetHeader("HX-Request") == "true" || strings.Contains(c.GetHeader("Accept"), "text/html") {
		comp := searchpages.RankingsPage(searchpages.RankingsData{
			Bestsellers: entries,
			AuthUser:    authUserFromCtx(c),
		})
		c.Status(http.StatusOK)
		_ = comp.Render(c.Request.Context(), c.Writer)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": entries})
}

// GetNewReleases handles GET /rankings/new-releases
func (h *Handler) GetNewReleases(c *gin.Context) {
	entries, err := h.rankSvc.GetNewReleases(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": entries})
}

// GetHome handles GET / — home page with rankings + recommendations
func (h *Handler) GetHome(c *gin.Context) {
	authUser := authUserFromCtx(c)

	bestsellers, _ := h.rankSvc.GetBestsellers(c.Request.Context())
	releases, _ := h.rankSvc.GetNewReleases(c.Request.Context())

	var sections []model.RecommendationSection
	if authUser != nil {
		sections, _ = h.recSvc.GetRecommendations(c.Request.Context(), authUser.ID)
	}

	comp := searchpages.HomePage(searchpages.HomeData{
		Bestsellers:     bestsellers,
		NewReleases:     releases,
		Recommendations: sections,
		AuthUser:        authUser,
	})
	c.Status(http.StatusOK)
	_ = comp.Render(c.Request.Context(), c.Writer)
}

// ── Recommendations ───────────────────────────────────────────────────────────

// GetRecommendations handles GET /recommendations
func (h *Handler) GetRecommendations(c *gin.Context) {
	actor := middleware.GetAuthUser(c)
	sections, err := h.recSvc.GetRecommendations(c.Request.Context(), actor.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": sections})
}

// GetStrategyConfigs handles GET /admin/recommendation-strategies
func (h *Handler) GetStrategyConfigs(c *gin.Context) {
	cfgs, err := h.recSvc.ListStrategyConfigs(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed"})
		return
	}
	if c.GetHeader("HX-Request") == "true" || strings.Contains(c.GetHeader("Accept"), "text/html") {
		comp := searchpages.StrategyConfigPage(cfgs, authUserFromCtx(c))
		c.Status(http.StatusOK)
		_ = comp.Render(c.Request.Context(), c.Writer)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": cfgs})
}

// PutStrategyConfig handles PUT /admin/recommendation-strategies/:id
func (h *Handler) PutStrategyConfig(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var body struct {
		Label     string `json:"label" form:"label"`
		SortOrder int    `json:"sort_order" form:"sort_order"`
		IsActive  bool   `json:"is_active" form:"is_active"`
	}
	if err := c.ShouldBind(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}

	cfg := &model.RecommendationStrategyConfig{
		ID:        id,
		Label:     body.Label,
		SortOrder: body.SortOrder,
		IsActive:  body.IsActive,
	}
	if err := h.recSvc.UpdateStrategyConfig(c.Request.Context(), cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

// ── helpers ───────────────────────────────────────────────────────────────────

func buildFilter(c *gin.Context, userID *uuid.UUID) model.SearchFilter {
	f := model.SearchFilter{
		Query:    c.Query("q"),
		Sort:     c.Query("sort"),
		Page:     queryInt(c, "page", 1),
		PageSize: queryInt(c, "page_size", 20),
	}

	if catStr := c.Query("category_id"); catStr != "" {
		if id, err := uuid.Parse(catStr); err == nil {
			f.CategoryID = &id
		}
	}
	if authorStr := c.Query("author_id"); authorStr != "" {
		if id, err := uuid.Parse(authorStr); err == nil {
			f.AuthorID = &id
		}
	}
	if tags := c.QueryArray("tag"); len(tags) > 0 {
		f.Tags = tags
	}
	if fromStr := c.Query("date_from"); fromStr != "" {
		if t, err := time.Parse("2006-01-02", fromStr); err == nil {
			f.DateFrom = &t
		}
	}
	if toStr := c.Query("date_to"); toStr != "" {
		if t, err := time.Parse("2006-01-02", toStr); err == nil {
			f.DateTo = &t
		}
	}
	// Status: only admin/reviewer can filter by non-PUBLISHED statuses.
	if statusParam := c.Query("status"); statusParam != "" && userID != nil {
		authUser := middleware.GetAuthUser(c)
		if statusParam == "PUBLISHED" {
			f.Status = statusParam
		} else if authUser != nil && hasSearchRole(authUser, "ADMIN", "REVIEWER") {
			f.Status = statusParam
		}
		// Non-admin/reviewer requesting non-PUBLISHED status: ignore the filter
		// (defaults to PUBLISHED in the repository layer).
	}
	return f
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

func authUserFromCtx(c *gin.Context) *middleware.AuthUser {
	return middleware.GetAuthUser(c)
}

func hasSearchRole(u *middleware.AuthUser, roles ...string) bool {
	for _, required := range roles {
		for _, r := range u.Roles {
			if r == required {
				return true
			}
		}
	}
	return false
}
