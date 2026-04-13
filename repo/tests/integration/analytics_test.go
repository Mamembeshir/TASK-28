package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAnalyticsDashboard_AsAdmin(t *testing.T) {
	truncate(t)
	registerUser(t, "analyticsadmin1", "analyticsadmin1@test.com", "Password@123456")
	makeAdmin(t, "analyticsadmin1")
	token := loginUser(t, "analyticsadmin1", "Password@123456")
	client := authedClient(token)

	req, _ := http.NewRequest("GET", testServer.URL+"/analytics/dashboard", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.NotNil(t, body["metrics"])
}

func TestGetAnalyticsDashboard_AsRegularUser(t *testing.T) {
	truncate(t)
	registerUser(t, "analyticsuser1", "analyticsuser1@test.com", "Password@123456")
	token := loginUser(t, "analyticsuser1", "Password@123456")
	client := authedClient(token)

	req, _ := http.NewRequest("GET", testServer.URL+"/analytics/dashboard", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Regular users can access the dashboard but see limited metrics
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestGetReportList_RequiresAdmin(t *testing.T) {
	truncate(t)
	registerUser(t, "analyticsuser2", "analyticsuser2@test.com", "Password@123456")
	token := loginUser(t, "analyticsuser2", "Password@123456")
	client := authedClient(token)

	resp, err := client.Get(testServer.URL + "/analytics/reports")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestPostGenerateReport_AsAdmin(t *testing.T) {
	truncate(t)
	registerUser(t, "analyticsadmin2", "analyticsadmin2@test.com", "Password@123456")
	makeAdmin(t, "analyticsadmin2")
	token := loginUser(t, "analyticsadmin2", "Password@123456")
	userID := getUserIDByUsername(t, "analyticsadmin2")
	_ = userID

	client := authedClient(token)
	body := strings.NewReader("report_type=ANALYTICS")
	req, _ := http.NewRequest("POST", testServer.URL+"/analytics/reports/generate", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	assert.Contains(t, result, "id")

	// Verify a report record was created in DB
	var count int
	err = testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM scheduled_reports`).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestGetAuditLogs_AdminOnly(t *testing.T) {
	truncate(t)
	// Register non-admin
	registerUser(t, "audituser1", "audituser1@test.com", "Password@123456")
	token := loginUser(t, "audituser1", "Password@123456")
	client := authedClient(token)

	resp, err := client.Get(testServer.URL + "/audit-logs")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestGetAuditLogs_AdminCanAccess(t *testing.T) {
	truncate(t)
	registerUser(t, "auditadmin1", "auditadmin1@test.com", "Password@123456")
	makeAdmin(t, "auditadmin1")
	token := loginUser(t, "auditadmin1", "Password@123456")
	client := authedClient(token)

	req, _ := http.NewRequest("GET", testServer.URL+"/audit-logs", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Contains(t, body, "entries")
}

func TestExportAuditLog_AdminOnly(t *testing.T) {
	truncate(t)
	registerUser(t, "auditadmin2", "auditadmin2@test.com", "Password@123456")
	makeAdmin(t, "auditadmin2")
	token := loginUser(t, "auditadmin2", "Password@123456")
	client := authedClient(token)

	resp, err := client.Post(testServer.URL+"/audit-logs/export", "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Contains(t, body, "file_path")
}
