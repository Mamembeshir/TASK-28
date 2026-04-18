package integration_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── GET /messaging/notifications/unread-count ───────────────────────────────

func TestGetUnreadCount_Authenticated_ReturnsCount(t *testing.T) {
	truncate(t)
	registerUser(t, "unread_user1", "unread_user1@example.com", "SecurePass1!")
	token := loginUser(t, "unread_user1", "SecurePass1!")

	req, _ := http.NewRequest(http.MethodGet,
		testServer.URL+"/messaging/notifications/unread-count", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := authedClient(t, token).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Contains(t, body, "count", "response should contain 'count' field")

	// New user should have 0 unread notifications
	count := body["count"].(float64)
	assert.Equal(t, float64(0), count)
}

func TestGetUnreadCount_Unauthenticated_Redirects(t *testing.T) {
	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(testServer.URL + "/messaging/notifications/unread-count")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.True(t, resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusUnauthorized,
		"expected redirect or 401, got %d", resp.StatusCode)
}

func TestGetUnreadCount_AfterReceivingNotification_CountIncreases(t *testing.T) {
	truncate(t)
	registerUser(t, "unread_author1", "unread_author1@example.com", "SecurePass1!")
	registerUser(t, "unread_reviewer1", "unread_reviewer1@example.com", "SecurePass1!")
	registerUser(t, "unread_admin1", "unread_admin1@example.com", "SecurePass1!")
	makeAuthor(t, "unread_author1")
	makeReviewer(t, "unread_reviewer1")
	makeAdmin(t, "unread_admin1")

	authorToken := loginUser(t, "unread_author1", "SecurePass1!")
	reviewerToken := loginUser(t, "unread_reviewer1", "SecurePass1!")
	adminToken := loginUser(t, "unread_admin1", "SecurePass1!")

	authorClient := authedClient(t, authorToken)
	reviewerClient := authedClient(t, reviewerToken)
	adminClient := authedClient(t, adminToken)

	// Check initial count for author
	req, _ := http.NewRequest(http.MethodGet,
		testServer.URL+"/messaging/notifications/unread-count", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := authorClient.Do(req)
	require.NoError(t, err)
	var before map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&before)
	resp.Body.Close()
	beforeCount := before["count"].(float64)

	// Create a draft and submit for review — triggers review notification to author on approve
	resourceID := createDraft(t, authorClient, "Notification Test Resource", "desc")
	version := getResourceVersion(t, resourceID)
	subResp, err := authorClient.PostForm(testServer.URL+"/resources/"+resourceID+"/submit",
		map[string][]string{"version": {fmt.Sprintf("%d", version)}})
	require.NoError(t, err)
	subResp.Body.Close()

	// Reviewer approves — triggers review_decision notification for author
	version = getResourceVersion(t, resourceID)
	appResp, err := reviewerClient.PostForm(testServer.URL+"/resources/"+resourceID+"/approve",
		map[string][]string{"version": {fmt.Sprintf("%d", version)}})
	require.NoError(t, err)
	appResp.Body.Close()

	// Admin publishes — triggers publish_complete notification for author
	version = getResourceVersion(t, resourceID)
	pubResp, err := adminClient.PostForm(testServer.URL+"/resources/"+resourceID+"/publish",
		map[string][]string{"version": {fmt.Sprintf("%d", version)}})
	require.NoError(t, err)
	pubResp.Body.Close()

	// Check count again for author — should have increased
	req2, _ := http.NewRequest(http.MethodGet,
		testServer.URL+"/messaging/notifications/unread-count", nil)
	req2.Header.Set("Accept", "application/json")
	resp2, err := authorClient.Do(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()

	assert.Equal(t, http.StatusOK, resp2.StatusCode)
	var after map[string]interface{}
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&after))
	afterCount := after["count"].(float64)
	assert.Greater(t, afterCount, beforeCount, "unread count should increase after notifications")
}
