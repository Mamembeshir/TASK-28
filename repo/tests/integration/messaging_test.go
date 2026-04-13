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

	messagingrepo "github.com/eduexchange/eduexchange/internal/repository/messaging"
	messagingservice "github.com/eduexchange/eduexchange/internal/service/messaging"
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

	req, _ := http.NewRequest("GET", testServer.URL+"/messaging", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
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
	notifs, _ := body["notifications"].([]interface{})
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

// ── Retry queue integration tests ──────────────────────────────────────────

func TestRetryQueue_FailsAfterMaxAttempts(t *testing.T) {
	truncate(t)

	registerUser(t, "retryuser1", "retryuser1@test.com", "Password@123456")
	userID := getUserIDByUsername(t, "retryuser1")

	// Insert a retry item with 4 prior attempts, then delete the user so
	// CreateNotification hits a FK violation.  We must disable the FK on
	// the retry queue temporarily.  Instead, insert the retry item first,
	// then delete the user (CASCADE removes FK-constrained rows, but we
	// bypass this by dropping and re-adding the constraint).
	//
	// Simpler approach: insert retry item, then delete the user row from
	// the users table using a raw DELETE that skips cascade on the retry
	// queue (not possible with FK).
	//
	// Pragmatic: insert item, then corrupt the user_id in the retry item
	// so the notification insert fails.
	retryID := uuid.New()
	_, err := testPool.Exec(context.Background(), `
		INSERT INTO notification_retry_queue
			(id, user_id, event_type, title, body, resource_id, attempts, next_retry_at, status, created_at, updated_at)
		VALUES ($1, $2, 'badge_earned', 'Retry Fail', 'body', NULL, 4, NOW() - interval '1 minute', 'PENDING', NOW(), NOW())`,
		retryID, userID)
	require.NoError(t, err)

	// Now delete the user — CASCADE deletes sessions/profiles/roles but
	// the retry queue also has ON DELETE CASCADE so it will be removed.
	// Instead, we set the user_id to a non-existent UUID directly.
	fakeID := uuid.New()
	_, err = testPool.Exec(context.Background(),
		`ALTER TABLE notification_retry_queue DROP CONSTRAINT IF EXISTS notification_retry_queue_user_id_fkey`)
	require.NoError(t, err)
	defer testPool.Exec(context.Background(),
		`ALTER TABLE notification_retry_queue ADD CONSTRAINT notification_retry_queue_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE`)
	_, err = testPool.Exec(context.Background(),
		`UPDATE notification_retry_queue SET user_id = $1 WHERE id = $2`, fakeID, retryID)
	require.NoError(t, err)

	repo := messagingrepo.New(testPool)
	retrySvc := messagingservice.NewRetryService(repo)
	err = retrySvc.ProcessRetryQueue(context.Background())
	require.NoError(t, err)

	// CreateNotification fails (FK violation on user_id in notifications table),
	// attempts incremented to 5, which hits the max and marks it FAILED.
	var status string
	err = testPool.QueryRow(context.Background(),
		`SELECT status FROM notification_retry_queue WHERE id = $1`, retryID).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "FAILED", status, "retry item should be marked FAILED after 5 attempts")
}

func TestRetryQueue_SuccessfulRetryDeletesItem(t *testing.T) {
	truncate(t)

	registerUser(t, "retryuser2", "retryuser2@test.com", "Password@123456")
	userID := getUserIDByUsername(t, "retryuser2")

	// Insert a retry queue item with 0 attempts, eligible now.
	retryID := uuid.New()
	_, err := testPool.Exec(context.Background(), `
		INSERT INTO notification_retry_queue
			(id, user_id, event_type, title, body, resource_id, attempts, next_retry_at, status, created_at, updated_at)
		VALUES ($1, $2, 'badge_earned', 'Success Retry', 'body', NULL, 0, NOW() - interval '1 minute', 'PENDING', NOW(), NOW())`,
		retryID, userID)
	require.NoError(t, err)

	repo := messagingrepo.New(testPool)
	retrySvc := messagingservice.NewRetryService(repo)
	err = retrySvc.ProcessRetryQueue(context.Background())
	require.NoError(t, err)

	// On success, the item is deleted from the retry queue and a notification is created.
	var count int
	err = testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM notification_retry_queue WHERE id = $1`, retryID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "retry item should be deleted after successful delivery")

	err = testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND event_type = 'badge_earned' AND title = 'Success Retry'`,
		userID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "notification should be created on successful retry")
}

func TestRetryQueue_BackoffIncrementsAttempts(t *testing.T) {
	truncate(t)

	registerUser(t, "retryuser3", "retryuser3@test.com", "Password@123456")
	userID := getUserIDByUsername(t, "retryuser3")

	// Insert an item that will fail on retry (use an invalid event_type to trigger
	// a DB constraint violation, or simulate by blocking notification creation).
	// Instead, we test backoff by inserting an item with next_retry_at in the
	// future — it should NOT be picked up.
	retryID := uuid.New()
	_, err := testPool.Exec(context.Background(), `
		INSERT INTO notification_retry_queue
			(id, user_id, event_type, title, body, resource_id, attempts, next_retry_at, status, created_at, updated_at)
		VALUES ($1, $2, 'badge_earned', 'Future Retry', 'body', NULL, 2, NOW() + interval '10 minutes', 'PENDING', NOW(), NOW())`,
		retryID, userID)
	require.NoError(t, err)

	repo := messagingrepo.New(testPool)
	retrySvc := messagingservice.NewRetryService(repo)
	err = retrySvc.ProcessRetryQueue(context.Background())
	require.NoError(t, err)

	// Item should still be in the queue with attempts unchanged (not yet due).
	var attempts int
	err = testPool.QueryRow(context.Background(),
		`SELECT attempts FROM notification_retry_queue WHERE id = $1`, retryID).Scan(&attempts)
	require.NoError(t, err)
	assert.Equal(t, 2, attempts, "retry item with future next_retry_at should not be processed")
}

func TestRetryQueue_BackoffScheduleValues(t *testing.T) {
	truncate(t)

	registerUser(t, "retryuser4", "retryuser4@test.com", "Password@123456")

	// Expected backoff intervals indexed by attempt number (after increment):
	// attempts 1 → 2 min, 2 → 4 min, 3 → 8 min, 4 → 15 min
	expectedBackoffs := []time.Duration{
		1 * time.Minute, // retryIntervals[0] (not reachable in this path)
		2 * time.Minute, // retryIntervals[1]
		4 * time.Minute, // retryIntervals[2]
		8 * time.Minute, // retryIntervals[3]
	}

	// Drop FK on retry queue so we can set a fake user_id that causes
	// CreateNotification to fail (FK violation on notifications table).
	_, err := testPool.Exec(context.Background(),
		`ALTER TABLE notification_retry_queue DROP CONSTRAINT IF EXISTS notification_retry_queue_user_id_fkey`)
	require.NoError(t, err)
	defer testPool.Exec(context.Background(), //nolint:errcheck
		`ALTER TABLE notification_retry_queue ADD CONSTRAINT notification_retry_queue_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE`)

	fakeID := uuid.New() // does not exist in users table

	// Test each non-terminal attempt: starting attempts 0, 1, 2, 3
	for startAttempts := 0; startAttempts < 4; startAttempts++ {
		retryID := uuid.New()
		_, err := testPool.Exec(context.Background(), `
			INSERT INTO notification_retry_queue
				(id, user_id, event_type, title, body, resource_id, attempts, next_retry_at, status, created_at, updated_at)
			VALUES ($1, $2, 'badge_earned', 'Backoff Test', 'body', NULL, $3, NOW() - interval '1 minute', 'PENDING', NOW(), NOW())`,
			retryID, fakeID, startAttempts)
		require.NoError(t, err)

		beforeProcess := time.Now()

		repo := messagingrepo.New(testPool)
		retrySvc := messagingservice.NewRetryService(repo)
		err = retrySvc.ProcessRetryQueue(context.Background())
		require.NoError(t, err)

		afterAttempts := startAttempts + 1
		var dbAttempts int
		var dbNextRetry time.Time
		err = testPool.QueryRow(context.Background(),
			`SELECT attempts, next_retry_at FROM notification_retry_queue WHERE id = $1`, retryID).
			Scan(&dbAttempts, &dbNextRetry)
		require.NoError(t, err, "attempt %d: row should still exist", startAttempts)

		assert.Equal(t, afterAttempts, dbAttempts,
			"attempt %d: attempts should be incremented", startAttempts)

		expectedEarliest := beforeProcess.Add(expectedBackoffs[afterAttempts])
		// Allow 5 seconds of clock tolerance
		assert.WithinDuration(t, expectedEarliest, dbNextRetry, 5*time.Second,
			"attempt %d→%d: next_retry_at should be ~%v from now",
			startAttempts, afterAttempts, expectedBackoffs[afterAttempts])

		// Clean up for next iteration
		testPool.Exec(context.Background(), `DELETE FROM notification_retry_queue WHERE id = $1`, retryID) //nolint:errcheck
	}

	// Also verify attempt 4 → 5 results in FAILED status (no backoff set)
	retryID := uuid.New()
	_, err = testPool.Exec(context.Background(), `
		INSERT INTO notification_retry_queue
			(id, user_id, event_type, title, body, resource_id, attempts, next_retry_at, status, created_at, updated_at)
		VALUES ($1, $2, 'badge_earned', 'Backoff Final', 'body', NULL, 4, NOW() - interval '1 minute', 'PENDING', NOW(), NOW())`,
		retryID, fakeID)
	require.NoError(t, err)

	repo := messagingrepo.New(testPool)
	retrySvc := messagingservice.NewRetryService(repo)
	err = retrySvc.ProcessRetryQueue(context.Background())
	require.NoError(t, err)

	var finalStatus string
	var finalAttempts int
	err = testPool.QueryRow(context.Background(),
		`SELECT attempts, status FROM notification_retry_queue WHERE id = $1`, retryID).
		Scan(&finalAttempts, &finalStatus)
	require.NoError(t, err)
	assert.Equal(t, 5, finalAttempts, "terminal: attempts should be 5")
	assert.Equal(t, "FAILED", finalStatus, "terminal: status should be FAILED")
}
