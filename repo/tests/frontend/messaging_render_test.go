package frontend_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/eduexchange/eduexchange/internal/model"
	components "github.com/eduexchange/eduexchange/internal/templ/components"
	messagingpages "github.com/eduexchange/eduexchange/internal/templ/pages/messaging"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCenterPage_Renders(t *testing.T) {
	data := messagingpages.CenterData{
		Notifications: []model.Notification{},
		UnreadCount:   0,
		AuthUser:      testAuthUser,
		CurrentFilter: messagingpages.NotificationFilterParams{IsRead: "all", Page: 1, PageSize: 20},
	}

	var buf bytes.Buffer
	err := messagingpages.CenterPage(data).Render(context.Background(), &buf)
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "Messaging Center")
	assert.Contains(t, html, "Mark All Read")
}

func TestCenterPage_ShowsUnreadCount(t *testing.T) {
	data := messagingpages.CenterData{
		Notifications: []model.Notification{
			{ID: uuid.New(), UserID: uuid.New(), EventType: model.EventBadgeEarned, Title: "Badge!", Body: "You earned it", IsRead: false, CreatedAt: time.Now()},
		},
		UnreadCount:   1,
		AuthUser:      testAuthUser,
		CurrentFilter: messagingpages.NotificationFilterParams{IsRead: "unread", Page: 1, PageSize: 20},
	}

	var buf bytes.Buffer
	err := messagingpages.CenterPage(data).Render(context.Background(), &buf)
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "1")
	assert.Contains(t, html, "Badge!")
}

func TestNotificationCard_UnreadHasDot(t *testing.T) {
	n := model.Notification{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		EventType: model.EventLevelUp,
		Title:     "Level Up!",
		Body:      "You reached level 2",
		IsRead:    false,
		CreatedAt: time.Now(),
	}

	var buf bytes.Buffer
	err := messagingpages.NotificationCard(n).Render(context.Background(), &buf)
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "Level Up!")
	// Unread dot class should be present
	assert.Contains(t, html, "bg-blue")
}

func TestNotificationCard_ReadState(t *testing.T) {
	readAt := time.Now()
	n := model.Notification{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		EventType: model.EventBadgeEarned,
		Title:     "Old Badge",
		Body:      "Already read",
		IsRead:    true,
		ReadAt:    &readAt,
		CreatedAt: time.Now().Add(-1 * time.Hour),
	}

	var buf bytes.Buffer
	err := messagingpages.NotificationCard(n).Render(context.Background(), &buf)
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "Old Badge")
}

func TestNotificationList_EmptyState(t *testing.T) {
	var buf bytes.Buffer
	err := messagingpages.NotificationList([]model.Notification{}).Render(context.Background(), &buf)
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "No notifications")
}

func TestNotificationList_WithItems(t *testing.T) {
	notifs := []model.Notification{
		{ID: uuid.New(), UserID: uuid.New(), EventType: model.EventReviewDecision, Title: "Approved!", Body: "Your resource was approved", IsRead: false, CreatedAt: time.Now()},
		{ID: uuid.New(), UserID: uuid.New(), EventType: model.EventBanNotice, Title: "Account Banned", Body: "You are banned", IsRead: false, CreatedAt: time.Now()},
	}

	var buf bytes.Buffer
	err := messagingpages.NotificationList(notifs).Render(context.Background(), &buf)
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "Approved!")
	assert.Contains(t, html, "Account Banned")
}

func TestSubscriptionsPage_Renders(t *testing.T) {
	subs := []model.NotificationSubscription{
		{UserID: uuid.New(), EventType: model.EventBadgeEarned, Enabled: true},
		{UserID: uuid.New(), EventType: model.EventLevelUp, Enabled: false},
	}

	data := messagingpages.SubscriptionsData{
		Subscriptions: subs,
		AuthUser:      testAuthUser,
	}

	var buf bytes.Buffer
	err := messagingpages.SubscriptionsPage(data).Render(context.Background(), &buf)
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "Notification Preferences")
	assert.Contains(t, html, "badge_earned")
	assert.Contains(t, html, "level_up")
}

func TestSubscriptionCard_EnabledState(t *testing.T) {
	sub := model.NotificationSubscription{
		UserID:    uuid.New(),
		EventType: model.EventReviewDecision,
		Enabled:   true,
	}

	var buf bytes.Buffer
	err := messagingpages.SubscriptionCard(sub).Render(context.Background(), &buf)
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "review_decision")
	assert.Contains(t, html, "checked")
}

func TestSubscriptionCard_DisabledState(t *testing.T) {
	sub := model.NotificationSubscription{
		UserID:    uuid.New(),
		EventType: model.EventFollowNewContent,
		Enabled:   false,
	}

	var buf bytes.Buffer
	err := messagingpages.SubscriptionCard(sub).Render(context.Background(), &buf)
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "follow_new_content")
	// Not checked
	assert.NotContains(t, html, `checked="checked"`)
}

func TestNotificationBell_NoUnread(t *testing.T) {
	data := components.BellData{
		UnreadCount:  0,
		RecentNotifs: []model.Notification{},
		AuthUser:     testAuthUser,
	}

	var buf bytes.Buffer
	err := components.NotificationBell(data).Render(context.Background(), &buf)
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "View All")
	// With 0 unread, badge class should be hidden
	assert.Contains(t, html, `class="hidden`)
}

func TestNotificationBell_WithUnread(t *testing.T) {
	data := components.BellData{
		UnreadCount: 5,
		RecentNotifs: []model.Notification{
			{ID: uuid.New(), EventType: model.EventBadgeEarned, Title: "Badge!", Body: "Earned", IsRead: false, CreatedAt: time.Now()},
		},
		AuthUser: testAuthUser,
	}

	var buf bytes.Buffer
	err := components.NotificationBell(data).Render(context.Background(), &buf)
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "5")
	assert.Contains(t, html, "Badge!")
}

func TestNotificationToast_Renders(t *testing.T) {
	data := components.ToastData{
		Title:     "Badge Earned",
		Body:      "You earned the Popular badge!",
		EventType: string(model.EventBadgeEarned),
	}

	var buf bytes.Buffer
	err := components.NotificationToast(data).Render(context.Background(), &buf)
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "Badge Earned")
	assert.Contains(t, html, "You earned the Popular badge!")
	// Auto-dismiss script should be present
	assert.True(t, strings.Contains(html, "setTimeout") || strings.Contains(html, "5000"))
}
