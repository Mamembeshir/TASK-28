package integration_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── Registration ─────────────────────────────────────────────────────────────

func TestRegister_Success(t *testing.T) {
	truncate(t)

	resp, err := http.PostForm(testServer.URL+"/register", url.Values{
		"username": {"newuser"},
		"email":    {"newuser@example.com"},
		"password": {"SecurePass1!"},
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	// 422 = page re-render with errors, 200/302 = success
	assert.NotEqual(t, http.StatusUnprocessableEntity, resp.StatusCode,
		"expected successful registration, got 422")
}

func TestRegister_DuplicateUsername(t *testing.T) {
	truncate(t)
	registerUser(t, "dupuser", "dup@example.com", "SecurePass1!")

	resp, err := http.PostForm(testServer.URL+"/register", url.Values{
		"username": {"dupuser"},
		"email":    {"other@example.com"},
		"password": {"SecurePass1!"},
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

func TestRegister_WeakPassword(t *testing.T) {
	truncate(t)

	resp, err := http.PostForm(testServer.URL+"/register", url.Values{
		"username": {"weakpwuser"},
		"email":    {"weak@example.com"},
		"password": {"short"},
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

func TestRegister_InvalidEmail(t *testing.T) {
	truncate(t)

	resp, err := http.PostForm(testServer.URL+"/register", url.Values{
		"username": {"someuser"},
		"email":    {"notanemail"},
		"password": {"SecurePass1!"},
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

// ─── Login ────────────────────────────────────────────────────────────────────

func TestLogin_Success(t *testing.T) {
	truncate(t)
	registerUser(t, "loginuser", "login@example.com", "SecurePass1!")

	client := &http.Client{CheckRedirect: func(r *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse // don't follow redirects
	}}

	resp, err := client.PostForm(testServer.URL+"/login", url.Values{
		"username": {"loginuser"},
		"password": {"SecurePass1!"},
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should redirect (302) or 200 with HX-Redirect
	assert.True(t, resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusOK,
		"expected 302 or 200, got %d", resp.StatusCode)

	// Cookie must be set
	var sessionCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "session_token" {
			sessionCookie = c
			break
		}
	}
	assert.NotNil(t, sessionCookie, "session_token cookie should be set")
	assert.NotEmpty(t, sessionCookie.Value)
}

func TestLogin_WrongPassword(t *testing.T) {
	truncate(t)
	registerUser(t, "loginuser2", "login2@example.com", "SecurePass1!")

	resp, err := http.PostForm(testServer.URL+"/login", url.Values{
		"username": {"loginuser2"},
		"password": {"WrongPassword1!"},
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

func TestLogin_NonExistentUser(t *testing.T) {
	truncate(t)

	resp, err := http.PostForm(testServer.URL+"/login", url.Values{
		"username": {"ghostuser"},
		"password": {"SecurePass1!"},
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

// ─── Lockout (AUTH-02) ────────────────────────────────────────────────────────

func TestLogin_Lockout_After5Failures(t *testing.T) {
	truncate(t)
	registerUser(t, "lockoutuser", "lockout@example.com", "SecurePass1!")

	// 5 bad attempts
	for i := 0; i < 5; i++ {
		resp, err := http.PostForm(testServer.URL+"/login", url.Values{
			"username": {"lockoutuser"},
			"password": {"WrongPass1!"},
		})
		require.NoError(t, err)
		resp.Body.Close()
	}

	// Correct password should now be rejected because of lockout
	resp, err := http.PostForm(testServer.URL+"/login", url.Values{
		"username": {"lockoutuser"},
		"password": {"SecurePass1!"},
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

// ─── Admin: List Users ────────────────────────────────────────────────────────

func TestAdminListUsers_RequiresAdmin(t *testing.T) {
	truncate(t)
	registerUser(t, "regularuser", "regular@example.com", "SecurePass1!")
	token := loginUser(t, "regularuser", "SecurePass1!")

	req, _ := http.NewRequest(http.MethodGet, testServer.URL+"/admin/users", nil)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: token})

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestAdminListUsers_AdminCanAccess(t *testing.T) {
	truncate(t)
	registerUser(t, "adminuser", "admin@example.com", "SecurePass1!")
	makeAdmin(t, "adminuser")
	token := loginUser(t, "adminuser", "SecurePass1!")

	req, _ := http.NewRequest(http.MethodGet, testServer.URL+"/admin/users", nil)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: token})

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// ─── Admin: Status Transition ─────────────────────────────────────────────────

func TestAdminStatusTransition_ActiveToSuspended(t *testing.T) {
	truncate(t)
	registerUser(t, "adminuser", "admin@example.com", "SecurePass1!")
	makeAdmin(t, "adminuser")
	adminToken := loginUser(t, "adminuser", "SecurePass1!")

	registerUser(t, "targetuser", "target@example.com", "SecurePass1!")

	// Get target user ID
	var targetID string
	err := testPool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE username = 'targetuser'`).Scan(&targetID)
	require.NoError(t, err)

	var version int
	err = testPool.QueryRow(context.Background(),
		`SELECT version FROM users WHERE id = $1`, targetID).Scan(&version)
	require.NoError(t, err)

	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/admin/users/%s/status", testServer.URL, targetID),
		strings.NewReader(fmt.Sprintf("status=SUSPENDED&version=%d", version)),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session_token", Value: adminToken})

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify in DB
	var status string
	err = testPool.QueryRow(context.Background(),
		`SELECT status FROM users WHERE id = $1`, targetID).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "SUSPENDED", status)
}

func TestAdminStatusTransition_InvalidTransition_Rejected(t *testing.T) {
	truncate(t)
	registerUser(t, "adminuser", "admin@example.com", "SecurePass1!")
	makeAdmin(t, "adminuser")
	adminToken := loginUser(t, "adminuser", "SecurePass1!")

	registerUser(t, "targetuser", "target@example.com", "SecurePass1!")

	var targetID string
	err := testPool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE username = 'targetuser'`).Scan(&targetID)
	require.NoError(t, err)

	// First: go ACTIVE → DEACTIVATED (terminal)
	_, err = testPool.Exec(context.Background(),
		`UPDATE users SET status = 'DEACTIVATED', version = version + 1 WHERE id = $1`, targetID)
	require.NoError(t, err)

	var version int
	err = testPool.QueryRow(context.Background(),
		`SELECT version FROM users WHERE id = $1`, targetID).Scan(&version)
	require.NoError(t, err)

	// Try DEACTIVATED → ACTIVE (not allowed)
	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/admin/users/%s/status", testServer.URL, targetID),
		strings.NewReader(fmt.Sprintf("status=ACTIVE&version=%d", version)),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session_token", Value: adminToken})

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

// ─── Admin: Role Assignment ────────────────────────────────────────────────────

func TestAdminAssignRole(t *testing.T) {
	truncate(t)
	registerUser(t, "adminuser", "admin@example.com", "SecurePass1!")
	makeAdmin(t, "adminuser")
	adminToken := loginUser(t, "adminuser", "SecurePass1!")

	registerUser(t, "targetuser", "target@example.com", "SecurePass1!")

	var targetID string
	err := testPool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE username = 'targetuser'`).Scan(&targetID)
	require.NoError(t, err)

	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/admin/users/%s/roles/assign", testServer.URL, targetID),
		strings.NewReader("role=AUTHOR"),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session_token", Value: adminToken})

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify in DB
	var count int
	err = testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM user_roles WHERE user_id = $1 AND role = 'AUTHOR'`, targetID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestAdminRemoveRole_LastRole_Rejected(t *testing.T) {
	truncate(t)
	registerUser(t, "adminuser", "admin@example.com", "SecurePass1!")
	makeAdmin(t, "adminuser")
	adminToken := loginUser(t, "adminuser", "SecurePass1!")

	registerUser(t, "targetuser", "target@example.com", "SecurePass1!")

	var targetID string
	err := testPool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE username = 'targetuser'`).Scan(&targetID)
	require.NoError(t, err)

	// targetuser has only REGULAR_USER — try to remove it
	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/admin/users/%s/roles/remove", testServer.URL, targetID),
		strings.NewReader("role=REGULAR_USER"),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session_token", Value: adminToken})

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

// ─── Optimistic Locking ────────────────────────────────────────────────────────

func TestAdminStatusTransition_StaleVersion_Rejected(t *testing.T) {
	truncate(t)
	registerUser(t, "adminuser", "admin@example.com", "SecurePass1!")
	makeAdmin(t, "adminuser")
	adminToken := loginUser(t, "adminuser", "SecurePass1!")

	registerUser(t, "targetuser", "target@example.com", "SecurePass1!")

	var targetID string
	err := testPool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE username = 'targetuser'`).Scan(&targetID)
	require.NoError(t, err)

	// Send stale version (0 instead of actual 1)
	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/admin/users/%s/status", testServer.URL, targetID),
		strings.NewReader("status=SUSPENDED&version=0"),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session_token", Value: adminToken})

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}
