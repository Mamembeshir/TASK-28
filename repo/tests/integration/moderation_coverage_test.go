package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── GET /moderation/reports ─────────────────────────────────────────────────

func TestListModerationReports_AsReviewer_Returns200(t *testing.T) {
	truncate(t)
	registerUser(t, "modrep_reviewer1", "modrep_reviewer1@example.com", "SecurePass1!")
	makeReviewer(t, "modrep_reviewer1")
	token := loginUser(t, "modrep_reviewer1", "SecurePass1!")

	req, _ := http.NewRequest(http.MethodGet, testServer.URL+"/moderation/reports", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := authedClient(t, token).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Contains(t, body, "reports")
}

func TestListModerationReports_AsRegularUser_Forbidden(t *testing.T) {
	truncate(t)
	registerUser(t, "modrep_regular1", "modrep_regular1@example.com", "SecurePass1!")
	token := loginUser(t, "modrep_regular1", "SecurePass1!")

	resp, err := authedClient(t, token).Get(testServer.URL + "/moderation/reports")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestListModerationReports_WithExistingReport_ReturnsIt(t *testing.T) {
	truncate(t)
	registerUser(t, "modrep_author2", "modrep_author2@example.com", "SecurePass1!")
	registerUser(t, "modrep_reporter2", "modrep_reporter2@example.com", "SecurePass1!")
	registerUser(t, "modrep_reviewer2", "modrep_reviewer2@example.com", "SecurePass1!")
	registerUser(t, "modrep_admin2", "modrep_admin2@example.com", "SecurePass1!")
	makeAuthor(t, "modrep_author2")
	makeReviewer(t, "modrep_reviewer2")
	makeAdmin(t, "modrep_admin2")

	authorToken := loginUser(t, "modrep_author2", "SecurePass1!")
	reporterToken := loginUser(t, "modrep_reporter2", "SecurePass1!")
	reviewerToken := loginUser(t, "modrep_reviewer2", "SecurePass1!")
	adminToken := loginUser(t, "modrep_admin2", "SecurePass1!")

	resourceID := createModPublishedResource(t, authorToken, adminToken, "Report List Test Resource")

	// Create a report
	reportResp, err := authedClient(t, reporterToken).PostForm(testServer.URL+"/reports", url.Values{
		"resource_id": {resourceID},
		"reason_type": {"SPAM"},
		"description": {"This is spam"},
	})
	require.NoError(t, err)
	reportResp.Body.Close()

	// List reports as reviewer
	req, _ := http.NewRequest(http.MethodGet, testServer.URL+"/moderation/reports", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := authedClient(t, reviewerToken).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	reports := body["reports"].([]interface{})
	assert.GreaterOrEqual(t, len(reports), 1)
}

// ─── GET /moderation/reports/:id ─────────────────────────────────────────────

func TestGetModerationReportDetail_AsReviewer_Returns200(t *testing.T) {
	truncate(t)
	registerUser(t, "repdet_author1", "repdet_author1@example.com", "SecurePass1!")
	registerUser(t, "repdet_reporter1", "repdet_reporter1@example.com", "SecurePass1!")
	registerUser(t, "repdet_reviewer1", "repdet_reviewer1@example.com", "SecurePass1!")
	registerUser(t, "repdet_admin1", "repdet_admin1@example.com", "SecurePass1!")
	makeAuthor(t, "repdet_author1")
	makeReviewer(t, "repdet_reviewer1")
	makeAdmin(t, "repdet_admin1")

	authorToken := loginUser(t, "repdet_author1", "SecurePass1!")
	reporterToken := loginUser(t, "repdet_reporter1", "SecurePass1!")
	reviewerToken := loginUser(t, "repdet_reviewer1", "SecurePass1!")
	adminToken := loginUser(t, "repdet_admin1", "SecurePass1!")

	resourceID := createModPublishedResource(t, authorToken, adminToken, "Detail Test Resource")

	reportResp, err := authedClient(t, reporterToken).PostForm(testServer.URL+"/reports", url.Values{
		"resource_id": {resourceID},
		"reason_type": {"INAPPROPRIATE"},
		"description": {"Inappropriate content"},
	})
	require.NoError(t, err)
	reportResp.Body.Close()

	var reportID string
	err = testPool.QueryRow(context.Background(),
		`SELECT id FROM reports WHERE resource_id=$1 ORDER BY created_at DESC LIMIT 1`, resourceID).
		Scan(&reportID)
	require.NoError(t, err)

	req, _ := http.NewRequest(http.MethodGet,
		testServer.URL+"/moderation/reports/"+reportID, nil)
	req.Header.Set("Accept", "application/json")
	resp, err := authedClient(t, reviewerToken).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestGetModerationReportDetail_AsRegularUser_Forbidden(t *testing.T) {
	truncate(t)
	registerUser(t, "repdet_regular1", "repdet_regular1@example.com", "SecurePass1!")
	token := loginUser(t, "repdet_regular1", "SecurePass1!")

	req, _ := http.NewRequest(http.MethodGet,
		testServer.URL+"/moderation/reports/"+uuid.New().String(), nil)
	resp, err := authedClient(t, token).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// ─── GET /moderation/anomalies ────────────────────────────────────────────────

func TestListAnomalies_AsReviewer_Returns200(t *testing.T) {
	truncate(t)
	registerUser(t, "anom_reviewer1", "anom_reviewer1@example.com", "SecurePass1!")
	makeReviewer(t, "anom_reviewer1")
	token := loginUser(t, "anom_reviewer1", "SecurePass1!")

	req, _ := http.NewRequest(http.MethodGet, testServer.URL+"/moderation/anomalies", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := authedClient(t, token).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Contains(t, body, "flags")
}

func TestListAnomalies_AsRegularUser_Forbidden(t *testing.T) {
	truncate(t)
	registerUser(t, "anom_regular1", "anom_regular1@example.com", "SecurePass1!")
	token := loginUser(t, "anom_regular1", "SecurePass1!")

	resp, err := authedClient(t, token).Get(testServer.URL + "/moderation/anomalies")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// ─── POST /moderation/anomalies/:id/review ───────────────────────────────────

func TestReviewAnomaly_AsReviewer_Updates(t *testing.T) {
	truncate(t)
	registerUser(t, "anomrev_reviewer1", "anomrev_reviewer1@example.com", "SecurePass1!")
	makeReviewer(t, "anomrev_reviewer1")
	token := loginUser(t, "anomrev_reviewer1", "SecurePass1!")

	// Get the reviewer's user ID to include in user_ids array
	var reviewerID string
	testPool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE username='anomrev_reviewer1'`).Scan(&reviewerID)
	require.NotEmpty(t, reviewerID)

	// Insert an anomaly flag directly into the DB
	flagID := uuid.New()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO anomaly_flags
		 (id, flag_type, user_ids, evidence_json, status, created_at, updated_at)
		 VALUES ($1, 'LIKE_RING', ARRAY[$2::uuid], '{"type":"like_ring","count":50}'::jsonb, 'OPEN', NOW(), NOW())`,
		flagID, reviewerID,
	)
	require.NoError(t, err)

	resp, err := authedClient(t, token).PostForm(
		testServer.URL+"/moderation/anomalies/"+flagID.String()+"/review",
		url.Values{"decision": {"DISMISSED"}},
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "DISMISSED", body["status"])
}

func TestReviewAnomaly_AsRegularUser_Forbidden(t *testing.T) {
	truncate(t)
	registerUser(t, "anomrev_regular1", "anomrev_regular1@example.com", "SecurePass1!")
	token := loginUser(t, "anomrev_regular1", "SecurePass1!")

	resp, err := authedClient(t, token).PostForm(
		testServer.URL+"/moderation/anomalies/"+uuid.New().String()+"/review",
		url.Values{"decision": {"DISMISSED"}},
	)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// ─── POST /moderation/resources/:id/restore ──────────────────────────────────

func TestRestoreResource_AsAdmin_RestoresTakenDownResource(t *testing.T) {
	truncate(t)
	registerUser(t, "restore_author1", "restore_author1@example.com", "SecurePass1!")
	registerUser(t, "restore_reviewer1", "restore_reviewer1@example.com", "SecurePass1!")
	registerUser(t, "restore_admin1", "restore_admin1@example.com", "SecurePass1!")
	makeAuthor(t, "restore_author1")
	makeReviewer(t, "restore_reviewer1")
	makeAdmin(t, "restore_admin1")

	authorToken := loginUser(t, "restore_author1", "SecurePass1!")
	reviewerToken := loginUser(t, "restore_reviewer1", "SecurePass1!")
	adminToken := loginUser(t, "restore_admin1", "SecurePass1!")
	adminClient := authedClient(t, adminToken)

	resourceID := createModPublishedResource(t, authorToken, adminToken, "Resource To Restore")

	// Take it down via moderation
	takedownResp, err := authedClient(t, reviewerToken).PostForm(
		testServer.URL+"/moderation/resources/"+resourceID+"/takedown",
		url.Values{"evidence": {"Policy violation confirmed"}},
	)
	require.NoError(t, err)
	takedownResp.Body.Close()
	require.Equal(t, http.StatusOK, takedownResp.StatusCode)

	// Verify TAKEN_DOWN
	var statusStr string
	testPool.QueryRow(context.Background(),
		`SELECT status FROM resources WHERE id=$1`, resourceID).Scan(&statusStr)
	require.Equal(t, "TAKEN_DOWN", statusStr)

	// Restore as admin via moderation endpoint
	resp, err := adminClient.PostForm(
		testServer.URL+"/moderation/resources/"+resourceID+"/restore",
		url.Values{},
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "restored", body["status"])

	// Verify resource is back to PUBLISHED
	testPool.QueryRow(context.Background(),
		`SELECT status FROM resources WHERE id=$1`, resourceID).Scan(&statusStr)
	assert.Equal(t, "PUBLISHED", statusStr)
}

func TestRestoreResource_AsReviewer_Forbidden(t *testing.T) {
	truncate(t)
	registerUser(t, "restore_reviewer2", "restore_reviewer2@example.com", "SecurePass1!")
	makeReviewer(t, "restore_reviewer2")
	token := loginUser(t, "restore_reviewer2", "SecurePass1!")

	resp, err := authedClient(t, token).PostForm(
		testServer.URL+"/moderation/resources/"+uuid.New().String()+"/restore",
		url.Values{},
	)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// ─── POST /moderation/users/:id/unban ────────────────────────────────────────

func TestUnbanUser_AsAdmin_UnbansUser(t *testing.T) {
	truncate(t)
	registerUser(t, "unban_target1", "unban_target1@example.com", "SecurePass1!")
	registerUser(t, "unban_admin1", "unban_admin1@example.com", "SecurePass1!")
	makeAdmin(t, "unban_admin1")

	adminToken := loginUser(t, "unban_admin1", "SecurePass1!")
	adminClient := authedClient(t, adminToken)

	var targetUserID string
	err := testPool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE username='unban_target1'`).Scan(&targetUserID)
	require.NoError(t, err)

	// Ban the user first
	banResp, err := adminClient.PostForm(
		testServer.URL+"/moderation/users/"+targetUserID+"/ban",
		url.Values{
			"ban_type": {"1_DAY"},
			"reason":   {"Testing ban/unban cycle"},
		},
	)
	require.NoError(t, err)
	banResp.Body.Close()
	require.Equal(t, http.StatusOK, banResp.StatusCode)

	// Verify user is banned
	var banCount int
	testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM user_bans WHERE user_id=$1 AND lifted_at IS NULL`, targetUserID).
		Scan(&banCount)
	assert.Equal(t, 1, banCount)

	// Unban the user
	resp, err := adminClient.PostForm(
		testServer.URL+"/moderation/users/"+targetUserID+"/unban",
		url.Values{},
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "unbanned", body["status"])

	// Verify ban is lifted
	testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM user_bans WHERE user_id=$1 AND lifted_at IS NULL`, targetUserID).
		Scan(&banCount)
	assert.Equal(t, 0, banCount)
}

func TestUnbanUser_AsRegularUser_Forbidden(t *testing.T) {
	truncate(t)
	registerUser(t, "unban_regular1", "unban_regular1@example.com", "SecurePass1!")
	token := loginUser(t, "unban_regular1", "SecurePass1!")

	resp, err := authedClient(t, token).PostForm(
		testServer.URL+"/moderation/users/"+uuid.New().String()+"/unban",
		url.Values{},
	)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}
