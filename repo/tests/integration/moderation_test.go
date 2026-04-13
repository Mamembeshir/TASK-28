package integration_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/eduexchange/eduexchange/internal/cron"
	engagementrepo "github.com/eduexchange/eduexchange/internal/repository/engagement"
	gamificationrepo "github.com/eduexchange/eduexchange/internal/repository/gamification"
	gamificationservice "github.com/eduexchange/eduexchange/internal/service/gamification"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createModPublishedResource creates a published resource with a custom title for moderation tests.
func createModPublishedResource(t *testing.T, authorToken, adminToken, title string) string {
	t.Helper()
	authorClient := authedClient(t,authorToken)
	adminClient := authedClient(t,adminToken)

	// Create draft - use content_body (not content) to match the handler
	resp, err := authorClient.PostForm(testServer.URL+"/resources", url.Values{
		"title":        {title},
		"description":  {"A test resource description that is detailed"},
		"content_body": {"Content body here for the test resource"},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusSeeOther, resp.StatusCode, "expected redirect after resource creation")
	loc := resp.Header.Get("Location")
	// Extract resource ID from location: /resources/<id>
	resourceID := strings.TrimPrefix(loc, "/resources/")
	// Remove any trailing path
	if idx := strings.Index(resourceID, "/"); idx != -1 {
		resourceID = resourceID[:idx]
	}
	require.NotEmpty(t, resourceID, "resource ID from redirect must not be empty")
	resp.Body.Close()

	// Submit for review (use correct version from DB)
	version := getResourceVersion(t, resourceID)
	resp, err = authorClient.PostForm(testServer.URL+"/resources/"+resourceID+"/submit", url.Values{
		"version": {fmt.Sprintf("%d", version)},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusSeeOther, resp.StatusCode, "expected redirect after submit")
	resp.Body.Close()

	// Approve
	version = getResourceVersion(t, resourceID)
	resp, err = adminClient.PostForm(testServer.URL+"/resources/"+resourceID+"/approve", url.Values{
		"version": {fmt.Sprintf("%d", version)},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusSeeOther, resp.StatusCode, "expected redirect after approve")
	resp.Body.Close()

	// Publish
	version = getResourceVersion(t, resourceID)
	resp, err = adminClient.PostForm(testServer.URL+"/resources/"+resourceID+"/publish", url.Values{
		"version": {fmt.Sprintf("%d", version)},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusSeeOther, resp.StatusCode, "expected redirect after publish")
	resp.Body.Close()

	return resourceID
}

// ── TestCreateReport_Success ───────────────────────────────────────────────────

func TestCreateReport_Success(t *testing.T) {
	truncate(t)

	registerUser(t, "author_r1", "author_r1@example.com", "SecurePass1!")
	registerUser(t, "reporter_r1", "reporter_r1@example.com", "SecurePass1!")
	registerUser(t, "admin_r1", "admin_r1@example.com", "SecurePass1!")
	makeAuthor(t, "author_r1")
	makeAdmin(t, "admin_r1")

	authorToken := loginUser(t, "author_r1", "SecurePass1!")
	reporterToken := loginUser(t, "reporter_r1", "SecurePass1!")
	adminToken := loginUser(t, "admin_r1", "SecurePass1!")

	resourceID := createModPublishedResource(t, authorToken, adminToken, "Report Test Resource")

	reporterClient := authedClient(t,reporterToken)
	resp, err := reporterClient.PostForm(testServer.URL+"/reports", url.Values{
		"resource_id": {resourceID},
		"reason_type": {"SPAM"},
		"description": {"This resource is spam."},
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	// Verify in DB
	var count int
	err = testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM reports WHERE resource_id=$1 AND status='OPEN'`, resourceID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

// ── TestCreateReport_RequiresAuth ─────────────────────────────────────────────

func TestCreateReport_RequiresAuth(t *testing.T) {
	truncate(t)

	// Use publicClient so the request carries a valid CSRF token.
	// Without auth the server should reject with 302 (redirect to login) or 401.
	client := publicClient(t)

	resp, err := client.PostForm(testServer.URL+"/reports", url.Values{
		"resource_id": {uuid.New().String()},
		"reason_type": {"SPAM"},
		"description": {"test"},
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	// Unauthed → redirect to login (302) or 401
	assert.True(t, resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusUnauthorized,
		"expected 302 or 401, got %d", resp.StatusCode)
}

// ── TestAssignReport_Success ──────────────────────────────────────────────────

func TestAssignReport_Success(t *testing.T) {
	truncate(t)

	registerUser(t, "author_ar", "author_ar@example.com", "SecurePass1!")
	registerUser(t, "reviewer_ar", "reviewer_ar@example.com", "SecurePass1!")
	registerUser(t, "reporter_ar", "reporter_ar@example.com", "SecurePass1!")
	registerUser(t, "admin_ar", "admin_ar@example.com", "SecurePass1!")
	makeAuthor(t, "author_ar")
	makeReviewer(t, "reviewer_ar")
	makeAdmin(t, "admin_ar")

	authorToken := loginUser(t, "author_ar", "SecurePass1!")
	reviewerToken := loginUser(t, "reviewer_ar", "SecurePass1!")
	reporterToken := loginUser(t, "reporter_ar", "SecurePass1!")
	adminToken := loginUser(t, "admin_ar", "SecurePass1!")

	resourceID := createModPublishedResource(t, authorToken, adminToken, "Assign Report Resource")

	// Create a report
	reporterClient := authedClient(t,reporterToken)
	resp, err := reporterClient.PostForm(testServer.URL+"/reports", url.Values{
		"resource_id": {resourceID},
		"reason_type": {"INAPPROPRIATE"},
		"description": {"Inappropriate content"},
	})
	require.NoError(t, err)
	resp.Body.Close()

	// Get report ID
	var reportID string
	err = testPool.QueryRow(context.Background(),
		`SELECT id FROM reports WHERE resource_id=$1 ORDER BY created_at DESC LIMIT 1`, resourceID).
		Scan(&reportID)
	require.NoError(t, err)

	// Assign
	reviewerClient := authedClient(t,reviewerToken)
	resp, err = reviewerClient.PostForm(testServer.URL+"/moderation/reports/"+reportID+"/assign", url.Values{})
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify status changed to UNDER_REVIEW
	var status string
	err = testPool.QueryRow(context.Background(),
		`SELECT status FROM reports WHERE id=$1`, reportID).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "UNDER_REVIEW", status)
}

// ── TestResolveReport_WithTakedown ────────────────────────────────────────────

func TestResolveReport_WithTakedown(t *testing.T) {
	truncate(t)

	registerUser(t, "author_rr", "author_rr@example.com", "SecurePass1!")
	registerUser(t, "reviewer_rr", "reviewer_rr@example.com", "SecurePass1!")
	registerUser(t, "reporter_rr", "reporter_rr@example.com", "SecurePass1!")
	registerUser(t, "admin_rr", "admin_rr@example.com", "SecurePass1!")
	makeAuthor(t, "author_rr")
	makeReviewer(t, "reviewer_rr")
	makeAdmin(t, "admin_rr")

	authorToken := loginUser(t, "author_rr", "SecurePass1!")
	reviewerToken := loginUser(t, "reviewer_rr", "SecurePass1!")
	reporterToken := loginUser(t, "reporter_rr", "SecurePass1!")
	adminToken := loginUser(t, "admin_rr", "SecurePass1!")

	resourceID := createModPublishedResource(t, authorToken, adminToken, "Resolve Report Resource")

	// Create report
	reporterClient := authedClient(t,reporterToken)
	resp, err := reporterClient.PostForm(testServer.URL+"/reports", url.Values{
		"resource_id": {resourceID},
		"reason_type": {"COPYRIGHT"},
		"description": {"Copyright violation"},
	})
	require.NoError(t, err)
	resp.Body.Close()

	var reportID string
	err = testPool.QueryRow(context.Background(),
		`SELECT id FROM reports WHERE resource_id=$1 ORDER BY created_at DESC LIMIT 1`, resourceID).
		Scan(&reportID)
	require.NoError(t, err)

	// Assign
	reviewerClient := authedClient(t,reviewerToken)
	resp, err = reviewerClient.PostForm(testServer.URL+"/moderation/reports/"+reportID+"/assign", url.Values{})
	require.NoError(t, err)
	resp.Body.Close()

	// Takedown via moderation
	resp, err = reviewerClient.PostForm(testServer.URL+"/moderation/resources/"+resourceID+"/takedown", url.Values{
		"evidence": {"Copyright infringement confirmed"},
	})
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify resource is TAKEN_DOWN
	var statusStr string
	err = testPool.QueryRow(context.Background(),
		`SELECT status FROM resources WHERE id=$1`, resourceID).Scan(&statusStr)
	require.NoError(t, err)
	assert.Equal(t, "TAKEN_DOWN", statusStr)

	// Resolve report
	resp, err = reviewerClient.PostForm(testServer.URL+"/moderation/reports/"+reportID+"/resolve", url.Values{
		"action_type": {"TAKEDOWN"},
		"notes":       {"Resource taken down for copyright violation"},
		"evidence":    {"Copyright infringement confirmed"},
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify report is RESOLVED
	var reportStatus string
	err = testPool.QueryRow(context.Background(),
		`SELECT status FROM reports WHERE id=$1`, reportID).Scan(&reportStatus)
	require.NoError(t, err)
	assert.Equal(t, "RESOLVED", reportStatus)
}

// ── TestDismissReport_Success ─────────────────────────────────────────────────

func TestDismissReport_Success(t *testing.T) {
	truncate(t)

	registerUser(t, "author_dr", "author_dr@example.com", "SecurePass1!")
	registerUser(t, "reviewer_dr", "reviewer_dr@example.com", "SecurePass1!")
	registerUser(t, "reporter_dr", "reporter_dr@example.com", "SecurePass1!")
	registerUser(t, "admin_dr", "admin_dr@example.com", "SecurePass1!")
	makeAuthor(t, "author_dr")
	makeReviewer(t, "reviewer_dr")
	makeAdmin(t, "admin_dr")

	authorToken := loginUser(t, "author_dr", "SecurePass1!")
	reporterToken := loginUser(t, "reporter_dr", "SecurePass1!")
	reviewerToken := loginUser(t, "reviewer_dr", "SecurePass1!")
	adminToken := loginUser(t, "admin_dr", "SecurePass1!")

	resourceID := createModPublishedResource(t, authorToken, adminToken, "Dismiss Report Resource")

	// Create report
	reporterClient := authedClient(t,reporterToken)
	resp, err := reporterClient.PostForm(testServer.URL+"/reports", url.Values{
		"resource_id": {resourceID},
		"reason_type": {"OTHER"},
		"description": {"Not sure about this"},
	})
	require.NoError(t, err)
	resp.Body.Close()

	var reportID string
	err = testPool.QueryRow(context.Background(),
		`SELECT id FROM reports WHERE resource_id=$1 ORDER BY created_at DESC LIMIT 1`, resourceID).
		Scan(&reportID)
	require.NoError(t, err)

	// Assign
	reviewerClient := authedClient(t,reviewerToken)
	resp, err = reviewerClient.PostForm(testServer.URL+"/moderation/reports/"+reportID+"/assign", url.Values{})
	require.NoError(t, err)
	resp.Body.Close()

	// Dismiss
	resp, err = reviewerClient.PostForm(testServer.URL+"/moderation/reports/"+reportID+"/dismiss", url.Values{
		"notes": {"Report does not violate any policy"},
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify report is DISMISSED
	var reportStatus string
	err = testPool.QueryRow(context.Background(),
		`SELECT status FROM reports WHERE id=$1`, reportID).Scan(&reportStatus)
	require.NoError(t, err)
	assert.Equal(t, "DISMISSED", reportStatus)
}

// ── TestBanUser_1Day_CanBrowseNotPost ─────────────────────────────────────────

func TestBanUser_1Day_CanBrowseNotPost(t *testing.T) {
	truncate(t)

	registerUser(t, "target_ban", "target_ban@example.com", "SecurePass1!")
	registerUser(t, "admin_ban", "admin_ban@example.com", "SecurePass1!")
	makeAdmin(t, "admin_ban")
	makeAuthor(t, "target_ban")

	targetToken := loginUser(t, "target_ban", "SecurePass1!")
	adminToken := loginUser(t, "admin_ban", "SecurePass1!")

	// Get target user ID
	var targetUserID string
	err := testPool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE username='target_ban'`).Scan(&targetUserID)
	require.NoError(t, err)

	// Admin bans target user for 1 day
	adminClient := authedClient(t,adminToken)
	resp, err := adminClient.PostForm(testServer.URL+"/moderation/users/"+targetUserID+"/ban", url.Values{
		"ban_type": {"1_DAY"},
		"reason":   {"Testing 1-day ban"},
	})
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	targetClient := authedClient(t,targetToken)

	// GET /resources should work (200)
	resp, err = targetClient.Get(testServer.URL + "/resources")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// POST /resources should return 403 (temp-banned user cannot post)
	resp, err = targetClient.PostForm(testServer.URL+"/resources", url.Values{
		"title":       {"Banned user resource"},
		"description": {"This should be blocked"},
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// ── TestBanUser_PermanentRequiresAdmin ───────────────────────────────────────

func TestBanUser_PermanentRequiresAdmin(t *testing.T) {
	truncate(t)

	registerUser(t, "target_pban", "target_pban@example.com", "SecurePass1!")
	registerUser(t, "reviewer_pban", "reviewer_pban@example.com", "SecurePass1!")
	makeReviewer(t, "reviewer_pban")

	var targetUserID string
	err := testPool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE username='target_pban'`).Scan(&targetUserID)
	require.NoError(t, err)

	reviewerToken := loginUser(t, "reviewer_pban", "SecurePass1!")
	reviewerClient := authedClient(t,reviewerToken)

	// Reviewer tries PERMANENT ban → 403 (route requires ADMIN)
	resp, err := reviewerClient.PostForm(testServer.URL+"/moderation/users/"+targetUserID+"/ban", url.Values{
		"ban_type": {"PERMANENT"},
		"reason":   {"Should be rejected"},
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// ── TestRateLimit_21stPost_Returns429 ────────────────────────────────────────

func TestRateLimit_21stPost_Returns429(t *testing.T) {
	truncate(t)

	registerUser(t, "rl_author", "rl_author@example.com", "SecurePass1!")
	registerUser(t, "rl_admin", "rl_admin@example.com", "SecurePass1!")
	makeAuthor(t, "rl_author")
	makeAdmin(t, "rl_admin")

	authorToken := loginUser(t, "rl_author", "SecurePass1!")
	authorClient := authedClient(t,authorToken)

	// First 20 posts should succeed (get redirected, not 429)
	for i := 1; i <= 20; i++ {
		resp, err := authorClient.PostForm(testServer.URL+"/resources", url.Values{
			"title":       {fmt.Sprintf("Rate Limit Resource %d at %d", i, time.Now().UnixNano())},
			"description": {"A resource description that is long enough"},
			"content":     {"Content"},
		})
		require.NoError(t, err)
		resp.Body.Close()
		assert.NotEqual(t, http.StatusTooManyRequests, resp.StatusCode,
			"post %d should not be rate limited", i)
	}

	// 21st post should be rate limited
	resp, err := authorClient.PostForm(testServer.URL+"/resources", url.Values{
		"title":       {fmt.Sprintf("Rate Limit Resource 21 at %d", time.Now().UnixNano())},
		"description": {"A resource description that is long enough"},
		"content":     {"Content"},
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
}

// ── TestLikeRingDetection_FlagsCreated ───────────────────────────────────────

func TestLikeRingDetection_FlagsCreated(t *testing.T) {
	truncate(t)

	registerUser(t, "ring_a", "ring_a@example.com", "SecurePass1!")
	registerUser(t, "ring_b", "ring_b@example.com", "SecurePass1!")
	registerUser(t, "ring_admin", "ring_admin@example.com", "SecurePass1!")
	makeAuthor(t, "ring_a")
	makeAuthor(t, "ring_b")
	makeAdmin(t, "ring_admin")

	// Get user IDs
	var userAID, userBID string
	err := testPool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE username='ring_a'`).Scan(&userAID)
	require.NoError(t, err)
	err = testPool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE username='ring_b'`).Scan(&userBID)
	require.NoError(t, err)

	// Create 16 resources for user A
	resourceIDsA := make([]string, 16)
	for i := 0; i < 16; i++ {
		id := uuid.New()
		resourceIDsA[i] = id.String()
		_, err := testPool.Exec(context.Background(), `
			INSERT INTO resources (id, title, description, content_body, author_id, status, current_version_number, version, created_at, updated_at)
			VALUES ($1, $2, 'desc', 'body', $3, 'PUBLISHED', 1, 1, NOW(), NOW())`,
			id, fmt.Sprintf("Ring Resource A%d", i), userAID)
		require.NoError(t, err)
	}

	// Create 16 resources for user B
	resourceIDsB := make([]string, 16)
	for i := 0; i < 16; i++ {
		id := uuid.New()
		resourceIDsB[i] = id.String()
		_, err := testPool.Exec(context.Background(), `
			INSERT INTO resources (id, title, description, content_body, author_id, status, current_version_number, version, created_at, updated_at)
			VALUES ($1, $2, 'desc', 'body', $3, 'PUBLISHED', 1, 1, NOW(), NOW())`,
			id, fmt.Sprintf("Ring Resource B%d", i), userBID)
		require.NoError(t, err)
	}

	// Insert 16 votes: user B votes on user A's resources
	for i := 0; i < 16; i++ {
		_, err := testPool.Exec(context.Background(), `
			INSERT INTO votes (id, user_id, resource_id, vote_type, created_at, updated_at)
			VALUES ($1, $2, $3, 'UP', NOW(), NOW())`,
			uuid.New(), userBID, resourceIDsA[i])
		require.NoError(t, err)
	}

	// Insert 16 votes: user A votes on user B's resources
	for i := 0; i < 16; i++ {
		_, err := testPool.Exec(context.Background(), `
			INSERT INTO votes (id, user_id, resource_id, vote_type, created_at, updated_at)
			VALUES ($1, $2, $3, 'UP', NOW(), NOW())`,
			uuid.New(), userAID, resourceIDsB[i])
		require.NoError(t, err)
	}

	// Run the like-ring detection cron
	gamRepo := gamificationrepo.New(testPool)
	gamSvc := gamificationservice.NewRankingService(gamRepo)
	engRepo := engagementrepo.New(testPool)
	scheduler := cron.New(gamSvc, engRepo, testPool, nil, nil, nil, nil, nil)

	scheduler.RunLikeRingDetection()

	// Small wait for async ops
	time.Sleep(100 * time.Millisecond)

	// Verify anomaly flag was created
	var flagCount int
	err = testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM anomaly_flags WHERE flag_type='LIKE_RING' AND status='OPEN'`).Scan(&flagCount)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, flagCount, 1, "expected at least one LIKE_RING anomaly flag")
}

// ── TestLikeRingDetection_OneWay_NoFlag ─────────────────────────────────────
// Negative test: high-volume one-way votes (A→B only, no B→A) must NOT
// produce a LIKE_RING anomaly flag.

func TestLikeRingDetection_OneWay_NoFlag(t *testing.T) {
	truncate(t)

	registerUser(t, "oneway_a", "oneway_a@example.com", "SecurePass1!")
	registerUser(t, "oneway_b", "oneway_b@example.com", "SecurePass1!")
	makeAuthor(t, "oneway_b")

	var userAID, userBID string
	err := testPool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE username='oneway_a'`).Scan(&userAID)
	require.NoError(t, err)
	err = testPool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE username='oneway_b'`).Scan(&userBID)
	require.NoError(t, err)

	// Create 20 resources authored by user B
	for i := 0; i < 20; i++ {
		_, err := testPool.Exec(context.Background(), `
			INSERT INTO resources (id, title, description, content_body, author_id, status, current_version_number, version, created_at, updated_at)
			VALUES ($1, $2, 'desc', 'body', $3, 'PUBLISHED', 1, 1, NOW(), NOW())`,
			uuid.New(), fmt.Sprintf("OneWay Resource %d", i), userBID)
		require.NoError(t, err)
	}

	// User A votes on all 20 of B's resources — one-way only, B never votes on A
	rows, err := testPool.Query(context.Background(),
		`SELECT id FROM resources WHERE author_id = $1`, userBID)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var resID uuid.UUID
		require.NoError(t, rows.Scan(&resID))
		_, err := testPool.Exec(context.Background(), `
			INSERT INTO votes (id, user_id, resource_id, vote_type, created_at, updated_at)
			VALUES ($1, $2, $3, 'UP', NOW(), NOW())`,
			uuid.New(), userAID, resID)
		require.NoError(t, err)
	}

	// Run like-ring detection
	gamRepo := gamificationrepo.New(testPool)
	gamSvc := gamificationservice.NewRankingService(gamRepo)
	engRepo := engagementrepo.New(testPool)
	scheduler := cron.New(gamSvc, engRepo, testPool, nil, nil, nil, nil, nil)
	scheduler.RunLikeRingDetection()

	// Assert zero LIKE_RING flags — one-way voting must not be flagged
	var flagCount int
	err = testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM anomaly_flags WHERE flag_type='LIKE_RING'`).Scan(&flagCount)
	require.NoError(t, err)
	assert.Equal(t, 0, flagCount, "one-way voting pattern must not produce a LIKE_RING flag")
}

// ─── Validation error regression tests ───────────────────────────────────────
// These tests guard against the recursive handleServiceError bug (Finding 1).
// An ErrValidation must return 422 Unprocessable Entity, never cause a stack
// overflow or return a 500.

// TestBanUser_InvalidBanType_Returns422 verifies that banning a user with an
// invalid ban_type value returns 422 (not 500 or a stack overflow).
func TestBanUser_InvalidBanType_Returns422(t *testing.T) {
	truncate(t)

	registerUser(t, "mod_val_admin", "mod_val_admin@example.com", "SecurePass1!")
	registerUser(t, "mod_val_target", "mod_val_target@example.com", "SecurePass1!")
	makeAdmin(t, "mod_val_admin")

	adminToken := loginUser(t, "mod_val_admin", "SecurePass1!")

	var targetID string
	err := testPool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE username = 'mod_val_target'`).Scan(&targetID)
	require.NoError(t, err)

	resp, err := authedClient(t,adminToken).PostForm(
		testServer.URL+"/moderation/users/"+targetID+"/ban",
		url.Values{
			"ban_type": {"INVALID_TYPE"}, // triggers ErrValidation
			"reason":   {"testing"},
		},
	)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode,
		"invalid ban_type must return 422, not 500 or stack overflow")
}
