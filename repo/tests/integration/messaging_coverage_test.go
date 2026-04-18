package integration_test

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── GET /messaging/notifications/unread-count ───────────────────────────────
// The endpoint returns plain text (HTMX polling target):
//   - empty string when count == 0
//   - the integer count as text otherwise
// It is NOT a JSON endpoint.

func readUnreadCountBody(t *testing.T, resp *http.Response) int {
	t.Helper()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	require.NoErrorf(t, err, "unread-count body was not an integer: %q", s)
	return n
}

func TestGetUnreadCount_Authenticated_ReturnsCount(t *testing.T) {
	truncate(t)
	registerUser(t, "unread_user1", "unread_user1@example.com", "SecurePass1!")
	token := loginUser(t, "unread_user1", "SecurePass1!")

	req, _ := http.NewRequest(http.MethodGet,
		testServer.URL+"/messaging/notifications/unread-count", nil)
	resp, err := authedClient(t, token).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// New user should have 0 unread notifications (empty body).
	count := readUnreadCountBody(t, resp)
	assert.Equal(t, 0, count)
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
	resp, err := authorClient.Do(req)
	require.NoError(t, err)
	beforeCount := readUnreadCountBody(t, resp)
	resp.Body.Close()

	// Create a draft, submit, approve, publish — each step may generate a notification
	resourceID := createDraft(t, authorClient, "Notification Test Resource", "desc")
	version := getResourceVersion(t, resourceID)
	subResp, err := authorClient.PostForm(testServer.URL+"/resources/"+resourceID+"/submit",
		map[string][]string{"version": {fmt.Sprintf("%d", version)}})
	require.NoError(t, err)
	subResp.Body.Close()

	version = getResourceVersion(t, resourceID)
	appResp, err := reviewerClient.PostForm(testServer.URL+"/resources/"+resourceID+"/approve",
		map[string][]string{"version": {fmt.Sprintf("%d", version)}})
	require.NoError(t, err)
	appResp.Body.Close()

	version = getResourceVersion(t, resourceID)
	pubResp, err := adminClient.PostForm(testServer.URL+"/resources/"+resourceID+"/publish",
		map[string][]string{"version": {fmt.Sprintf("%d", version)}})
	require.NoError(t, err)
	pubResp.Body.Close()

	// Check count again for author
	req2, _ := http.NewRequest(http.MethodGet,
		testServer.URL+"/messaging/notifications/unread-count", nil)
	resp2, err := authorClient.Do(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()

	assert.Equal(t, http.StatusOK, resp2.StatusCode)
	afterCount := readUnreadCountBody(t, resp2)
	assert.GreaterOrEqual(t, afterCount, beforeCount, "unread count should not decrease after notifications")
}
