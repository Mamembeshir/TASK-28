package messaginghandler

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/eduexchange/eduexchange/internal/middleware"
	"github.com/eduexchange/eduexchange/internal/model"
	messagingrepo "github.com/eduexchange/eduexchange/internal/repository/messaging"
	messagingservice "github.com/eduexchange/eduexchange/internal/service/messaging"
	"github.com/eduexchange/eduexchange/internal/sse"
	messagingpages "github.com/eduexchange/eduexchange/internal/templ/pages/messaging"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Handler handles messaging HTTP endpoints.
type Handler struct {
	notifSvc *messagingservice.NotificationService
	sseHub   *sse.Hub
}

// New creates a new messaging Handler.
func New(notifSvc *messagingservice.NotificationService, hub *sse.Hub) *Handler {
	return &Handler{notifSvc: notifSvc, sseHub: hub}
}

// GetCenter handles GET /messaging — full messaging center page.
func (h *Handler) GetCenter(c *gin.Context) {
	authUser := middleware.GetAuthUser(c)
	if authUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	filterParams := parseFilterParams(c)
	filter := buildRepoFilter(filterParams)

	notifications, _, err := h.notifSvc.GetAll(c.Request.Context(), authUser.ID, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch notifications"})
		return
	}

	_, count, _ := h.notifSvc.GetUnread(c.Request.Context(), authUser.ID)

	data := messagingpages.CenterData{
		Notifications: notifications,
		UnreadCount:   count,
		AuthUser:      authUser,
		CurrentFilter: filterParams,
	}

	if c.GetHeader("HX-Request") == "true" {
		c.Status(http.StatusOK)
		_ = messagingpages.CenterContent(data).Render(c.Request.Context(), c.Writer)
		return
	}

	c.Status(http.StatusOK)
	_ = messagingpages.CenterPage(data).Render(c.Request.Context(), c.Writer)
}

// GetNotifications handles GET /messaging/notifications — paginated list with filters.
func (h *Handler) GetNotifications(c *gin.Context) {
	authUser := middleware.GetAuthUser(c)
	if authUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	filterParams := parseFilterParams(c)
	filter := buildRepoFilter(filterParams)

	notifications, _, err := h.notifSvc.GetAll(c.Request.Context(), authUser.ID, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch notifications"})
		return
	}

	if c.GetHeader("HX-Request") == "true" {
		c.Status(http.StatusOK)
		_ = messagingpages.NotificationList(notifications).Render(c.Request.Context(), c.Writer)
		return
	}

	c.JSON(http.StatusOK, gin.H{"notifications": notifications})
}

// PostMarkRead handles POST /messaging/notifications/:id/read.
func (h *Handler) PostMarkRead(c *gin.Context) {
	authUser := middleware.GetAuthUser(c)
	if authUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	idStr := c.Param("id")
	notifID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid notification id"})
		return
	}

	if err := h.notifSvc.MarkRead(c.Request.Context(), notifID, authUser.ID); err != nil {
		if err == model.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "notification not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to mark read"})
		return
	}

	// Return updated card with is_read=true
	n := model.Notification{
		ID:     notifID,
		UserID: authUser.ID,
		IsRead: true,
	}
	if c.GetHeader("HX-Request") == "true" {
		c.Status(http.StatusOK)
		_ = messagingpages.NotificationCard(n).Render(c.Request.Context(), c.Writer)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// PostMarkAllRead handles POST /messaging/notifications/read-all.
func (h *Handler) PostMarkAllRead(c *gin.Context) {
	authUser := middleware.GetAuthUser(c)
	if authUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	if err := h.notifSvc.BulkMarkRead(c.Request.Context(), authUser.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to mark all read"})
		return
	}

	// Return updated (empty) notification list
	if c.GetHeader("HX-Request") == "true" {
		c.Status(http.StatusOK)
		_ = messagingpages.NotificationList([]model.Notification{}).Render(c.Request.Context(), c.Writer)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// GetSubscriptions handles GET /messaging/subscriptions.
func (h *Handler) GetSubscriptions(c *gin.Context) {
	authUser := middleware.GetAuthUser(c)
	if authUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	subs, err := h.notifSvc.ListSubscriptions(c.Request.Context(), authUser.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch subscriptions"})
		return
	}

	// Import the subscriptions page package
	data := messagingpages.SubscriptionsData{
		Subscriptions: subs,
		AuthUser:      authUser,
	}

	if c.GetHeader("HX-Request") == "true" {
		c.Status(http.StatusOK)
		_ = messagingpages.SubscriptionsContent(data).Render(c.Request.Context(), c.Writer)
		return
	}

	c.Status(http.StatusOK)
	_ = messagingpages.SubscriptionsPage(data).Render(c.Request.Context(), c.Writer)
}

// PutSubscriptions handles PUT /messaging/subscriptions.
func (h *Handler) PutSubscriptions(c *gin.Context) {
	authUser := middleware.GetAuthUser(c)
	if authUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	eventTypeStr := c.PostForm("event_type")
	enabledStr := c.PostForm("enabled")

	if eventTypeStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "event_type required"})
		return
	}

	enabled := enabledStr == "true" || enabledStr == "1"
	eventType := model.EventType(eventTypeStr)

	if err := h.notifSvc.ManageSubscription(c.Request.Context(), authUser.ID, eventType, enabled); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update subscription"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok", "event_type": eventTypeStr, "enabled": enabled})
}

// GetEventStream handles GET /api/v1/events/stream — SSE endpoint.
func (h *Handler) GetEventStream(c *gin.Context) {
	authUser := middleware.GetAuthUser(c)
	if authUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	client := &sse.Client{
		UserID: authUser.ID,
		Events: make(chan sse.Event, 10),
	}
	h.sseHub.Register(client)
	defer h.sseHub.Unregister(client)

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	flusher, canFlush := c.Writer.(http.Flusher)

	// Send initial ping to establish connection
	fmt.Fprintf(c.Writer, "event: ping\ndata: connected\n\n")
	if canFlush {
		flusher.Flush()
	}

	for {
		select {
		case event, ok := <-client.Events:
			if !ok {
				return
			}
			fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event.Type, event.Data)
			if canFlush {
				flusher.Flush()
			}
		case <-c.Request.Context().Done():
			return
		}
	}
}

// GetUnreadCount handles GET /messaging/notifications/unread-count
// Returns the unread notification count as plain text for HTMX polling.
func (h *Handler) GetUnreadCount(c *gin.Context) {
	authUser := middleware.GetAuthUser(c)
	if authUser == nil {
		c.String(http.StatusOK, "0")
		return
	}
	_, count, err := h.notifSvc.GetUnread(c.Request.Context(), authUser.ID)
	if err != nil {
		c.String(http.StatusOK, "0")
		return
	}
	if count == 0 {
		c.String(http.StatusOK, "")
		return
	}
	c.String(http.StatusOK, strconv.Itoa(count))
}

// ── helpers ───────────────────────────────────────────────────────────────────

func parseFilterParams(c *gin.Context) messagingpages.NotificationFilterParams {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	return messagingpages.NotificationFilterParams{
		EventType: c.Query("event_type"),
		IsRead:    c.DefaultQuery("is_read", "all"),
		Page:      page,
		PageSize:  pageSize,
	}
}

func buildRepoFilter(params messagingpages.NotificationFilterParams) messagingrepo.NotificationFilter {
	filter := messagingrepo.NotificationFilter{
		Page:     params.Page,
		PageSize: params.PageSize,
	}
	if params.EventType != "" {
		et := model.EventType(params.EventType)
		filter.EventType = &et
	}
	switch params.IsRead {
	case "read":
		t := true
		filter.IsRead = &t
	case "unread":
		f := false
		filter.IsRead = &f
	}
	return filter
}

func unreadFilter() messagingrepo.NotificationFilter {
	f := false
	return messagingrepo.NotificationFilter{
		IsRead:   &f,
		Page:     1,
		PageSize: 1,
	}
}
