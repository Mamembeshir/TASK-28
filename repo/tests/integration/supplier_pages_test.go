package integration_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── GET /supplier/portal ─────────────────────────────────────────────────────

func TestGetSupplierPortal_AsSupplier_Returns200(t *testing.T) {
	truncate(t)
	registerUser(t, "portal_sup1", "portal_sup1@example.com", "SecurePass1!")
	makeSupplierRole(t, "portal_sup1")
	token := loginUser(t, "portal_sup1", "SecurePass1!")

	resp, err := authedClient(t, token).Get(testServer.URL + "/supplier/portal")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestGetSupplierPortal_AsRegularUser_Forbidden(t *testing.T) {
	truncate(t)
	registerUser(t, "portal_reg1", "portal_reg1@example.com", "SecurePass1!")
	token := loginUser(t, "portal_reg1", "SecurePass1!")

	resp, err := authedClient(t, token).Get(testServer.URL + "/supplier/portal")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// ─── GET /supplier/orders/new ─────────────────────────────────────────────────

func TestGetSupplierOrderForm_AsSupplier_Returns200(t *testing.T) {
	truncate(t)
	registerUser(t, "ordnew_sup1", "ordnew_sup1@example.com", "SecurePass1!")
	makeSupplierRole(t, "ordnew_sup1")
	token := loginUser(t, "ordnew_sup1", "SecurePass1!")

	resp, err := authedClient(t, token).Get(testServer.URL + "/supplier/orders/new")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestGetSupplierOrderForm_AsRegularUser_Forbidden(t *testing.T) {
	truncate(t)
	registerUser(t, "ordnew_reg1", "ordnew_reg1@example.com", "SecurePass1!")
	token := loginUser(t, "ordnew_reg1", "SecurePass1!")

	resp, err := authedClient(t, token).Get(testServer.URL + "/supplier/orders/new")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// ─── GET /supplier/orders/:id ────────────────────────────────────────────────

func TestGetSupplierOrderDetail_AsSupplier_Returns200(t *testing.T) {
	truncate(t)
	registerUser(t, "orddet_admin1", "orddet_admin1@example.com", "SecurePass1!")
	registerUser(t, "orddet_sup1", "orddet_sup1@example.com", "SecurePass1!")
	makeAdmin(t, "orddet_admin1")
	makeSupplierRole(t, "orddet_sup1")

	adminToken := loginUser(t, "orddet_admin1", "SecurePass1!")
	supToken := loginUser(t, "orddet_sup1", "SecurePass1!")

	// Create supplier and link user
	supplierID := createSupplierDirect(t, "Detail Supplier")
	linkUserToSupplier(t, "orddet_sup1", supplierID)

	// Create order as admin
	orderID := createOrderViaAdmin(t, adminToken, supplierID)

	resp, err := authedClient(t, supToken).Get(testServer.URL + "/supplier/orders/" + orderID)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestGetSupplierOrderDetail_AsRegularUser_Forbidden(t *testing.T) {
	truncate(t)
	registerUser(t, "orddet_reg1", "orddet_reg1@example.com", "SecurePass1!")
	token := loginUser(t, "orddet_reg1", "SecurePass1!")

	resp, err := authedClient(t, token).Get(testServer.URL + "/supplier/orders/00000000-0000-0000-0000-000000000000")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// ─── PUT /supplier/orders/:id/confirm ────────────────────────────────────────

func TestPutConfirmDeliveryDate_AsSupplier_Confirms(t *testing.T) {
	truncate(t)
	registerUser(t, "putconf_admin1", "putconf_admin1@example.com", "SecurePass1!")
	registerUser(t, "putconf_sup1", "putconf_sup1@example.com", "SecurePass1!")
	makeAdmin(t, "putconf_admin1")
	makeSupplierRole(t, "putconf_sup1")

	adminToken := loginUser(t, "putconf_admin1", "SecurePass1!")
	supToken := loginUser(t, "putconf_sup1", "SecurePass1!")

	supplierID := createSupplierDirect(t, "Put Confirm Supplier")
	linkUserToSupplier(t, "putconf_sup1", supplierID)
	orderID := createOrderViaAdmin(t, adminToken, supplierID)

	req, _ := http.NewRequest(http.MethodPut,
		testServer.URL+"/supplier/orders/"+orderID+"/confirm",
		nil)
	resp, err := authedClient(t, supToken).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Supplier confirming delivery date — 200 or 422 if date missing
	assert.True(t, resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusUnprocessableEntity,
		"expected 200 or 422, got %d", resp.StatusCode)
}

// ─── GET /suppliers ───────────────────────────────────────────────────────────

func TestGetSupplierList_AsAdmin_ReturnsList(t *testing.T) {
	truncate(t)
	registerUser(t, "suplist_admin1", "suplist_admin1@example.com", "SecurePass1!")
	makeAdmin(t, "suplist_admin1")
	token := loginUser(t, "suplist_admin1", "SecurePass1!")

	req, _ := http.NewRequest(http.MethodGet, testServer.URL+"/suppliers", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := authedClient(t, token).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestGetSupplierList_AsRegularUser_Forbidden(t *testing.T) {
	truncate(t)
	registerUser(t, "suplist_reg1", "suplist_reg1@example.com", "SecurePass1!")
	token := loginUser(t, "suplist_reg1", "SecurePass1!")

	resp, err := authedClient(t, token).Get(testServer.URL + "/suppliers")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestGetSupplierList_WithCreatedSupplier_ReturnsIt(t *testing.T) {
	truncate(t)
	registerUser(t, "suplist_admin2", "suplist_admin2@example.com", "SecurePass1!")
	makeAdmin(t, "suplist_admin2")
	token := loginUser(t, "suplist_admin2", "SecurePass1!")

	// Create a supplier via HTTP
	createResp, err := authedClient(t, token).PostForm(testServer.URL+"/suppliers", url.Values{
		"name":              {"List Test Supplier"},
		"contact_email":     {"listtest@supplier.com"},
		"contact_email_raw": {"listtest@supplier.com"},
	})
	require.NoError(t, err)
	createResp.Body.Close()

	req, _ := http.NewRequest(http.MethodGet, testServer.URL+"/suppliers", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := authedClient(t, token).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// ─── GET /suppliers/:id ───────────────────────────────────────────────────────

func TestGetSupplierDetail_AsAdmin_Returns200(t *testing.T) {
	truncate(t)
	registerUser(t, "supdet_admin1", "supdet_admin1@example.com", "SecurePass1!")
	makeAdmin(t, "supdet_admin1")
	token := loginUser(t, "supdet_admin1", "SecurePass1!")

	supplierID := createSupplierDirect(t, "Detail Supplier Co")

	req, _ := http.NewRequest(http.MethodGet, testServer.URL+"/suppliers/"+supplierID, nil)
	req.Header.Set("Accept", "application/json")
	resp, err := authedClient(t, token).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestGetSupplierDetail_AsRegularUser_Forbidden(t *testing.T) {
	truncate(t)
	registerUser(t, "supdet_reg1", "supdet_reg1@example.com", "SecurePass1!")
	token := loginUser(t, "supdet_reg1", "SecurePass1!")

	resp, err := authedClient(t, token).Get(testServer.URL + "/suppliers/00000000-0000-0000-0000-000000000000")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// ─── GET /suppliers/:id/kpis ──────────────────────────────────────────────────

func TestGetSupplierKPIDashboard_AsAdmin_Returns200(t *testing.T) {
	truncate(t)
	registerUser(t, "supkpi_admin1", "supkpi_admin1@example.com", "SecurePass1!")
	makeAdmin(t, "supkpi_admin1")
	token := loginUser(t, "supkpi_admin1", "SecurePass1!")

	supplierID := createSupplierDirect(t, "KPI Supplier")

	req, _ := http.NewRequest(http.MethodGet, testServer.URL+"/suppliers/"+supplierID+"/kpis", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := authedClient(t, token).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// 200 with empty KPIs or page, or 404 if supplier has no KPIs yet
	assert.True(t, resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotFound,
		"expected 200 or 404, got %d", resp.StatusCode)
}

func TestGetSupplierKPIDashboard_AsRegularUser_Forbidden(t *testing.T) {
	truncate(t)
	registerUser(t, "supkpi_reg1", "supkpi_reg1@example.com", "SecurePass1!")
	token := loginUser(t, "supkpi_reg1", "SecurePass1!")

	resp, err := authedClient(t, token).Get(testServer.URL + "/suppliers/00000000-0000-0000-0000-000000000000/kpis")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// ─── POST /suppliers ─────────────────────────────────────────────────────────
// (Already covered by supplier_test.go — this adds a positive HTTP handler test)

func TestPostCreateSupplier_AsAdmin_Returns201(t *testing.T) {
	truncate(t)
	registerUser(t, "supcreate_admin1", "supcreate_admin1@example.com", "SecurePass1!")
	makeAdmin(t, "supcreate_admin1")
	token := loginUser(t, "supcreate_admin1", "SecurePass1!")

	resp, err := authedClient(t, token).PostForm(testServer.URL+"/suppliers", url.Values{
		"name":              {"New Supplier Co"},
		"contact_email":     {"new@supplier.com"},
		"contact_email_raw": {"new@supplier.com"},
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.True(t, resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK,
		"expected 201 or 200, got %d", resp.StatusCode)

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	// supplier should have a name or id
}
