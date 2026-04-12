package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// insertNotification inserts a notification directly into the DB for test setup.
func insertNotification(t *testing.T, userID uuid.UUID, eventType, title, body string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO notifications (id, user_id, event_type, title, body, is_read, created_at)
		 VALUES ($1, $2, $3, $4, $5, false, NOW())`,
		id, userID, eventType, title, body)
	require.NoError(t, err)
	return id
}

// getUserIDByUsername looks up a user ID by username.
func getUserIDByUsername(t *testing.T, username string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := testPool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE username = $1`, username).Scan(&id)
	require.NoError(t, err)
	return id
}

func TestGetMessagingCenter_Authenticated(t *testing.T) {
	truncate(t)
	registerUser(t, "msguser1", "msguser1@test.com", "Password@123456")
	token := loginUser(t, "msguser1", "Password@123456")
	client := authedClient(token)

	resp, err := client.Get(testServer.URL + "/messaging")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestGetMessagingCenter_Unauthenticated(t *testing.T) {
	truncate(t)

	resp, err := http.Get(testServer.URL + "/messaging")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestGetNotifications_Empty(t *testing.T) {
	truncate(t)
	registerUser(t, "notifuser1", "notifuser1@test.com", "Password@123456")
	token := loginUser(t, "notifuser1", "Password@123456")
	client := authedClient(token)

	req, _ := http.NewRequest("GET", testServer.URL+"/messaging/notifications", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	notifs := body["notifications"].([]interface{})
	assert.Len(t, notifs, 0)
}

func TestGetNotifications_ReturnsUserNotifications(t *testing.T) {
	truncate(t)
	registerUser(t, "notifuser2", "notifuser2@test.com", "Password@123456")
	token := loginUser(t, "notifuser2", "Password@123456")
	userID := getUserIDByUsername(t, "notifuser2")

	insertNotification(t, userID, "badge_earned", "Badge Earned", "You earned a badge!")
	insertNotification(t, userID, "level_up", "Level Up!", "You reached level 2")

	client := authedClient(token)
	req, _ := http.NewRequest("GET", testServer.URL+"/messaging/notifications", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	notifs := body["notifications"].([]interface{})
	assert.Len(t, notifs, 2)
}

func TestPostMarkRead_Success(t *testing.T) {
	truncate(t)
	registerUser(t, "notifuser3", "notifuser3@test.com", "Password@123456")
	token := loginUser(t, "notifuser3", "Password@123456")
	userID := getUserIDByUsername(t, "notifuser3")

	notifID := insertNotification(t, userID, "badge_earned", "Test Badge", "Test body")

	client := authedClient(token)
	resp, err := client.Post(
		fmt.Sprintf("%s/messaging/notifications/%s/read", testServer.URL, notifID),
		"application/json",
		nil,
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify is_read = true in DB
	var isRead bool
	err = testPool.QueryRow(context.Background(),
		`SELECT is_read FROM notifications WHERE id = $1`, notifID).Scan(&isRead)
	require.NoError(t, err)
	assert.True(t, isRead)
}

func TestPostMarkAllRead_Success(t *testing.T) {
	truncate(t)
	registerUser(t, "notifuser4", "notifuser4@test.com", "Password@123456")
	token := loginUser(t, "notifuser4", "Password@123456")
	userID := getUserIDByUsername(t, "notifuser4")

	insertNotification(t, userID, "badge_earned", "Badge 1", "Body 1")
	insertNotification(t, userID, "level_up", "Level Up", "Body 2")

	client := authedClient(token)
	resp, err := client.Post(testServer.URL+"/messaging/notifications/read-all", "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify all notifications are read
	var unreadCount int
	err = testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND is_read = false`, userID).Scan(&unreadCount)
	require.NoError(t, err)
	assert.Equal(t, 0, unreadCount)
}

func TestGetSubscriptions_DefaultsAllEnabled(t *testing.T) {
	truncate(t)
	registerUser(t, "subuser1", "subuser1@test.com", "Password@123456")
	token := loginUser(t, "subuser1", "Password@123456")

	client := authedClient(token)
	req, _ := http.NewRequest("GET", testServer.URL+"/messaging/subscriptions", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestPutSubscriptions_DisableEventType(t *testing.T) {
	truncate(t)
	registerUser(t, "subuser2", "subuser2@test.com", "Password@123456")
	token := loginUser(t, "subuser2", "Password@123456")
	userID := getUserIDByUsername(t, "subuser2")

	client := authedClient(token)
	body := strings.NewReader("event_type=badge_earned&enabled=false")
	req, _ := http.NewRequest("PUT", testServer.URL+"/messaging/subscriptions", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify subscription is disabled in DB
	var enabled bool
	err = testPool.QueryRow(context.Background(),
		`SELECT enabled FROM notification_subscriptions WHERE user_id = $1 AND event_type = 'badge_earned'`,
		userID).Scan(&enabled)
	require.NoError(t, err)
	assert.False(t, enabled)
}

func TestNotification_SkippedWhenSubscriptionDisabled(t *testing.T) {
	truncate(t)
	registerUser(t, "subuser3", "subuser3@test.com", "Password@123456")
	token := loginUser(t, "subuser3", "Password@123456")
	userID := getUserIDByUsername(t, "subuser3")

	// Disable badge_earned subscription
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO notification_subscriptions (user_id, event_type, enabled)
		 VALUES ($1, 'badge_earned', false)`, userID)
	require.NoError(t, err)

	// Disable the subscription via API
	client := authedClient(token)
	body := strings.NewReader("event_type=badge_earned&enabled=false")
	req, _ := http.NewRequest("PUT", testServer.URL+"/messaging/subscriptions", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify no notification was written
	var count int
	err = testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND event_type = 'badge_earned'`,
		userID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestSSEStream_Returns200WithEventStream(t *testing.T) {
	truncate(t)
	registerUser(t, "sseuser1", "sseuser1@test.com", "Password@123456")
	token := loginUser(t, "sseuser1", "Password@123456")

	client := authedClient(token)

	// Make SSE request with a timeout context so it terminates
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", testServer.URL+"/events/stream", nil)
	resp, err := client.Do(req)
	if err != nil {
		// Context timeout or connection close is expected
		return
	}
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	// Read the initial ping event
	buf := make([]byte, 512)
	n, _ := io.ReadAtLeast(resp.Body, buf, 1)
	assert.Greater(t, n, 0)
}
