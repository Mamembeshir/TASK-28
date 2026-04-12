package adminhandler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/eduexchange/eduexchange/internal/middleware"
	"github.com/eduexchange/eduexchange/internal/model"
	authrepo "github.com/eduexchange/eduexchange/internal/repository/auth"
	authservice "github.com/eduexchange/eduexchange/internal/service/auth"
	adminpages "github.com/eduexchange/eduexchange/internal/templ/pages/admin"
)

// UserHandler serves admin user management routes.
type UserHandler struct {
	userSvc *authservice.UserService
}

func NewUserHandler(userSvc *authservice.UserService) *UserHandler {
	return &UserHandler{userSvc: userSvc}
}

// GetUserList renders the paginated user list with filter support.
func (h *UserHandler) GetUserList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	filter := authrepo.ListFilter{
		Page:     page,
		PageSize: pageSize,
		Status:   c.Query("status"),
		Role:     c.Query("role"),
		Search:   c.Query("search"),
	}

	users, total, err := h.userSvc.ListUsers(c.Request.Context(), filter)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	authUser := middleware.GetAuthUser(c)
	username := ""
	if authUser != nil {
		username = authUser.Username
	}

	data := adminpages.UserListData{
		Users:    users,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
		Filter:   filter,
		Username: username,
	}

	c.Status(http.StatusOK)
	_ = adminpages.UserListPage(data).Render(c.Request.Context(), c.Writer)
}

// GetUserDetail renders the detail page for a single user.
func (h *UserHandler) GetUserDetail(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}

	uwr, err := h.userSvc.GetUser(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			c.Status(http.StatusNotFound)
			return
		}
		c.Status(http.StatusInternalServerError)
		return
	}

	authUser := middleware.GetAuthUser(c)
	username := ""
	if authUser != nil {
		username = authUser.Username
	}

	data := adminpages.UserDetailData{
		UserWithRoles: *uwr,
		Username:      username,
	}

	c.Status(http.StatusOK)
	_ = adminpages.UserDetailPage(data).Render(c.Request.Context(), c.Writer)
}

// PostTransitionStatus changes a user's status.
func (h *UserHandler) PostTransitionStatus(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "Invalid user ID")
		return
	}

	toStr := c.PostForm("status")
	version, _ := strconv.Atoi(c.PostForm("version"))

	to, err := model.ParseUserStatus(toStr)
	if err != nil {
		respondError(c, http.StatusUnprocessableEntity, "Invalid status value")
		return
	}

	actor := middleware.GetAuthUser(c)
	var actorID uuid.UUID
	if actor != nil {
		actorID = actor.ID
	}

	if err := h.userSvc.TransitionStatus(c.Request.Context(), actorID, id, to, version); err != nil {
		switch {
		case errors.Is(err, model.ErrStaleVersion):
			respondError(c, http.StatusConflict, "The record was modified by someone else. Please refresh and try again.")
		case errors.Is(err, model.ErrNotFound):
			respondError(c, http.StatusNotFound, "User not found")
		default:
			respondError(c, http.StatusUnprocessableEntity, err.Error())
		}
		return
	}

	// Re-render detail page after change
	h.GetUserDetail(c)
}

// PostAssignRole adds a role to a user.
func (h *UserHandler) PostAssignRole(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "Invalid user ID")
		return
	}

	roleStr := c.PostForm("role")
	role, err := model.ParseRole(roleStr)
	if err != nil {
		respondError(c, http.StatusUnprocessableEntity, "Invalid role")
		return
	}

	actor := middleware.GetAuthUser(c)
	var actorID uuid.UUID
	if actor != nil {
		actorID = actor.ID
	}

	if err := h.userSvc.AssignRole(c.Request.Context(), actorID, id, role); err != nil {
		respondError(c, http.StatusInternalServerError, "Failed to assign role")
		return
	}

	h.GetUserDetail(c)
}

// PostRemoveRole removes a role from a user.
func (h *UserHandler) PostRemoveRole(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "Invalid user ID")
		return
	}

	roleStr := c.PostForm("role")
	role, err := model.ParseRole(roleStr)
	if err != nil {
		respondError(c, http.StatusUnprocessableEntity, "Invalid role")
		return
	}

	actor := middleware.GetAuthUser(c)
	var actorID uuid.UUID
	if actor != nil {
		actorID = actor.ID
	}

	if err := h.userSvc.RemoveRole(c.Request.Context(), actorID, id, role); err != nil {
		var ve *model.ValidationErrors
		if errors.As(err, &ve) {
			respondError(c, http.StatusUnprocessableEntity, ve.Error())
		} else if errors.Is(err, model.ErrNotFound) {
			respondError(c, http.StatusNotFound, "Role not assigned to user")
		} else {
			respondError(c, http.StatusInternalServerError, "Failed to remove role")
		}
		return
	}

	h.GetUserDetail(c)
}

// PostUnlockUser clears the lockout on a user.
func (h *UserHandler) PostUnlockUser(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "Invalid user ID")
		return
	}

	actor := middleware.GetAuthUser(c)
	var actorID uuid.UUID
	if actor != nil {
		actorID = actor.ID
	}

	if err := h.userSvc.UnlockUser(c.Request.Context(), actorID, id); err != nil {
		respondError(c, http.StatusInternalServerError, "Failed to unlock user")
		return
	}

	h.GetUserDetail(c)
}

func respondError(c *gin.Context, code int, msg string) {
	if c.GetHeader("HX-Request") == "true" {
		c.Header("HX-Reswap", "none")
		c.Data(code, "text/plain; charset=utf-8", []byte(msg))
		return
	}
	c.JSON(code, gin.H{"error": msg})
}
