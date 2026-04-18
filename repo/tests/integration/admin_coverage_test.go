package integration_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── GET /admin/users/:id ────────────────────────────────────────────────────

func TestGetAdminUserDetail_AsAdmin_Returns200(t *testing.T) {
	truncate(t)
	registerUser(t, "usrdet_admin1", "usrdet_admin1@example.com", "SecurePass1!")
	registerUser(t, "usrdet_target1", "usrdet_target1@example.com", "SecurePass1!")
	makeAdmin(t, "usrdet_admin1")

	adminToken := loginUser(t, "usrdet_admin1", "SecurePass1!")

	var targetID string
	err := testPool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE username='usrdet_target1'`).Scan(&targetID)
	require.NoError(t, err)

	resp, err := authedClient(t, adminToken).Get(
		testServer.URL + "/admin/users/" + targetID)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestGetAdminUserDetail_AsRegularUser_Forbidden(t *testing.T) {
	truncate(t)
	registerUser(t, "usrdet_reg1", "usrdet_reg1@example.com", "SecurePass1!")
	registerUser(t, "usrdet_target2", "usrdet_target2@example.com", "SecurePass1!")

	token := loginUser(t, "usrdet_reg1", "SecurePass1!")

	var targetID string
	testPool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE username='usrdet_target2'`).Scan(&targetID)

	resp, err := authedClient(t, token).Get(testServer.URL + "/admin/users/" + targetID)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestGetAdminUserDetail_NonExistentUser_Returns404(t *testing.T) {
	truncate(t)
	registerUser(t, "usrdet_admin2", "usrdet_admin2@example.com", "SecurePass1!")
	makeAdmin(t, "usrdet_admin2")
	token := loginUser(t, "usrdet_admin2", "SecurePass1!")

	resp, err := authedClient(t, token).Get(
		testServer.URL + "/admin/users/00000000-0000-0000-0000-000000000000")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// ─── POST /admin/users/:id/unlock ────────────────────────────────────────────

func TestPostUnlockUser_AsAdmin_ClearsLockout(t *testing.T) {
	truncate(t)
	registerUser(t, "unlock_admin1", "unlock_admin1@example.com", "SecurePass1!")
	registerUser(t, "unlock_target1", "unlock_target1@example.com", "SecurePass1!")
	makeAdmin(t, "unlock_admin1")

	adminToken := loginUser(t, "unlock_admin1", "SecurePass1!")

	var targetID string
	err := testPool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE username='unlock_target1'`).Scan(&targetID)
	require.NoError(t, err)

	// Simulate lockout by setting failed_login_count and locked_until
	_, err = testPool.Exec(context.Background(),
		`UPDATE users SET failed_login_count=5, locked_until=NOW()+INTERVAL '30 minutes'
		 WHERE id=$1`, targetID)
	require.NoError(t, err)

	client := authedClient(t, adminToken)
	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/admin/users/%s/unlock", testServer.URL, targetID),
		strings.NewReader(""),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify lockout is cleared
	var failedLoginCount int
	var lockedUntil *string
	testPool.QueryRow(context.Background(),
		`SELECT failed_login_count, locked_until::text FROM users WHERE id=$1`, targetID).
		Scan(&failedLoginCount, &lockedUntil)
	assert.Equal(t, 0, failedLoginCount)
	assert.Nil(t, lockedUntil, "locked_until should be cleared")
}

func TestPostUnlockUser_AsRegularUser_Forbidden(t *testing.T) {
	truncate(t)
	registerUser(t, "unlock_reg1", "unlock_reg1@example.com", "SecurePass1!")
	registerUser(t, "unlock_target2", "unlock_target2@example.com", "SecurePass1!")
	token := loginUser(t, "unlock_reg1", "SecurePass1!")

	var targetID string
	testPool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE username='unlock_target2'`).Scan(&targetID)

	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/admin/users/%s/unlock", testServer.URL, targetID),
		strings.NewReader(""),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := authedClient(t, token).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestPostUnlockUser_LockedOutUser_CanLoginAfterUnlock(t *testing.T) {
	truncate(t)
	registerUser(t, "unlock_admin3", "unlock_admin3@example.com", "SecurePass1!")
	registerUser(t, "unlock_locked3", "unlock_locked3@example.com", "SecurePass1!")
	makeAdmin(t, "unlock_admin3")

	adminToken := loginUser(t, "unlock_admin3", "SecurePass1!")

	var targetID string
	err := testPool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE username='unlock_locked3'`).Scan(&targetID)
	require.NoError(t, err)

	// Lock the user out manually
	_, err = testPool.Exec(context.Background(),
		`UPDATE users SET failed_login_count=5, locked_until=NOW()+INTERVAL '1 hour'
		 WHERE id=$1`, targetID)
	require.NoError(t, err)

	// Verify login fails while locked
	loginResp, err := publicClient(t).PostForm(testServer.URL+"/login", map[string][]string{
		"username": {"unlock_locked3"},
		"password": {"SecurePass1!"},
	})
	require.NoError(t, err)
	loginResp.Body.Close()
	assert.Equal(t, http.StatusUnprocessableEntity, loginResp.StatusCode,
		"expected login to fail while locked out")

	// Admin unlocks the user
	client := authedClient(t, adminToken)
	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/admin/users/%s/unlock", testServer.URL, targetID),
		strings.NewReader(""),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	unlockResp, err := client.Do(req)
	require.NoError(t, err)
	unlockResp.Body.Close()
	require.Equal(t, http.StatusOK, unlockResp.StatusCode)

	// Verify login now succeeds
	loginResp2, err := publicClient(t).PostForm(testServer.URL+"/login", map[string][]string{
		"username": {"unlock_locked3"},
		"password": {"SecurePass1!"},
	})
	require.NoError(t, err)
	defer loginResp2.Body.Close()
	assert.True(t, loginResp2.StatusCode == http.StatusOK || loginResp2.StatusCode == http.StatusFound,
		"expected successful login after unlock, got %d", loginResp2.StatusCode)
}
