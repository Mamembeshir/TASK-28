package integration_test

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── Full workflow: DRAFT → SUBMIT → APPROVE → PUBLISH ───────────────────────

func TestCatalog_DraftToPublish_FullFlow(t *testing.T) {
	truncate(t)

	// Setup users
	registerUser(t, "author1", "author1@example.com", "SecurePass1!")
	registerUser(t, "reviewer1", "reviewer1@example.com", "SecurePass1!")
	registerUser(t, "admin1", "admin1@example.com", "SecurePass1!")
	makeAuthor(t, "author1")
	makeReviewer(t, "reviewer1")
	makeAdmin(t, "admin1")

	authorToken := loginUser(t, "author1", "SecurePass1!")
	reviewerToken := loginUser(t, "reviewer1", "SecurePass1!")
	adminToken := loginUser(t, "admin1", "SecurePass1!")

	authorClient := authedClient(authorToken)
	reviewerClient := authedClient(reviewerToken)
	adminClient := authedClient(adminToken)

	// 1. Create draft
	resp, err := authorClient.PostForm(testServer.URL+"/resources", url.Values{
		"title":       {"Introduction to Go"},
		"description": {"A beginner guide to Go programming."},
		"content_body": {"Go is a statically typed language…"},
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusSeeOther, resp.StatusCode, "create draft should redirect")

	resourceURL := resp.Header.Get("Location")
	require.NotEmpty(t, resourceURL, "expected redirect to resource detail")

	// Extract resource ID from location: /resources/{id}
	parts := strings.Split(strings.TrimPrefix(resourceURL, "/resources/"), "/")
	resourceID := parts[0]
	require.NotEmpty(t, resourceID)

	// 2. Get resource — verify DRAFT
	resp2, err := authorClient.Get(testServer.URL + "/resources/" + resourceID)
	require.NoError(t, err)
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusOK, resp2.StatusCode)

	// 3. Submit for review — need version; fetch from DB
	version := getResourceVersion(t, resourceID)
	resp3, err := authorClient.PostForm(testServer.URL+"/resources/"+resourceID+"/submit", url.Values{
		"version": {fmt.Sprintf("%d", version)},
	})
	require.NoError(t, err)
	defer resp3.Body.Close()
	assert.Equal(t, http.StatusSeeOther, resp3.StatusCode)

	// Verify PENDING_REVIEW in DB
	assert.Equal(t, "PENDING_REVIEW", getResourceStatus(t, resourceID))

	// 4. Approve
	version = getResourceVersion(t, resourceID)
	resp4, err := reviewerClient.PostForm(testServer.URL+"/resources/"+resourceID+"/approve", url.Values{
		"version": {fmt.Sprintf("%d", version)},
	})
	require.NoError(t, err)
	defer resp4.Body.Close()
	assert.Equal(t, http.StatusSeeOther, resp4.StatusCode)
	assert.Equal(t, "APPROVED", getResourceStatus(t, resourceID))

	// 5. Publish
	version = getResourceVersion(t, resourceID)
	resp5, err := adminClient.PostForm(testServer.URL+"/resources/"+resourceID+"/publish", url.Values{
		"version": {fmt.Sprintf("%d", version)},
	})
	require.NoError(t, err)
	defer resp5.Body.Close()
	assert.Equal(t, http.StatusSeeOther, resp5.StatusCode)
	assert.Equal(t, "PUBLISHED", getResourceStatus(t, resourceID))
}

// ─── Reject → Revise → Resubmit ──────────────────────────────────────────────

func TestCatalog_RejectReviseResubmit(t *testing.T) {
	truncate(t)

	registerUser(t, "auth2", "auth2@example.com", "SecurePass1!")
	registerUser(t, "rev2", "rev2@example.com", "SecurePass1!")
	makeAuthor(t, "auth2")
	makeReviewer(t, "rev2")

	authorToken := loginUser(t, "auth2", "SecurePass1!")
	reviewerToken := loginUser(t, "rev2", "SecurePass1!")
	authorClient := authedClient(authorToken)
	reviewerClient := authedClient(reviewerToken)

	// Create + submit
	resourceID := createAndSubmitDraft(t, authorClient, "Needs Work", "Some content")

	// Reject
	version := getResourceVersion(t, resourceID)
	resp, err := reviewerClient.PostForm(testServer.URL+"/resources/"+resourceID+"/reject", url.Values{
		"version": {fmt.Sprintf("%d", version)},
		"notes":   {"Needs more detail."},
	})
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusSeeOther, resp.StatusCode)
	assert.Equal(t, "REJECTED", getResourceStatus(t, resourceID))

	// Revise — puts back to DRAFT
	version = getResourceVersion(t, resourceID)
	resp2, err := authorClient.PostForm(testServer.URL+"/resources/"+resourceID+"/revise", url.Values{
		"version": {fmt.Sprintf("%d", version)},
	})
	require.NoError(t, err)
	resp2.Body.Close()
	assert.Equal(t, http.StatusSeeOther, resp2.StatusCode)
	assert.Equal(t, "DRAFT", getResourceStatus(t, resourceID))

	// Resubmit
	version = getResourceVersion(t, resourceID)
	resp3, err := authorClient.PostForm(testServer.URL+"/resources/"+resourceID+"/submit", url.Values{
		"version": {fmt.Sprintf("%d", version)},
	})
	require.NoError(t, err)
	resp3.Body.Close()
	assert.Equal(t, http.StatusSeeOther, resp3.StatusCode)
	assert.Equal(t, "PENDING_REVIEW", getResourceStatus(t, resourceID))
}

// ─── EditPublished: new version, stays PENDING_REVIEW ─────────────────────────

func TestCatalog_EditPublished_CreatesNewVersion(t *testing.T) {
	truncate(t)

	registerUser(t, "auth3", "auth3@example.com", "SecurePass1!")
	registerUser(t, "rev3", "rev3@example.com", "SecurePass1!")
	registerUser(t, "adm3", "adm3@example.com", "SecurePass1!")
	makeAuthor(t, "auth3")
	makeReviewer(t, "rev3")
	makeAdmin(t, "adm3")

	authorToken := loginUser(t, "auth3", "SecurePass1!")
	reviewerToken := loginUser(t, "rev3", "SecurePass1!")
	adminToken := loginUser(t, "adm3", "SecurePass1!")

	authorClient := authedClient(authorToken)
	reviewerClient := authedClient(reviewerToken)
	adminClient := authedClient(adminToken)

	// Full publish
	resourceID := createAndSubmitDraft(t, authorClient, "Published Resource", "Great content")
	approveAndPublish(t, reviewerClient, adminClient, resourceID)
	assert.Equal(t, "PUBLISHED", getResourceStatus(t, resourceID))

	// Edit published — triggers PUT (handler uses EditPublished for PUBLISHED)
	version := getResourceVersion(t, resourceID)
	req, _ := http.NewRequest(http.MethodPut, testServer.URL+"/resources/"+resourceID, strings.NewReader(url.Values{
		"title":       {"Published Resource (Updated)"},
		"description": {"Updated description."},
		"version":     {fmt.Sprintf("%d", version)},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(sessionCookie(authorToken))
	resp, err := (&http.Client{CheckRedirect: func(r *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}).Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusSeeOther, resp.StatusCode)

	// Status should now be PENDING_REVIEW (old published → new version under review)
	assert.Equal(t, "PENDING_REVIEW", getResourceStatus(t, resourceID))
}

// ─── Takedown / Restore ───────────────────────────────────────────────────────

func TestCatalog_TakedownAndRestore(t *testing.T) {
	truncate(t)

	registerUser(t, "auth4", "auth4@example.com", "SecurePass1!")
	registerUser(t, "rev4", "rev4@example.com", "SecurePass1!")
	registerUser(t, "adm4", "adm4@example.com", "SecurePass1!")
	makeAuthor(t, "auth4")
	makeReviewer(t, "rev4")
	makeAdmin(t, "adm4")

	authorToken := loginUser(t, "auth4", "SecurePass1!")
	reviewerToken := loginUser(t, "rev4", "SecurePass1!")
	adminToken := loginUser(t, "adm4", "SecurePass1!")

	authorClient := authedClient(authorToken)
	reviewerClient := authedClient(reviewerToken)
	adminClient := authedClient(adminToken)

	resourceID := createAndSubmitDraft(t, authorClient, "Take Me Down", "Content")
	approveAndPublish(t, reviewerClient, adminClient, resourceID)

	// Takedown
	version := getResourceVersion(t, resourceID)
	resp, err := adminClient.PostForm(testServer.URL+"/resources/"+resourceID+"/takedown", url.Values{
		"version": {fmt.Sprintf("%d", version)},
		"reason":  {"Policy violation"},
	})
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusSeeOther, resp.StatusCode)
	assert.Equal(t, "TAKEN_DOWN", getResourceStatus(t, resourceID))

	// Restore
	version = getResourceVersion(t, resourceID)
	resp2, err := adminClient.PostForm(testServer.URL+"/resources/"+resourceID+"/restore", url.Values{
		"version": {fmt.Sprintf("%d", version)},
	})
	require.NoError(t, err)
	resp2.Body.Close()
	assert.Equal(t, http.StatusSeeOther, resp2.StatusCode)
	assert.Equal(t, "PUBLISHED", getResourceStatus(t, resourceID))
}

// ─── File upload: MIME validation ─────────────────────────────────────────────

func TestCatalog_FileUpload_ValidPDF(t *testing.T) {
	truncate(t)

	registerUser(t, "auth5", "auth5@example.com", "SecurePass1!")
	makeAuthor(t, "auth5")
	authorToken := loginUser(t, "auth5", "SecurePass1!")
	authorClient := authedClient(authorToken)

	resourceID := createDraft(t, authorClient, "File Test", "")

	// Upload a minimal valid PDF
	pdfBytes := minimalPDFBytes()
	body, ct := multipartFile("file", "test.pdf", pdfBytes)
	req, _ := http.NewRequest(http.MethodPost, testServer.URL+"/resources/"+resourceID+"/files", body)
	req.Header.Set("Content-Type", ct)
	req.AddCookie(sessionCookie(authorToken))
	resp, err := (&http.Client{}).Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
}

func TestCatalog_FileUpload_InvalidMIME(t *testing.T) {
	truncate(t)

	registerUser(t, "auth6", "auth6@example.com", "SecurePass1!")
	makeAuthor(t, "auth6")
	authorToken := loginUser(t, "auth6", "SecurePass1!")
	authorClient := authedClient(authorToken)
	_ = authorClient

	resourceID := createDraft(t, authedClient(authorToken), "MIME Test", "")

	// Upload a plain text file (not allowed)
	body, ct := multipartFile("file", "test.txt", []byte("hello world"))
	req, _ := http.NewRequest(http.MethodPost, testServer.URL+"/resources/"+resourceID+"/files", body)
	req.Header.Set("Content-Type", ct)
	req.AddCookie(sessionCookie(authorToken))
	resp, err := (&http.Client{}).Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

// ─── Roles: 403 enforcement ───────────────────────────────────────────────────

func TestCatalog_Approve_Requires_ReviewerOrAdmin(t *testing.T) {
	truncate(t)

	registerUser(t, "auth7", "auth7@example.com", "SecurePass1!")
	registerUser(t, "reg7", "reg7@example.com", "SecurePass1!")
	makeAuthor(t, "auth7")
	// reg7 is just a regular user

	authorToken := loginUser(t, "auth7", "SecurePass1!")
	regularToken := loginUser(t, "reg7", "SecurePass1!")

	resourceID := createAndSubmitDraft(t, authedClient(authorToken), "Protected", "Content")

	version := getResourceVersion(t, resourceID)
	resp, err := authedClient(regularToken).PostForm(testServer.URL+"/resources/"+resourceID+"/approve", url.Values{
		"version": {fmt.Sprintf("%d", version)},
	})
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestCatalog_Publish_Requires_Admin(t *testing.T) {
	truncate(t)

	registerUser(t, "auth8", "auth8@example.com", "SecurePass1!")
	registerUser(t, "rev8", "rev8@example.com", "SecurePass1!")
	makeAuthor(t, "auth8")
	makeReviewer(t, "rev8")

	authorToken := loginUser(t, "auth8", "SecurePass1!")
	reviewerToken := loginUser(t, "rev8", "SecurePass1!")

	authorClient := authedClient(authorToken)
	reviewerClient := authedClient(reviewerToken)

	resourceID := createAndSubmitDraft(t, authorClient, "Need Admin", "Content")

	// Approve (reviewer can do this)
	version := getResourceVersion(t, resourceID)
	reviewerClient.PostForm(testServer.URL+"/resources/"+resourceID+"/approve", url.Values{
		"version": {fmt.Sprintf("%d", version)},
	})

	// Reviewer tries to publish — must be 403
	version = getResourceVersion(t, resourceID)
	resp, err := reviewerClient.PostForm(testServer.URL+"/resources/"+resourceID+"/publish", url.Values{
		"version": {fmt.Sprintf("%d", version)},
	})
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// ─── Rate limit (MOD-01): 20 creates per clock-hour ──────────────────────────

func TestCatalog_CreateDraft_RateLimit(t *testing.T) {
	truncate(t)

	registerUser(t, "ratelim", "ratelim@example.com", "SecurePass1!")
	makeAuthor(t, "ratelim")
	token := loginUser(t, "ratelim", "SecurePass1!")
	client := authedClient(token)

	// Manually insert 20 rate limit counter entries to simulate hitting the limit.
	ctx := context.Background()
	_, err := testPool.Exec(ctx,
		`INSERT INTO rate_limit_counters (user_id, action_type, window_start, count)
		 SELECT u.id, 'resource_create', date_trunc('hour', NOW()), 20
		 FROM users u WHERE u.username = 'ratelim'
		 ON CONFLICT (user_id, action_type, window_start) DO UPDATE SET count = 20`,
	)
	require.NoError(t, err)

	// Next create should be rate-limited
	resp, err := client.PostForm(testServer.URL+"/resources", url.Values{
		"title": {"Should Fail"},
	})
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
}

// ─── Idempotent create (no double-submit on reload) ───────────────────────────

func TestCatalog_CreateDraft_UniquePerSubmit(t *testing.T) {
	truncate(t)

	registerUser(t, "idem1", "idem1@example.com", "SecurePass1!")
	makeAuthor(t, "idem1")
	token := loginUser(t, "idem1", "SecurePass1!")
	client := authedClient(token)

	// Two separate creates with same title should each succeed (no uniqueness constraint on title)
	resp1, err := client.PostForm(testServer.URL+"/resources", url.Values{
		"title": {"Same Title"},
	})
	require.NoError(t, err)
	resp1.Body.Close()
	assert.Equal(t, http.StatusSeeOther, resp1.StatusCode)

	resp2, err := client.PostForm(testServer.URL+"/resources", url.Values{
		"title": {"Same Title"},
	})
	require.NoError(t, err)
	resp2.Body.Close()
	assert.Equal(t, http.StatusSeeOther, resp2.StatusCode)

	// Two distinct resources should exist
	var count int
	testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM resources WHERE title = 'Same Title'`).Scan(&count)
	assert.Equal(t, 2, count)
}

// ─── Optimistic locking: stale version → 409/422 ─────────────────────────────

func TestCatalog_StaleVersion_Rejected(t *testing.T) {
	truncate(t)

	registerUser(t, "stale1", "stale1@example.com", "SecurePass1!")
	makeAuthor(t, "stale1")
	token := loginUser(t, "stale1", "SecurePass1!")
	client := authedClient(token)

	resourceID := createDraft(t, client, "Stale Test", "Content")

	// Submit with version 0 (already at version 1 in DB)
	req, _ := http.NewRequest(http.MethodPut, testServer.URL+"/resources/"+resourceID, strings.NewReader(url.Values{
		"title":   {"Updated Title"},
		"version": {"0"}, // stale
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(sessionCookie(token))
	resp, err := (&http.Client{CheckRedirect: func(r *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}).Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	// Should fail with 4xx (stale version)
	assert.True(t, resp.StatusCode >= 400, "expected 4xx for stale version, got %d", resp.StatusCode)
}

// ─── Bulk Import ──────────────────────────────────────────────────────────────

func TestCatalog_BulkImport_ValidCSV(t *testing.T) {
	truncate(t)

	registerUser(t, "adm_imp", "adm_imp@example.com", "SecurePass1!")
	makeAdmin(t, "adm_imp")
	token := loginUser(t, "adm_imp", "SecurePass1!")

	csv := "title,category,tags\nGo Basics,,programming\nGo Advanced,,programming,advanced\n"
	body, ct := multipartFile("file", "import.csv", []byte(csv))
	req, _ := http.NewRequest(http.MethodPost, testServer.URL+"/import/upload", body)
	req.Header.Set("Content-Type", ct)
	req.AddCookie(sessionCookie(token))
	resp, err := (&http.Client{CheckRedirect: func(r *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}).Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusSeeOther, resp.StatusCode)

	// Follow redirect to preview page
	previewURL := testServer.URL + resp.Header.Get("Location")
	resp2, err := authedClient(token).Get(previewURL)
	require.NoError(t, err)
	resp2.Body.Close()
	assert.Equal(t, http.StatusOK, resp2.StatusCode)
}

func TestCatalog_BulkImport_InvalidRows(t *testing.T) {
	truncate(t)

	registerUser(t, "adm_imp2", "adm_imp2@example.com", "SecurePass1!")
	makeAdmin(t, "adm_imp2")
	token := loginUser(t, "adm_imp2", "SecurePass1!")

	// Row 2 missing title, Row 3 valid
	csv := "title,category,tags\n,NonExistentCat,\nValid Title,,\n"
	body, ct := multipartFile("file", "invalid.csv", []byte(csv))
	req, _ := http.NewRequest(http.MethodPost, testServer.URL+"/import/upload", body)
	req.Header.Set("Content-Type", ct)
	req.AddCookie(sessionCookie(token))
	resp, err := (&http.Client{CheckRedirect: func(r *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}).Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	// Should still redirect to preview (validation is preview-only, not rejection)
	assert.Equal(t, http.StatusSeeOther, resp.StatusCode)
}

func TestCatalog_BulkImport_OverRowLimit(t *testing.T) {
	truncate(t)

	registerUser(t, "adm_imp3", "adm_imp3@example.com", "SecurePass1!")
	makeAdmin(t, "adm_imp3")
	token := loginUser(t, "adm_imp3", "SecurePass1!")

	var sb strings.Builder
	sb.WriteString("title,category,tags\n")
	for i := 0; i <= 500; i++ { // 501 rows > limit of 500
		sb.WriteString(fmt.Sprintf("Resource %d,,\n", i))
	}
	body, ct := multipartFile("file", "big.csv", []byte(sb.String()))
	req, _ := http.NewRequest(http.MethodPost, testServer.URL+"/import/upload", body)
	req.Header.Set("Content-Type", ct)
	req.AddCookie(sessionCookie(token))
	resp, err := (&http.Client{CheckRedirect: func(r *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}).Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

func TestCatalog_BulkImport_RequiresAdmin(t *testing.T) {
	truncate(t)

	registerUser(t, "auth_imp4", "auth_imp4@example.com", "SecurePass1!")
	makeAuthor(t, "auth_imp4")
	token := loginUser(t, "auth_imp4", "SecurePass1!")

	csv := "title\nSome Title\n"
	body, ct := multipartFile("file", "test.csv", []byte(csv))
	req, _ := http.NewRequest(http.MethodPost, testServer.URL+"/import/upload", body)
	req.Header.Set("Content-Type", ct)
	req.AddCookie(sessionCookie(token))
	resp, err := (&http.Client{}).Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// ─── Bulk Export (Admin only) ─────────────────────────────────────────────────

func TestCatalog_BulkExport_AdminOnly(t *testing.T) {
	truncate(t)

	registerUser(t, "auth_exp", "auth_exp@example.com", "SecurePass1!")
	makeAuthor(t, "auth_exp")
	token := loginUser(t, "auth_exp", "SecurePass1!")
	resp, err := authedClient(token).PostForm(testServer.URL+"/export/generate", url.Values{})
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestCatalog_BulkExport_GeneratesCSV(t *testing.T) {
	truncate(t)

	registerUser(t, "adm_exp", "adm_exp@example.com", "SecurePass1!")
	makeAdmin(t, "adm_exp")
	token := loginUser(t, "adm_exp", "SecurePass1!")
	resp, err := authedClient(token).PostForm(testServer.URL+"/export/generate", url.Values{})
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Disposition"), ".csv")
}

// ─── Audit log ────────────────────────────────────────────────────────────────

func TestCatalog_AuditLog_RecordedOnCreate(t *testing.T) {
	truncate(t)

	registerUser(t, "auditor1", "auditor1@example.com", "SecurePass1!")
	makeAuthor(t, "auditor1")
	token := loginUser(t, "auditor1", "SecurePass1!")

	resp, err := authedClient(token).PostForm(testServer.URL+"/resources", url.Values{
		"title": {"Audited Resource"},
	})
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)

	var count int
	testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM audit_logs WHERE action = 'resource.create'`).Scan(&count)
	assert.GreaterOrEqual(t, count, 1)
}

// ─── Unauthenticated access ───────────────────────────────────────────────────

func TestCatalog_Unauthenticated_Redirects(t *testing.T) {
	resp, err := (&http.Client{CheckRedirect: func(r *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}).Get(testServer.URL + "/resources/new")
	require.NoError(t, err)
	resp.Body.Close()
	// Should redirect to /login or 401
	assert.True(t, resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusUnauthorized,
		"expected redirect or 401, got %d", resp.StatusCode)
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func createDraft(t *testing.T, client *http.Client, title, description string) string {
	t.Helper()
	resp, err := client.PostForm(testServer.URL+"/resources", url.Values{
		"title":       {title},
		"description": {description},
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusSeeOther, resp.StatusCode, "createDraft: expected redirect")
	loc := resp.Header.Get("Location")
	parts := strings.Split(strings.TrimPrefix(loc, "/resources/"), "/")
	return parts[0]
}

func createAndSubmitDraft(t *testing.T, client *http.Client, title, description string) string {
	t.Helper()
	resourceID := createDraft(t, client, title, description)
	version := getResourceVersion(t, resourceID)
	resp, err := client.PostForm(testServer.URL+"/resources/"+resourceID+"/submit", url.Values{
		"version": {fmt.Sprintf("%d", version)},
	})
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusSeeOther, resp.StatusCode, "createAndSubmitDraft: expected redirect")
	return resourceID
}

func approveAndPublish(t *testing.T, reviewerClient, adminClient *http.Client, resourceID string) {
	t.Helper()
	version := getResourceVersion(t, resourceID)
	resp, err := reviewerClient.PostForm(testServer.URL+"/resources/"+resourceID+"/approve", url.Values{
		"version": {fmt.Sprintf("%d", version)},
	})
	require.NoError(t, err)
	resp.Body.Close()

	version = getResourceVersion(t, resourceID)
	resp2, err := adminClient.PostForm(testServer.URL+"/resources/"+resourceID+"/publish", url.Values{
		"version": {fmt.Sprintf("%d", version)},
	})
	require.NoError(t, err)
	resp2.Body.Close()
}

func getResourceStatus(t *testing.T, resourceID string) string {
	t.Helper()
	var status string
	err := testPool.QueryRow(context.Background(),
		`SELECT status FROM resources WHERE id = $1`, resourceID).Scan(&status)
	require.NoError(t, err)
	return status
}

func getResourceVersion(t *testing.T, resourceID string) int {
	t.Helper()
	var version int
	err := testPool.QueryRow(context.Background(),
		`SELECT version FROM resources WHERE id = $1`, resourceID).Scan(&version)
	require.NoError(t, err)
	return version
}

// multipartFile builds a multipart body for a single file field.
func multipartFile(field, filename string, content []byte) (*bytes.Buffer, string) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile(field, filename)
	fw.Write(content)
	w.Close()
	return &buf, w.FormDataContentType()
}

// minimalPDFBytes returns the smallest valid PDF magic bytes that mimetype will detect as application/pdf.
func minimalPDFBytes() []byte {
	return []byte("%PDF-1.4\n1 0 obj\n<</Type /Catalog /Pages 2 0 R>>\nendobj\n2 0 obj\n<</Type /Pages /Kids [3 0 R] /Count 1>>\nendobj\n3 0 obj\n<</Type /Page /Parent 2 0 R /MediaBox [0 0 612 792]>>\nendobj\nxref\n0 4\n0000000000 65535 f\n0000000009 00000 n\n0000000058 00000 n\n0000000115 00000 n\ntrailer\n<</Size 4 /Root 1 0 R>>\nstartxref\n190\n%%EOF")
}
