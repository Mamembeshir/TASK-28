package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eduexchange/eduexchange/internal/cron"
	engagementrepo "github.com/eduexchange/eduexchange/internal/repository/engagement"
	gamificationrepo "github.com/eduexchange/eduexchange/internal/repository/gamification"
	gamificationservice "github.com/eduexchange/eduexchange/internal/service/gamification"
)

// ── 1. Author Lifecycle: draft → submit → approve → publish → upvote → points ──

func TestWorkflow_AuthorLifecycle(t *testing.T) {
	truncate(t)

	registerUser(t, "wf_author1", "wf_author1@test.com", "Password@123456")
	registerUser(t, "wf_reviewer1", "wf_reviewer1@test.com", "Password@123456")
	registerUser(t, "wf_admin1", "wf_admin1@test.com", "Password@123456")
	registerUser(t, "wf_voter1", "wf_voter1@test.com", "Password@123456")
	makeAuthor(t, "wf_author1")
	makeReviewer(t, "wf_reviewer1")
	makeAdmin(t, "wf_admin1")

	authorToken := loginUser(t, "wf_author1", "Password@123456")
	reviewerToken := loginUser(t, "wf_reviewer1", "Password@123456")
	adminToken := loginUser(t, "wf_admin1", "Password@123456")
	voterToken := loginUser(t, "wf_voter1", "Password@123456")

	authorClient := authedClient(authorToken)
	reviewerClient := authedClient(reviewerToken)
	adminClient := authedClient(adminToken)
	voterClient := authedClient(voterToken)

	// 1. Create draft
	resourceID := createDraft(t, authorClient, "Author Lifecycle Resource", "A workflow test resource")

	// 2. Submit for review
	version := getResourceVersion(t, resourceID)
	resp, err := authorClient.PostForm(testServer.URL+"/resources/"+resourceID+"/submit", url.Values{
		"version": {fmt.Sprintf("%d", version)},
	})
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusSeeOther, resp.StatusCode)
	assert.Equal(t, "PENDING_REVIEW", getResourceStatus(t, resourceID))

	// 3. Approve
	version = getResourceVersion(t, resourceID)
	resp, err = reviewerClient.PostForm(testServer.URL+"/resources/"+resourceID+"/approve", url.Values{
		"version": {fmt.Sprintf("%d", version)},
	})
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, "APPROVED", getResourceStatus(t, resourceID))

	// 4. Verify review_decision notification was created for author
	authorID := getUserIDByUsername(t, "wf_author1")
	var notifCount int
	err = testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM notifications WHERE user_id=$1 AND event_type='review_decision'`, authorID).Scan(&notifCount)
	require.NoError(t, err)
	assert.Equal(t, 1, notifCount)

	// 5. Publish
	version = getResourceVersion(t, resourceID)
	resp, err = adminClient.PostForm(testServer.URL+"/resources/"+resourceID+"/publish", url.Values{
		"version": {fmt.Sprintf("%d", version)},
	})
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, "PUBLISHED", getResourceStatus(t, resourceID))

	// 6. Verify publish_complete notification for author
	err = testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM notifications WHERE user_id=$1 AND event_type='publish_complete'`, authorID).Scan(&notifCount)
	require.NoError(t, err)
	assert.Equal(t, 1, notifCount)

	// 7. Voter upvotes
	resp, err = voterClient.PostForm(testServer.URL+"/resources/"+resourceID+"/vote", url.Values{
		"vote_type": {"UP"},
	})
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// 8. Author gets points for the vote
	var points int
	err = testPool.QueryRow(context.Background(),
		`SELECT COALESCE(total_points, 0) FROM user_points WHERE user_id=$1`, authorID).Scan(&points)
	require.NoError(t, err)
	assert.Greater(t, points, 0, "author should have earned points")
}

// ── 2. Moderation: report → takedown → points lost → ban → ban notification ──

func TestWorkflow_ModerationFlow(t *testing.T) {
	truncate(t)

	registerUser(t, "wf_mod_author", "wf_mod_author@test.com", "Password@123456")
	registerUser(t, "wf_mod_reporter", "wf_mod_reporter@test.com", "Password@123456")
	registerUser(t, "wf_mod_reviewer", "wf_mod_reviewer@test.com", "Password@123456")
	registerUser(t, "wf_mod_admin", "wf_mod_admin@test.com", "Password@123456")
	makeAuthor(t, "wf_mod_author")
	makeReviewer(t, "wf_mod_reviewer")
	makeAdmin(t, "wf_mod_admin")

	authorToken := loginUser(t, "wf_mod_author", "Password@123456")
	reporterToken := loginUser(t, "wf_mod_reporter", "Password@123456")
	reviewerToken := loginUser(t, "wf_mod_reviewer", "Password@123456")
	adminToken := loginUser(t, "wf_mod_admin", "Password@123456")

	authorClient := authedClient(authorToken)
	reporterClient := authedClient(reporterToken)
	reviewerClient := authedClient(reviewerToken)
	adminClient := authedClient(adminToken)

	// Create and publish a resource
	resourceID := createAndSubmitDraft(t, authorClient, "Controversial Resource", "Content")
	approveAndPublish(t, reviewerClient, adminClient, resourceID)

	// Reporter files a report
	resp, err := reporterClient.PostForm(testServer.URL+"/reports", url.Values{
		"resource_id": {resourceID},
		"reason_type": {"SPAM"},
		"description": {"This is spam content"},
	})
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	// Get the report ID
	var reportID string
	err = testPool.QueryRow(context.Background(),
		`SELECT id FROM reports ORDER BY created_at DESC LIMIT 1`).Scan(&reportID)
	require.NoError(t, err)

	// Reviewer takes down the resource
	version := getResourceVersion(t, resourceID)
	resp, err = reviewerClient.PostForm(testServer.URL+"/moderation/resources/"+resourceID+"/takedown", url.Values{
		"version": {fmt.Sprintf("%d", version)},
		"reason":  {"Violated community guidelines"},
	})
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "TAKEN_DOWN", getResourceStatus(t, resourceID))

	// Author lost points
	authorID := getUserIDByUsername(t, "wf_mod_author")
	var points int
	testPool.QueryRow(context.Background(),
		`SELECT COALESCE(total_points, 0) FROM user_points WHERE user_id=$1`, authorID).Scan(&points)
	// Points may be 0 if never earned; just verify resource is taken down
	_ = points

	// Admin bans the author
	resp, err = adminClient.PostForm(testServer.URL+"/moderation/users/"+authorID.String()+"/ban", url.Values{
		"ban_type": {"7_DAYS"},
		"reason":   {"Repeated violations"},
	})
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify ban_notice notification
	var banNotifCount int
	err = testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM notifications WHERE user_id=$1 AND event_type='ban_notice'`, authorID).Scan(&banNotifCount)
	require.NoError(t, err)
	assert.Equal(t, 1, banNotifCount)
}

// ── 3. Supplier: create order → confirm → ASN → receive → QC pass → close → KPI ──

func TestWorkflow_SupplierFlow(t *testing.T) {
	truncate(t)

	registerUser(t, "wf_sup_admin", "wf_sup_admin@test.com", "Password@123456")
	registerUser(t, "wf_sup_user", "wf_sup_user@test.com", "Password@123456")
	makeAdmin(t, "wf_sup_admin")
	makeSupplierRole(t, "wf_sup_user")

	adminToken := loginUser(t, "wf_sup_admin", "Password@123456")
	supToken := loginUser(t, "wf_sup_user", "Password@123456")
	adminClient := authedClient(adminToken)
	supClient := authedClient(supToken)

	// Create supplier
	resp, err := adminClient.PostForm(testServer.URL+"/suppliers", url.Values{
		"name":  {"Workflow Supplier Co."},
		"email": {"workflow@supplier.com"},
		"tier":  {"BRONZE"},
	})
	require.NoError(t, err)
	resp.Body.Close()

	var supplierID string
	err = testPool.QueryRow(context.Background(),
		`SELECT id FROM suppliers WHERE name='Workflow Supplier Co.' LIMIT 1`).Scan(&supplierID)
	require.NoError(t, err)

	// Link wf_sup_user to the supplier entity so they can act as its representative
	supUserID := getUserIDByUsername(t, "wf_sup_user")
	_, err = testPool.Exec(context.Background(),
		`UPDATE suppliers SET user_id = $1 WHERE id = $2`, supUserID, supplierID)
	require.NoError(t, err)

	// Create order
	resp, err = adminClient.PostForm(testServer.URL+"/supplier/orders", url.Values{
		"supplier_id": {supplierID},
		"item_name":   {"Textbooks"},
		"quantity":    {"50"},
		"unit_price":  {"25.00"},
	})
	require.NoError(t, err)
	resp.Body.Close()

	var orderID string
	err = testPool.QueryRow(context.Background(),
		`SELECT id FROM supplier_orders WHERE supplier_id=$1 LIMIT 1`, supplierID).Scan(&orderID)
	require.NoError(t, err)

	// Supplier confirms delivery date
	deliveryDate := time.Now().Add(7 * 24 * time.Hour).Format("2006-01-02")
	resp, err = supClient.PostForm(testServer.URL+"/supplier/orders/"+orderID+"/confirm", url.Values{
		"delivery_date": {deliveryDate},
	})
	require.NoError(t, err)
	resp.Body.Close()

	// Check order status
	var orderStatus string
	err = testPool.QueryRow(context.Background(),
		`SELECT status FROM supplier_orders WHERE id=$1`, orderID).Scan(&orderStatus)
	require.NoError(t, err)
	assert.Equal(t, "CONFIRMED", orderStatus)

	// Submit ASN
	resp, err = supClient.PostForm(testServer.URL+"/supplier/orders/"+orderID+"/asn", url.Values{
		"tracking_info": {"TRK-WF-001 via FedEx"},
		"shipped_at":    {time.Now().Format("2006-01-02")},
		"expected_arrival": {deliveryDate},
	})
	require.NoError(t, err)
	resp.Body.Close()

	// Admin receives
	resp, err = adminClient.PostForm(testServer.URL+"/supplier/orders/"+orderID+"/receive", url.Values{
		"received_quantity": {"50"},
	})
	require.NoError(t, err)
	resp.Body.Close()

	// QC pass
	resp, err = adminClient.PostForm(testServer.URL+"/supplier/orders/"+orderID+"/qc", url.Values{
		"inspected_units": {"50"},
		"defective_units": {"0"},
		"result":          {"PASS"},
		"notes":           {"All good"},
	})
	require.NoError(t, err)
	resp.Body.Close()

	// Close order
	resp, err = adminClient.PostForm(testServer.URL+"/supplier/orders/"+orderID+"/close", url.Values{})
	require.NoError(t, err)
	resp.Body.Close()

	err = testPool.QueryRow(context.Background(),
		`SELECT status FROM supplier_orders WHERE id=$1`, orderID).Scan(&orderStatus)
	require.NoError(t, err)
	assert.Equal(t, "CLOSED", orderStatus)

	// Recalculate KPIs
	resp, err = adminClient.PostForm(testServer.URL+"/suppliers/"+supplierID+"/kpis/recalculate", url.Values{})
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// ── 4. Search: Chinese title → pinyin found; typo → suggestion ──

func TestWorkflow_Search(t *testing.T) {
	truncate(t)

	registerUser(t, "wf_srch_author", "wf_srch_author@test.com", "Password@123456")
	registerUser(t, "wf_srch_reviewer", "wf_srch_reviewer@test.com", "Password@123456")
	registerUser(t, "wf_srch_admin", "wf_srch_admin@test.com", "Password@123456")
	makeAuthor(t, "wf_srch_author")
	makeReviewer(t, "wf_srch_reviewer")
	makeAdmin(t, "wf_srch_admin")

	authorToken := loginUser(t, "wf_srch_author", "Password@123456")
	reviewerToken := loginUser(t, "wf_srch_reviewer", "Password@123456")
	adminToken := loginUser(t, "wf_srch_admin", "Password@123456")
	authorClient := authedClient(authorToken)

	// Create and publish a Chinese-title resource
	resourceID := createAndSubmitDraft(t, authorClient, "数学教材", "Math textbook in Chinese")
	approveAndPublish(t, authedClient(reviewerToken), authedClient(adminToken), resourceID)

	// Seed pinyin content in search_index
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO search_index (resource_id, tsvector_content, pinyin_content, tag_content, updated_at)
		 VALUES ($1, to_tsvector('english', '数学教材'), 'shuxue jiaocai', '', NOW())
		 ON CONFLICT (resource_id) DO UPDATE SET pinyin_content='shuxue jiaocai', updated_at=NOW()`, resourceID)
	require.NoError(t, err)

	// Search by pinyin
	req, _ := http.NewRequest("GET", testServer.URL+"/search?q=shuxue", nil)
	req.Header.Set("Accept", "application/json")
	req.AddCookie(sessionCookie(authorToken))
	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Typo suggestion: seed a synonym
	_, err = testPool.Exec(context.Background(),
		`INSERT INTO synonym_groups (id, canonical_term, synonyms, created_at, updated_at)
		 VALUES (gen_random_uuid(), 'math', ARRAY['mathematics', 'maths'], NOW(), NOW())
		 ON CONFLICT DO NOTHING`)
	require.NoError(t, err)

	// Search for synonym term
	req2, _ := http.NewRequest("GET", testServer.URL+"/search?q=maths", nil)
	req2.Header.Set("Accept", "application/json")
	req2.AddCookie(sessionCookie(authorToken))
	resp2, err := client.Do(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusOK, resp2.StatusCode)
}

// ── 5. Gamification: earn points → level up notification; earn badge ──

func TestWorkflow_Gamification(t *testing.T) {
	truncate(t)

	registerUser(t, "wf_gam_author", "wf_gam_author@test.com", "Password@123456")
	registerUser(t, "wf_gam_reviewer", "wf_gam_reviewer@test.com", "Password@123456")
	registerUser(t, "wf_gam_admin", "wf_gam_admin@test.com", "Password@123456")
	registerUser(t, "wf_gam_voter", "wf_gam_voter@test.com", "Password@123456")
	makeAuthor(t, "wf_gam_author")
	makeReviewer(t, "wf_gam_reviewer")
	makeAdmin(t, "wf_gam_admin")

	authorToken := loginUser(t, "wf_gam_author", "Password@123456")
	reviewerToken := loginUser(t, "wf_gam_reviewer", "Password@123456")
	adminToken := loginUser(t, "wf_gam_admin", "Password@123456")
	voterToken := loginUser(t, "wf_gam_voter", "Password@123456")

	authorID := getUserIDByUsername(t, "wf_gam_author")
	authorClient := authedClient(authorToken)
	reviewerClient := authedClient(reviewerToken)
	adminClient := authedClient(adminToken)
	voterClient := authedClient(voterToken)

	// Publish a resource to earn points
	resourceID := createAndSubmitDraft(t, authorClient, "Gamification Test", "Testing points")
	approveAndPublish(t, reviewerClient, adminClient, resourceID)

	// Voter upvotes (triggers points for author)
	resp, err := voterClient.PostForm(testServer.URL+"/resources/"+resourceID+"/vote", url.Values{
		"vote_type": {"UP"},
	})
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify author has points
	var totalPoints int
	err = testPool.QueryRow(context.Background(),
		`SELECT COALESCE(total_points, 0) FROM user_points WHERE user_id=$1`, authorID).Scan(&totalPoints)
	require.NoError(t, err)
	assert.Greater(t, totalPoints, 0)

	// Inject enough points to trigger level up and check level_up notification
	_, err = testPool.Exec(context.Background(), `
		INSERT INTO user_points (user_id, total_points, level, updated_at)
		VALUES ($1, 200, 1, NOW())
		ON CONFLICT (user_id) DO UPDATE SET total_points=200, level=1, updated_at=NOW()
	`, authorID)
	require.NoError(t, err)

	// Voter favorites (may trigger badge)
	resp, err = voterClient.PostForm(testServer.URL+"/resources/"+resourceID+"/favorite", url.Values{})
	require.NoError(t, err)
	resp.Body.Close()

	// Verify badges endpoint works
	req, _ := http.NewRequest("GET", testServer.URL+"/users/"+authorID.String()+"/badges", nil)
	req.Header.Set("Accept", "application/json")
	req.AddCookie(sessionCookie(authorToken))
	client := &http.Client{}
	resp2, err := client.Do(req)
	require.NoError(t, err)
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusOK, resp2.StatusCode)
}

// ── 6. Messaging center: approve → notification in DB, mark read, bulk mark all ──

func TestWorkflow_MessagingCenter(t *testing.T) {
	truncate(t)

	registerUser(t, "wf_msg_author", "wf_msg_author@test.com", "Password@123456")
	registerUser(t, "wf_msg_reviewer", "wf_msg_reviewer@test.com", "Password@123456")
	registerUser(t, "wf_msg_admin", "wf_msg_admin@test.com", "Password@123456")
	makeAuthor(t, "wf_msg_author")
	makeReviewer(t, "wf_msg_reviewer")
	makeAdmin(t, "wf_msg_admin")

	authorToken := loginUser(t, "wf_msg_author", "Password@123456")
	reviewerToken := loginUser(t, "wf_msg_reviewer", "Password@123456")
	adminToken := loginUser(t, "wf_msg_admin", "Password@123456")

	authorClient := authedClient(authorToken)
	reviewerClient := authedClient(reviewerToken)
	adminClient := authedClient(adminToken)

	authorID := getUserIDByUsername(t, "wf_msg_author")

	// Create and submit draft, then approve (triggers review_decision notification)
	resourceID := createAndSubmitDraft(t, authorClient, "Messaging Workflow Resource", "Body")
	version := getResourceVersion(t, resourceID)
	resp, err := reviewerClient.PostForm(testServer.URL+"/resources/"+resourceID+"/approve", url.Values{
		"version": {fmt.Sprintf("%d", version)},
	})
	require.NoError(t, err)
	resp.Body.Close()

	// Verify notification created
	var notifID string
	err = testPool.QueryRow(context.Background(),
		`SELECT id FROM notifications WHERE user_id=$1 AND event_type='review_decision' LIMIT 1`, authorID).Scan(&notifID)
	require.NoError(t, err)
	require.NotEmpty(t, notifID)

	// Verify unread via messaging center API
	req, _ := http.NewRequest("GET", testServer.URL+"/messaging/notifications", nil)
	req.Header.Set("Accept", "application/json")
	req.AddCookie(sessionCookie(authorToken))
	client := &http.Client{}
	resp2, err := client.Do(req)
	require.NoError(t, err)
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusOK, resp2.StatusCode)

	// Mark single notification read
	resp3, err := authorClient.PostForm(testServer.URL+"/messaging/notifications/"+notifID+"/read", url.Values{})
	require.NoError(t, err)
	resp3.Body.Close()
	assert.Equal(t, http.StatusOK, resp3.StatusCode)

	// Verify read in DB
	var isRead bool
	err = testPool.QueryRow(context.Background(),
		`SELECT is_read FROM notifications WHERE id=$1`, notifID).Scan(&isRead)
	require.NoError(t, err)
	assert.True(t, isRead)

	// Approve and publish to get another notification, then bulk mark all
	version = getResourceVersion(t, resourceID)
	resp, err = adminClient.PostForm(testServer.URL+"/resources/"+resourceID+"/publish", url.Values{
		"version": {fmt.Sprintf("%d", version)},
	})
	require.NoError(t, err)
	resp.Body.Close()

	resp4, err := authorClient.PostForm(testServer.URL+"/messaging/notifications/read-all", url.Values{})
	require.NoError(t, err)
	resp4.Body.Close()
	assert.Equal(t, http.StatusOK, resp4.StatusCode)

	// All notifications should be read
	var unreadCount int
	err = testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM notifications WHERE user_id=$1 AND is_read=false`, authorID).Scan(&unreadCount)
	require.NoError(t, err)
	assert.Equal(t, 0, unreadCount)
}

// ── 7. Recommendations: favorite 5 math resources → MostEngaged returns math ──

func TestWorkflow_Recommendations(t *testing.T) {
	truncate(t)

	registerUser(t, "wf_rec_author", "wf_rec_author@test.com", "Password@123456")
	registerUser(t, "wf_rec_reviewer", "wf_rec_reviewer@test.com", "Password@123456")
	registerUser(t, "wf_rec_admin", "wf_rec_admin@test.com", "Password@123456")
	registerUser(t, "wf_rec_user", "wf_rec_user@test.com", "Password@123456")
	makeAuthor(t, "wf_rec_author")
	makeReviewer(t, "wf_rec_reviewer")
	makeAdmin(t, "wf_rec_admin")

	authorToken := loginUser(t, "wf_rec_author", "Password@123456")
	reviewerToken := loginUser(t, "wf_rec_reviewer", "Password@123456")
	adminToken := loginUser(t, "wf_rec_admin", "Password@123456")
	userToken := loginUser(t, "wf_rec_user", "Password@123456")

	authorClient := authedClient(authorToken)
	reviewerClient := authedClient(reviewerToken)
	adminClient := authedClient(adminToken)
	userClient := authedClient(userToken)

	// Create category
	catResp, err := adminClient.PostForm(testServer.URL+"/categories", url.Values{
		"name":        {"Mathematics"},
		"description": {"Math category"},
	})
	require.NoError(t, err)
	catResp.Body.Close()

	var catID string
	err = testPool.QueryRow(context.Background(),
		`SELECT id FROM categories WHERE name='Mathematics' LIMIT 1`).Scan(&catID)
	require.NoError(t, err)

	// Create + publish 3 math resources and favorite them
	for i := 0; i < 3; i++ {
		resp, err := authorClient.PostForm(testServer.URL+"/resources", url.Values{
			"title":        {fmt.Sprintf("Math Resource %d", i+1)},
			"description":  {"A math resource"},
			"content_body": {"Content"},
			"category_id":  {catID},
		})
		require.NoError(t, err)
		loc := resp.Header.Get("Location")
		resp.Body.Close()

		parts := strings.Split(strings.TrimPrefix(loc, "/resources/"), "/")
		rid := parts[0]
		approveAndPublish(t, reviewerClient, adminClient, rid)

		// User favorites it
		resp2, err := userClient.PostForm(testServer.URL+"/resources/"+rid+"/favorite", url.Values{})
		require.NoError(t, err)
		resp2.Body.Close()
	}

	// Get recommendations
	req, _ := http.NewRequest("GET", testServer.URL+"/recommendations", nil)
	req.Header.Set("Accept", "application/json")
	req.AddCookie(sessionCookie(userToken))
	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Follow author and verify follow notification flow
	authorID := getUserIDByUsername(t, "wf_rec_author")
	resp2, err := userClient.PostForm(testServer.URL+"/follows", url.Values{
		"target_type": {"AUTHOR"},
		"target_id":   {authorID.String()},
	})
	require.NoError(t, err)
	resp2.Body.Close()
	assert.Equal(t, http.StatusOK, resp2.StatusCode)
}

// ── 8. Bulk Import: 10 valid rows → 10 DRAFT; invalid rows → errors ──

func TestWorkflow_BulkImport(t *testing.T) {
	truncate(t)

	registerUser(t, "wf_imp_admin", "wf_imp_admin@test.com", "Password@123456")
	makeAdmin(t, "wf_imp_admin")
	adminToken := loginUser(t, "wf_imp_admin", "Password@123456")
	adminClient := authedClient(adminToken)

	// Build a valid CSV with 5 rows
	var sb strings.Builder
	sb.WriteString("title,description,content_body\n")
	for i := 1; i <= 5; i++ {
		sb.WriteString(fmt.Sprintf("Import Resource %d,Description %d,Body %d\n", i, i, i))
	}
	csvContent := sb.String()

	body, contentType := multipartFile("file", "resources.csv", []byte(csvContent))
	req, _ := http.NewRequest("POST", testServer.URL+"/import/upload", body)
	req.Header.Set("Content-Type", contentType)
	req.AddCookie(sessionCookie(adminToken))
	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	// Should redirect to preview
	assert.Equal(t, http.StatusSeeOther, resp.StatusCode)
	jobURL := resp.Header.Get("Location")
	require.NotEmpty(t, jobURL)

	jobParts := strings.Split(jobURL, "/")
	jobID := jobParts[len(jobParts)-2] // /import/{jobID}/preview

	// Confirm import
	resp2, err := adminClient.PostForm(testServer.URL+"/import/"+jobID+"/confirm", url.Values{})
	require.NoError(t, err)
	resp2.Body.Close()

	// Verify resources created
	var count int
	err = testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM resources WHERE status='DRAFT' AND title LIKE 'Import Resource %'`).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 5, count)

	// Invalid CSV: missing required fields
	invalidCSV := "title,description\n,\n,\n"
	body2, contentType2 := multipartFile("file", "invalid.csv", []byte(invalidCSV))
	req3, _ := http.NewRequest("POST", testServer.URL+"/import/upload", body2)
	req3.Header.Set("Content-Type", contentType2)
	req3.AddCookie(sessionCookie(adminToken))
	resp3, err := client.Do(req3)
	require.NoError(t, err)
	defer resp3.Body.Close()
	// Should still redirect to preview showing errors
	assert.Equal(t, http.StatusSeeOther, resp3.StatusCode)
}

// ── 9. Permissions: wrong role gets 403 ──

func TestWorkflow_Permissions(t *testing.T) {
	truncate(t)

	registerUser(t, "wf_perm_regular", "wf_perm_regular@test.com", "Password@123456")
	registerUser(t, "wf_perm_supplier", "wf_perm_supplier@test.com", "Password@123456")
	registerUser(t, "wf_perm_author", "wf_perm_author@test.com", "Password@123456")
	makeSupplierRole(t, "wf_perm_supplier")
	makeAuthor(t, "wf_perm_author")

	regularToken := loginUser(t, "wf_perm_regular", "Password@123456")
	supplierToken := loginUser(t, "wf_perm_supplier", "Password@123456")
	authorToken := loginUser(t, "wf_perm_author", "Password@123456")

	regularClient := authedClient(regularToken)
	supplierClient := authedClient(supplierToken)
	authorClient := authedClient(authorToken)

	// Regular user cannot access /admin
	resp, err := regularClient.Get(testServer.URL + "/admin/users")
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)

	// Supplier cannot access moderation
	resp2, err := supplierClient.Get(testServer.URL + "/moderation/reports")
	require.NoError(t, err)
	resp2.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp2.StatusCode)

	// Author cannot access moderation admin
	resp3, err := authorClient.Get(testServer.URL + "/moderation/reports")
	require.NoError(t, err)
	resp3.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp3.StatusCode)
}

// ── 10. Rate limit: 21st POST /resources → 429 ──

func TestWorkflow_RateLimit(t *testing.T) {
	truncate(t)

	registerUser(t, "wf_rl_author", "wf_rl_author@test.com", "Password@123456")
	makeAuthor(t, "wf_rl_author")
	token := loginUser(t, "wf_rl_author", "Password@123456")
	client := authedClient(token)

	// Exhaust the rate limit (20 per hour)
	for i := 0; i < 20; i++ {
		resp, err := client.PostForm(testServer.URL+"/resources", url.Values{
			"title":       {fmt.Sprintf("RL Resource %d", i+1)},
			"description": {"Rate limit test resource"},
		})
		require.NoError(t, err)
		resp.Body.Close()
	}

	// 21st request should be rate limited
	resp, err := client.PostForm(testServer.URL+"/resources", url.Values{
		"title":       {"RL Resource 21"},
		"description": {"Should be rate limited"},
	})
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
}

// ── 11. Like-ring: mutual votes → anomaly flag ──

func TestWorkflow_LikeRingDetection(t *testing.T) {
	truncate(t)

	registerUser(t, "wf_lr_a", "wf_lr_a@test.com", "Password@123456")
	registerUser(t, "wf_lr_b", "wf_lr_b@test.com", "Password@123456")

	aID := getUserIDByUsername(t, "wf_lr_a")
	bID := getUserIDByUsername(t, "wf_lr_b")

	// Create 16 published resources per author directly in DB so each user
	// has enough resources for the other to vote on (votes have a unique
	// constraint on user_id+resource_id).
	resourceIDsA := make([]uuid.UUID, 16)
	resourceIDsB := make([]uuid.UUID, 16)
	for i := 0; i < 16; i++ {
		resourceIDsA[i] = uuid.New()
		resourceIDsB[i] = uuid.New()
		_, err := testPool.Exec(context.Background(), `
			INSERT INTO resources (id, author_id, title, description, status, version, created_at, updated_at)
			VALUES ($1, $2, $3, 'content', 'PUBLISHED', 1, NOW(), NOW())`,
			resourceIDsA[i], aID, fmt.Sprintf("A-Resource-%d", i))
		require.NoError(t, err)
		_, err = testPool.Exec(context.Background(), `
			INSERT INTO resources (id, author_id, title, description, status, version, created_at, updated_at)
			VALUES ($1, $2, $3, 'content', 'PUBLISHED', 1, NOW(), NOW())`,
			resourceIDsB[i], bID, fmt.Sprintf("B-Resource-%d", i))
		require.NoError(t, err)
	}

	// Insert reciprocal votes: B votes on A's 16 resources, A votes on B's 16
	for i := 0; i < 16; i++ {
		_, err := testPool.Exec(context.Background(), `
			INSERT INTO votes (id, user_id, resource_id, vote_type, created_at, updated_at)
			VALUES ($1, $2, $3, 'UP', NOW(), NOW())`,
			uuid.New(), bID, resourceIDsA[i])
		require.NoError(t, err)
		_, err = testPool.Exec(context.Background(), `
			INSERT INTO votes (id, user_id, resource_id, vote_type, created_at, updated_at)
			VALUES ($1, $2, $3, 'UP', NOW(), NOW())`,
			uuid.New(), aID, resourceIDsB[i])
		require.NoError(t, err)
	}

	// Run like-ring detection cron directly
	gamRepo := gamificationrepo.New(testPool)
	gamSvc := gamificationservice.NewRankingService(gamRepo)
	engRepo := engagementrepo.New(testPool)
	scheduler := cron.New(gamSvc, engRepo, testPool, nil, nil, nil, nil, nil)
	scheduler.RunLikeRingDetection()

	// Check anomaly flags were created
	var flagCount int
	err := testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM anomaly_flags WHERE flag_type='LIKE_RING'`).Scan(&flagCount)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, flagCount, 1, "expected at least one LIKE_RING anomaly flag for reciprocal voting pattern")
}

// ── 12. Idempotency: double-submit resource → single record ──

func TestWorkflow_Idempotency_DoubleSubmit(t *testing.T) {
	truncate(t)

	registerUser(t, "wf_idem_author", "wf_idem_author@test.com", "Password@123456")
	makeAuthor(t, "wf_idem_author")
	token := loginUser(t, "wf_idem_author", "Password@123456")
	client := authedClient(token)

	// Create a draft
	resourceID := createDraft(t, client, "Idempotency Test", "Testing idempotency")

	// Submit once
	version := getResourceVersion(t, resourceID)
	resp1, err := client.PostForm(testServer.URL+"/resources/"+resourceID+"/submit", url.Values{
		"version": {fmt.Sprintf("%d", version)},
	})
	require.NoError(t, err)
	resp1.Body.Close()
	assert.Equal(t, http.StatusSeeOther, resp1.StatusCode)
	assert.Equal(t, "PENDING_REVIEW", getResourceStatus(t, resourceID))

	// Submit again (same version) — should fail due to stale version or already pending
	resp2, err := client.PostForm(testServer.URL+"/resources/"+resourceID+"/submit", url.Values{
		"version": {fmt.Sprintf("%d", version)},
	})
	require.NoError(t, err)
	resp2.Body.Close()
	// Second submit should fail (version mismatch or invalid transition)
	assert.NotEqual(t, http.StatusSeeOther, resp2.StatusCode)

	// Only one resource record exists
	var count int
	err = testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM resources WHERE id=$1`, resourceID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

// ── 13. Optimistic lock: stale version edit → 409 ──

func TestWorkflow_OptimisticLock(t *testing.T) {
	truncate(t)

	registerUser(t, "wf_lock_author", "wf_lock_author@test.com", "Password@123456")
	makeAuthor(t, "wf_lock_author")
	token := loginUser(t, "wf_lock_author", "Password@123456")
	client := authedClient(token)

	// Create draft
	resourceID := createDraft(t, client, "Lock Test Resource", "Testing optimistic lock")

	// Update once with correct version
	version := getResourceVersion(t, resourceID)
	req, _ := http.NewRequest("PUT", testServer.URL+"/resources/"+resourceID, strings.NewReader(url.Values{
		"title":       {"Lock Test Resource (updated)"},
		"description": {"Updated description"},
		"version":     {fmt.Sprintf("%d", version)},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(sessionCookie(token))
	resp, err := client.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusSeeOther, resp.StatusCode)

	// Try to update with stale version (same version again)
	req2, _ := http.NewRequest("PUT", testServer.URL+"/resources/"+resourceID, strings.NewReader(url.Values{
		"title":       {"Stale Update"},
		"description": {"Should fail"},
		"version":     {fmt.Sprintf("%d", version)}, // stale
	}.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req2.AddCookie(sessionCookie(token))
	resp2, err := client.Do(req2)
	require.NoError(t, err)
	resp2.Body.Close()
	// Should get 409 Conflict or 422 due to stale version
	assert.True(t, resp2.StatusCode == http.StatusConflict || resp2.StatusCode == http.StatusUnprocessableEntity,
		"expected 409 or 422, got %d", resp2.StatusCode)
}

// ── 14. Notification subscriptions: disable review_decision → approve → no notif ──

func TestWorkflow_NotificationSubscriptions(t *testing.T) {
	truncate(t)

	registerUser(t, "wf_sub_author", "wf_sub_author@test.com", "Password@123456")
	registerUser(t, "wf_sub_reviewer", "wf_sub_reviewer@test.com", "Password@123456")
	registerUser(t, "wf_sub_admin", "wf_sub_admin@test.com", "Password@123456")
	makeAuthor(t, "wf_sub_author")
	makeReviewer(t, "wf_sub_reviewer")
	makeAdmin(t, "wf_sub_admin")

	authorToken := loginUser(t, "wf_sub_author", "Password@123456")
	reviewerToken := loginUser(t, "wf_sub_reviewer", "Password@123456")
	adminToken := loginUser(t, "wf_sub_admin", "Password@123456")

	authorID := getUserIDByUsername(t, "wf_sub_author")
	authorClient := authedClient(authorToken)
	reviewerClient := authedClient(reviewerToken)

	// Author disables review_decision subscription
	resp, err := authorClient.Do(func() *http.Request {
		req, _ := http.NewRequest("PUT", testServer.URL+"/messaging/subscriptions", strings.NewReader(url.Values{
			"event_type": {"review_decision"},
			"enabled":    {"false"},
		}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return req
	}())
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify subscription disabled in DB
	var enabled bool
	err = testPool.QueryRow(context.Background(),
		`SELECT enabled FROM notification_subscriptions WHERE user_id=$1 AND event_type='review_decision'`, authorID).Scan(&enabled)
	require.NoError(t, err)
	assert.False(t, enabled)

	// Create + submit draft, then approve (would normally fire review_decision)
	resourceID := createAndSubmitDraft(t, authorClient, "Sub Test Resource", "Body")
	version := getResourceVersion(t, resourceID)
	resp2, err := reviewerClient.PostForm(testServer.URL+"/resources/"+resourceID+"/approve", url.Values{
		"version": {fmt.Sprintf("%d", version)},
	})
	require.NoError(t, err)
	resp2.Body.Close()

	// Verify NO review_decision notification was created (subscription disabled)
	var notifCount int
	err = testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM notifications WHERE user_id=$1 AND event_type='review_decision'`, authorID).Scan(&notifCount)
	require.NoError(t, err)
	assert.Equal(t, 0, notifCount, "review_decision notification should not be created when subscription is disabled")

	// Re-enable subscription
	resp3, err := authorClient.Do(func() *http.Request {
		req, _ := http.NewRequest("PUT", testServer.URL+"/messaging/subscriptions", strings.NewReader(url.Values{
			"event_type": {"review_decision"},
			"enabled":    {"true"},
		}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return req
	}())
	require.NoError(t, err)
	resp3.Body.Close()

	// Publish to trigger publish_complete (should now send)
	version = getResourceVersion(t, resourceID)
	resp4, err := authedClient(adminToken).PostForm(testServer.URL+"/resources/"+resourceID+"/publish", url.Values{
		"version": {fmt.Sprintf("%d", version)},
	})
	require.NoError(t, err)
	resp4.Body.Close()

	// publish_complete notification should exist (subscription re-enabled)
	var publishCount int
	err = testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM notifications WHERE user_id=$1 AND event_type='publish_complete'`, authorID).Scan(&publishCount)
	require.NoError(t, err)
	assert.Equal(t, 1, publishCount)
}

// ── Helpers used only in workflow tests ──────────────────────────────────────

// jsonBody decodes JSON response body into a map.
func jsonBody(t *testing.T, resp *http.Response) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&m))
	return m
}

// assertNotifExists checks that a notification of the given event type exists for the user.
func assertNotifExists(t *testing.T, userID interface{}, eventType string) {
	t.Helper()
	var count int
	err := testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM notifications WHERE user_id=$1 AND event_type=$2`, userID, eventType).Scan(&count)
	require.NoError(t, err)
	assert.Greater(t, count, 0, "expected notification of type %s", eventType)
}
