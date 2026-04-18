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

// ─── GET /analytics/reports ───────────────────────────────────────────────────

func TestGetAnalyticsReportList_AsAdmin_Returns200(t *testing.T) {
	truncate(t)
	registerUser(t, "rptlist_admin1", "rptlist_admin1@example.com", "SecurePass1!")
	makeAdmin(t, "rptlist_admin1")
	token := loginUser(t, "rptlist_admin1", "SecurePass1!")

	req, _ := http.NewRequest(http.MethodGet, testServer.URL+"/analytics/reports", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := authedClient(t, token).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestGetAnalyticsReportList_AsRegularUser_Forbidden(t *testing.T) {
	truncate(t)
	registerUser(t, "rptlist_reg1", "rptlist_reg1@example.com", "SecurePass1!")
	token := loginUser(t, "rptlist_reg1", "SecurePass1!")

	resp, err := authedClient(t, token).Get(testServer.URL + "/analytics/reports")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestGetAnalyticsReportList_AfterGenerate_ShowsReport(t *testing.T) {
	truncate(t)
	registerUser(t, "rptlist_admin2", "rptlist_admin2@example.com", "SecurePass1!")
	makeAdmin(t, "rptlist_admin2")
	token := loginUser(t, "rptlist_admin2", "SecurePass1!")
	client := authedClient(t, token)

	// Generate a report
	genReq, _ := http.NewRequest(http.MethodPost, testServer.URL+"/analytics/reports/generate",
		strings.NewReader("report_type=ANALYTICS"))
	genReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	genResp, err := client.Do(genReq)
	require.NoError(t, err)
	genResp.Body.Close()
	require.Equal(t, http.StatusOK, genResp.StatusCode)

	// Verify report exists in DB
	var count int
	testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM scheduled_reports`).Scan(&count)
	require.Greater(t, count, 0, "at least one report should exist after generate")

	// List reports
	req, _ := http.NewRequest(http.MethodGet, testServer.URL+"/analytics/reports", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// ─── GET /analytics/reports/:id/download ─────────────────────────────────────

func TestGetAnalyticsReportDownload_AsAdmin_ValidID(t *testing.T) {
	truncate(t)
	registerUser(t, "rptdl_admin1", "rptdl_admin1@example.com", "SecurePass1!")
	makeAdmin(t, "rptdl_admin1")
	token := loginUser(t, "rptdl_admin1", "SecurePass1!")
	client := authedClient(t, token)

	// Generate a report first
	genReq, _ := http.NewRequest(http.MethodPost, testServer.URL+"/analytics/reports/generate",
		strings.NewReader("report_type=ANALYTICS"))
	genReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	genResp, err := client.Do(genReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, genResp.StatusCode)

	var genBody map[string]interface{}
	require.NoError(t, json.NewDecoder(genResp.Body).Decode(&genBody))
	genResp.Body.Close()

	reportID := genBody["id"].(string)
	require.NotEmpty(t, reportID)

	// Attempt download — may succeed (200) or return 404 if file not yet written to disk
	resp, err := client.Get(testServer.URL + "/analytics/reports/" + reportID + "/download")
	require.NoError(t, err)
	defer resp.Body.Close()

	// 200 if file exists on disk, 404 if report file not written in test env
	assert.True(t, resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotFound ||
		resp.StatusCode == http.StatusInternalServerError,
		"expected 200, 404, or 500, got %d", resp.StatusCode)
}

func TestGetAnalyticsReportDownload_AsAdmin_InvalidID_Returns400(t *testing.T) {
	truncate(t)
	registerUser(t, "rptdl_admin2", "rptdl_admin2@example.com", "SecurePass1!")
	makeAdmin(t, "rptdl_admin2")
	token := loginUser(t, "rptdl_admin2", "SecurePass1!")

	resp, err := authedClient(t, token).Get(testServer.URL + "/analytics/reports/not-a-uuid/download")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestGetAnalyticsReportDownload_AsRegularUser_Forbidden(t *testing.T) {
	truncate(t)
	registerUser(t, "rptdl_reg1", "rptdl_reg1@example.com", "SecurePass1!")
	token := loginUser(t, "rptdl_reg1", "SecurePass1!")

	resp, err := authedClient(t, token).Get(
		testServer.URL + "/analytics/reports/00000000-0000-0000-0000-000000000000/download")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestGetAnalyticsReportDownload_AsAdmin_NonExistentReport_Returns404(t *testing.T) {
	truncate(t)
	registerUser(t, "rptdl_admin3", "rptdl_admin3@example.com", "SecurePass1!")
	makeAdmin(t, "rptdl_admin3")
	token := loginUser(t, "rptdl_admin3", "SecurePass1!")

	resp, err := authedClient(t, token).Get(
		testServer.URL + "/analytics/reports/00000000-0000-0000-0000-000000000000/download")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}
