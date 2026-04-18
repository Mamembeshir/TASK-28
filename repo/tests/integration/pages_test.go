package integration_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── GET /login ───────────────────────────────────────────────────────────────

func TestGetLogin_Returns200(t *testing.T) {
	resp, err := publicClient(t).Get(testServer.URL + "/login")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// ─── GET /register ───────────────────────────────────────────────────────────

func TestGetRegister_Returns200(t *testing.T) {
	resp, err := publicClient(t).Get(testServer.URL + "/register")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// ─── POST /logout ─────────────────────────────────────────────────────────────

func TestPostLogout_AuthenticatedUser_ClearsSession(t *testing.T) {
	truncate(t)
	registerUser(t, "logout_user1", "logout1@example.com", "SecurePass1!")
	token := loginUser(t, "logout_user1", "SecurePass1!")
	client := authedClient(t, token)

	resp, err := client.PostForm(testServer.URL+"/logout", url.Values{})
	require.NoError(t, err)
	defer resp.Body.Close()

	// Logout redirects to /login (302) for plain clients, or responds 200 for HTMX (via HX-Redirect).
	// 303 accepted for robustness across redirect helpers.
	assert.True(t,
		resp.StatusCode == http.StatusFound ||
			resp.StatusCode == http.StatusSeeOther ||
			resp.StatusCode == http.StatusOK,
		"expected 302, 303 or 200, got %d", resp.StatusCode)
}

func TestPostLogout_UnauthenticatedUser_DoesNotError(t *testing.T) {
	resp, err := publicClient(t).PostForm(testServer.URL+"/logout", url.Values{})
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.True(t, resp.StatusCode < 500, "expected non-5xx, got %d", resp.StatusCode)
}

// ─── GET / ────────────────────────────────────────────────────────────────────

func TestGetHome_Authenticated_Returns200(t *testing.T) {
	truncate(t)
	registerUser(t, "home_user1", "home1@example.com", "SecurePass1!")
	token := loginUser(t, "home_user1", "SecurePass1!")

	resp, err := authedClient(t, token).Get(testServer.URL + "/")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestGetHome_Unauthenticated_RedirectsToLogin(t *testing.T) {
	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(testServer.URL + "/")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.True(t, resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusUnauthorized,
		"expected 302 or 401, got %d", resp.StatusCode)
}

// ─── GET /tags ────────────────────────────────────────────────────────────────

func TestGetTags_Authenticated_ReturnsList(t *testing.T) {
	truncate(t)
	registerUser(t, "tags_user1", "tags1@example.com", "SecurePass1!")
	token := loginUser(t, "tags_user1", "SecurePass1!")

	req, _ := http.NewRequest(http.MethodGet, testServer.URL+"/tags", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := authedClient(t, token).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestGetTags_Unauthenticated_Redirects(t *testing.T) {
	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(testServer.URL + "/tags")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.True(t, resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusUnauthorized,
		"expected redirect or 401, got %d", resp.StatusCode)
}

// ─── GET /resources/new ──────────────────────────────────────────────────────

func TestGetResourceNew_AsAuthor_Returns200(t *testing.T) {
	truncate(t)
	registerUser(t, "new_author1", "new_author1@example.com", "SecurePass1!")
	makeAuthor(t, "new_author1")
	token := loginUser(t, "new_author1", "SecurePass1!")

	resp, err := authedClient(t, token).Get(testServer.URL + "/resources/new")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestGetResourceNew_AsRegularUser_Forbidden(t *testing.T) {
	truncate(t)
	registerUser(t, "new_regular1", "new_regular1@example.com", "SecurePass1!")
	token := loginUser(t, "new_regular1", "SecurePass1!")

	resp, err := authedClient(t, token).Get(testServer.URL + "/resources/new")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// ─── GET /resources/:id/edit ─────────────────────────────────────────────────

func TestGetResourceEdit_AsAuthor_Returns200(t *testing.T) {
	truncate(t)
	registerUser(t, "edit_author1", "edit_author1@example.com", "SecurePass1!")
	makeAuthor(t, "edit_author1")
	token := loginUser(t, "edit_author1", "SecurePass1!")
	client := authedClient(t, token)

	resourceID := createDraft(t, client, "Edit Page Test", "description")

	resp, err := client.Get(testServer.URL + "/resources/" + resourceID + "/edit")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestGetResourceEdit_AsRegularUser_Forbidden(t *testing.T) {
	truncate(t)
	registerUser(t, "edit_author2", "edit_author2@example.com", "SecurePass1!")
	registerUser(t, "edit_regular2", "edit_regular2@example.com", "SecurePass1!")
	makeAuthor(t, "edit_author2")
	authorToken := loginUser(t, "edit_author2", "SecurePass1!")
	regularToken := loginUser(t, "edit_regular2", "SecurePass1!")

	resourceID := createDraft(t, authedClient(t, authorToken), "Edit Forbidden Test", "desc")

	resp, err := authedClient(t, regularToken).Get(testServer.URL + "/resources/" + resourceID + "/edit")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// ─── GET /review-queue ───────────────────────────────────────────────────────

func TestGetReviewQueue_AsReviewer_Returns200(t *testing.T) {
	truncate(t)
	registerUser(t, "rq_reviewer1", "rq_reviewer1@example.com", "SecurePass1!")
	makeReviewer(t, "rq_reviewer1")
	token := loginUser(t, "rq_reviewer1", "SecurePass1!")

	resp, err := authedClient(t, token).Get(testServer.URL + "/review-queue")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestGetReviewQueue_AsRegularUser_Forbidden(t *testing.T) {
	truncate(t)
	registerUser(t, "rq_regular1", "rq_regular1@example.com", "SecurePass1!")
	token := loginUser(t, "rq_regular1", "SecurePass1!")

	resp, err := authedClient(t, token).Get(testServer.URL + "/review-queue")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// ─── GET /import ─────────────────────────────────────────────────────────────

func TestGetImport_AsAdmin_Returns200(t *testing.T) {
	truncate(t)
	registerUser(t, "imp_admin1", "imp_admin1@example.com", "SecurePass1!")
	makeAdmin(t, "imp_admin1")
	token := loginUser(t, "imp_admin1", "SecurePass1!")

	resp, err := authedClient(t, token).Get(testServer.URL + "/import")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestGetImport_AsRegularUser_Forbidden(t *testing.T) {
	truncate(t)
	registerUser(t, "imp_regular1", "imp_regular1@example.com", "SecurePass1!")
	token := loginUser(t, "imp_regular1", "SecurePass1!")

	resp, err := authedClient(t, token).Get(testServer.URL + "/import")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// ─── GET /export ─────────────────────────────────────────────────────────────

func TestGetExport_AsAdmin_Returns200(t *testing.T) {
	truncate(t)
	registerUser(t, "exp_admin1", "exp_admin1@example.com", "SecurePass1!")
	makeAdmin(t, "exp_admin1")
	token := loginUser(t, "exp_admin1", "SecurePass1!")

	resp, err := authedClient(t, token).Get(testServer.URL + "/export")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestGetExport_AsRegularUser_Forbidden(t *testing.T) {
	truncate(t)
	registerUser(t, "exp_regular1", "exp_regular1@example.com", "SecurePass1!")
	token := loginUser(t, "exp_regular1", "SecurePass1!")

	resp, err := authedClient(t, token).Get(testServer.URL + "/export")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// ─── GET /categories ─────────────────────────────────────────────────────────

func TestGetCategories_AsAdmin_ReturnsList(t *testing.T) {
	truncate(t)
	registerUser(t, "cat_admin1", "cat_admin1@example.com", "SecurePass1!")
	makeAdmin(t, "cat_admin1")
	token := loginUser(t, "cat_admin1", "SecurePass1!")

	req, _ := http.NewRequest(http.MethodGet, testServer.URL+"/categories", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := authedClient(t, token).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestGetCategories_AsRegularUser_Forbidden(t *testing.T) {
	truncate(t)
	registerUser(t, "cat_regular1", "cat_regular1@example.com", "SecurePass1!")
	token := loginUser(t, "cat_regular1", "SecurePass1!")

	resp, err := authedClient(t, token).Get(testServer.URL + "/categories")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// ─── GET /recommendation-strategies ─────────────────────────────────────────

func TestGetRecommendationStrategies_AsAdmin_Returns200(t *testing.T) {
	truncate(t)
	registerUser(t, "strat_admin1", "strat_admin1@example.com", "SecurePass1!")
	makeAdmin(t, "strat_admin1")
	token := loginUser(t, "strat_admin1", "SecurePass1!")

	req, _ := http.NewRequest(http.MethodGet, testServer.URL+"/recommendation-strategies", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := authedClient(t, token).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestGetRecommendationStrategies_AsRegularUser_Forbidden(t *testing.T) {
	truncate(t)
	registerUser(t, "strat_regular1", "strat_regular1@example.com", "SecurePass1!")
	token := loginUser(t, "strat_regular1", "SecurePass1!")

	resp, err := authedClient(t, token).Get(testServer.URL + "/recommendation-strategies")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// ─── PUT /recommendation-strategies/:id ──────────────────────────────────────

func TestPutRecommendationStrategy_AsAdmin_UpdatesConfig(t *testing.T) {
	truncate(t)
	registerUser(t, "strat_admin2", "strat_admin2@example.com", "SecurePass1!")
	makeAdmin(t, "strat_admin2")
	token := loginUser(t, "strat_admin2", "SecurePass1!")
	client := authedClient(t, token)

	// recommendation_strategy_config is seeded by migrations and not truncated
	var stratID string
	err := testPool.QueryRow(context.Background(),
		`SELECT id FROM recommendation_strategy_config ORDER BY sort_order LIMIT 1`).Scan(&stratID)
	require.NoError(t, err, "recommendation_strategy_config should be seeded")

	req, _ := http.NewRequest(http.MethodPut,
		testServer.URL+"/recommendation-strategies/"+stratID,
		strings.NewReader("label=Updated+Label&sort_order=1&is_active=true"),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
