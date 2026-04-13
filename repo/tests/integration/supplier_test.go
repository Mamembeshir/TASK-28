package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/eduexchange/eduexchange/internal/model"
	supplierrepo "github.com/eduexchange/eduexchange/internal/repository/supplier"
	supplierservice "github.com/eduexchange/eduexchange/internal/service/supplier"
	appcron "github.com/eduexchange/eduexchange/internal/cron"
	engagementrepo "github.com/eduexchange/eduexchange/internal/repository/engagement"
	gamificationrepo "github.com/eduexchange/eduexchange/internal/repository/gamification"
	gamificationservice "github.com/eduexchange/eduexchange/internal/service/gamification"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Helpers ────────────────────────────────────────────────────────────────────

func makeSupplierRole(t *testing.T, username string) {
	t.Helper()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO user_roles (user_id, role, created_at)
		 SELECT id, 'SUPPLIER', NOW() FROM users WHERE username = $1 ON CONFLICT DO NOTHING`,
		username,
	)
	require.NoError(t, err)
}

func createSupplierDirect(t *testing.T, name string) string {
	t.Helper()
	ctx := context.Background()
	repo := supplierrepo.New(testPool)
	svc := supplierservice.NewSupplierService(repo, nil, []byte("test-encryption-key-32-bytes!!!!"))
	s, err := svc.CreateSupplier(ctx, uuid.Nil, name, "test@example.com", "tes****@****.***")
	require.NoError(t, err)
	return s.ID.String()
}

func linkUserToSupplier(t *testing.T, username, supplierID string) {
	t.Helper()
	_, err := testPool.Exec(context.Background(),
		`UPDATE suppliers SET user_id = (SELECT id FROM users WHERE username = $1) WHERE id = $2`,
		username, supplierID,
	)
	require.NoError(t, err)
}

func createOrderViaAdmin(t *testing.T, adminToken, supplierID string) string {
	t.Helper()
	formData := url.Values{
		"supplier_id": {supplierID},
		"sku":         {"SKU-001"},
		"description": {"Test Product"},
		"quantity":    {"10"},
		"unit_price":  {"99.99"},
	}
	req, _ := http.NewRequest("POST", testServer.URL+"/supplier/orders", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session_token", Value: adminToken})

	client := &http.Client{CheckRedirect: func(r *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode, "create order should return 201")

	body, _ := io.ReadAll(resp.Body)
	var order model.SupplierOrder
	require.NoError(t, json.Unmarshal(body, &order))
	return order.ID.String()
}

// ── Tests ─────────────────────────────────────────────────────────────────────

// TestCreateOrder_Success — admin creates order → CREATED status
func TestCreateOrder_Success(t *testing.T) {
	truncate(t)

	registerUser(t, "admin_co", "admin_co@test.com", "Admin@12345678")
	makeAdmin(t, "admin_co")
	adminToken := loginUser(t, "admin_co", "Admin@12345678")

	supplierID := createSupplierDirect(t, "Test Supplier")

	formData := url.Values{
		"supplier_id": {supplierID},
		"sku":         {"SKU-001"},
		"description": {"Test Product"},
		"quantity":    {"10"},
		"unit_price":  {"99.99"},
	}
	req, _ := http.NewRequest("POST", testServer.URL+"/supplier/orders", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session_token", Value: adminToken})

	client := &http.Client{CheckRedirect: func(r *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var order model.SupplierOrder
	require.NoError(t, json.Unmarshal(body, &order))
	assert.Equal(t, model.OrderStatusCreated, order.Status)
	assert.True(t, strings.HasPrefix(order.OrderNumber, "ORD-"))
}

// TestConfirmDeliveryDate_Success — supplier confirms date → CONFIRMED
func TestConfirmDeliveryDate_Success(t *testing.T) {
	truncate(t)

	registerUser(t, "admin_cdd", "admin_cdd@test.com", "Admin@12345678")
	makeAdmin(t, "admin_cdd")
	adminToken := loginUser(t, "admin_cdd", "Admin@12345678")

	registerUser(t, "sup_cdd", "sup_cdd@test.com", "Supplier@12345678")
	makeSupplierRole(t, "sup_cdd")
	supToken := loginUser(t, "sup_cdd", "Supplier@12345678")

	supplierID := createSupplierDirect(t, "CDD Supplier")
	linkUserToSupplier(t, "sup_cdd", supplierID)
	orderID := createOrderViaAdmin(t, adminToken, supplierID)

	// Supplier confirms delivery date
	formData := url.Values{
		"delivery_date": {time.Now().Add(7 * 24 * time.Hour).Format("2006-01-02")},
	}
	req, _ := http.NewRequest("POST", testServer.URL+"/supplier/orders/"+orderID+"/confirm", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session_token", Value: supToken})

	client := &http.Client{CheckRedirect: func(r *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify order status
	var row struct{ Status string }
	testPool.QueryRow(context.Background(), "SELECT status FROM supplier_orders WHERE id = $1", orderID).Scan(&row.Status)
	assert.Equal(t, "CONFIRMED", row.Status)
}

// TestSubmitASN_Success — supplier submits ASN → SHIPPED
func TestSubmitASN_Success(t *testing.T) {
	truncate(t)

	registerUser(t, "admin_asn", "admin_asn@test.com", "Admin@12345678")
	makeAdmin(t, "admin_asn")
	adminToken := loginUser(t, "admin_asn", "Admin@12345678")

	registerUser(t, "sup_asn", "sup_asn@test.com", "Supplier@12345678")
	makeSupplierRole(t, "sup_asn")
	supToken := loginUser(t, "sup_asn", "Supplier@12345678")

	supplierID := createSupplierDirect(t, "ASN Supplier")
	linkUserToSupplier(t, "sup_asn", supplierID)
	orderID := createOrderViaAdmin(t, adminToken, supplierID)

	// First confirm delivery date
	deliveryDate := time.Now().Add(7 * 24 * time.Hour).Format("2006-01-02")
	formData := url.Values{"delivery_date": {deliveryDate}}
	req, _ := http.NewRequest("POST", testServer.URL+"/supplier/orders/"+orderID+"/confirm", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session_token", Value: supToken})
	client := &http.Client{CheckRedirect: func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Submit ASN
	asnData := url.Values{
		"tracking_info":    {"TRACK-12345"},
		"shipped_at":       {time.Now().Format("2006-01-02")},
		"expected_arrival": {time.Now().Add(5 * 24 * time.Hour).Format("2006-01-02")},
	}
	req2, _ := http.NewRequest("POST", testServer.URL+"/supplier/orders/"+orderID+"/asn", strings.NewReader(asnData.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req2.AddCookie(&http.Cookie{Name: "session_token", Value: supToken})
	resp2, err := client.Do(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()

	assert.Equal(t, http.StatusOK, resp2.StatusCode)

	var status string
	testPool.QueryRow(context.Background(), "SELECT status FROM supplier_orders WHERE id = $1", orderID).Scan(&status)
	assert.Equal(t, "SHIPPED", status)
}

// TestConfirmReceipt_Success — admin confirms receipt → RECEIVED
func TestConfirmReceipt_Success(t *testing.T) {
	truncate(t)

	registerUser(t, "admin_rcpt", "admin_rcpt@test.com", "Admin@12345678")
	makeAdmin(t, "admin_rcpt")
	adminToken := loginUser(t, "admin_rcpt", "Admin@12345678")

	registerUser(t, "sup_rcpt", "sup_rcpt@test.com", "Supplier@12345678")
	makeSupplierRole(t, "sup_rcpt")
	supToken := loginUser(t, "sup_rcpt", "Supplier@12345678")

	supplierID := createSupplierDirect(t, "RCPT Supplier")
	linkUserToSupplier(t, "sup_rcpt", supplierID)
	orderID := createOrderViaAdmin(t, adminToken, supplierID)
	client := &http.Client{CheckRedirect: func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}

	// Confirm delivery date
	deliveryDate := time.Now().Add(7 * 24 * time.Hour).Format("2006-01-02")
	req, _ := http.NewRequest("POST", testServer.URL+"/supplier/orders/"+orderID+"/confirm",
		strings.NewReader(url.Values{"delivery_date": {deliveryDate}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session_token", Value: supToken})
	resp, _ := client.Do(req)
	resp.Body.Close()

	// Submit ASN
	asnData := url.Values{"tracking_info": {"TRACK-999"}, "shipped_at": {time.Now().Format("2006-01-02")}}
	req2, _ := http.NewRequest("POST", testServer.URL+"/supplier/orders/"+orderID+"/asn", strings.NewReader(asnData.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req2.AddCookie(&http.Cookie{Name: "session_token", Value: supToken})
	resp2, _ := client.Do(req2)
	resp2.Body.Close()

	// Admin confirms receipt
	req3, _ := http.NewRequest("POST", testServer.URL+"/supplier/orders/"+orderID+"/receive", nil)
	req3.AddCookie(&http.Cookie{Name: "session_token", Value: adminToken})
	resp3, err := client.Do(req3)
	require.NoError(t, err)
	defer resp3.Body.Close()

	assert.Equal(t, http.StatusOK, resp3.StatusCode)

	var status string
	testPool.QueryRow(context.Background(), "SELECT status FROM supplier_orders WHERE id = $1", orderID).Scan(&status)
	assert.Equal(t, "RECEIVED", status)
}

// TestSubmitQCResult_Pass — admin submits QC pass → QC_PASSED
func TestSubmitQCResult_Pass(t *testing.T) {
	truncate(t)

	registerUser(t, "admin_qcp", "admin_qcp@test.com", "Admin@12345678")
	makeAdmin(t, "admin_qcp")
	adminToken := loginUser(t, "admin_qcp", "Admin@12345678")

	registerUser(t, "sup_qcp", "sup_qcp@test.com", "Supplier@12345678")
	makeSupplierRole(t, "sup_qcp")
	supToken := loginUser(t, "sup_qcp", "Supplier@12345678")

	supplierID := createSupplierDirect(t, "QCP Supplier")
	linkUserToSupplier(t, "sup_qcp", supplierID)
	orderID := createOrderViaAdmin(t, adminToken, supplierID)
	client := &http.Client{CheckRedirect: func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}

	advanceOrderToReceived(t, client, adminToken, supToken, orderID)

	// Submit QC Pass
	qcData := url.Values{
		"inspected_units": {"100"},
		"defective_units": {"0"},
		"result":          {"PASS"},
		"notes":           {"All good"},
	}
	req, _ := http.NewRequest("POST", testServer.URL+"/supplier/orders/"+orderID+"/qc", strings.NewReader(qcData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session_token", Value: adminToken})
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var status string
	testPool.QueryRow(context.Background(), "SELECT status FROM supplier_orders WHERE id = $1", orderID).Scan(&status)
	assert.Equal(t, "QC_PASSED", status)
}

// TestSubmitQCResult_Fail — admin submits QC fail → QC_FAILED
func TestSubmitQCResult_Fail(t *testing.T) {
	truncate(t)

	registerUser(t, "admin_qcf", "admin_qcf@test.com", "Admin@12345678")
	makeAdmin(t, "admin_qcf")
	adminToken := loginUser(t, "admin_qcf", "Admin@12345678")

	registerUser(t, "sup_qcf", "sup_qcf@test.com", "Supplier@12345678")
	makeSupplierRole(t, "sup_qcf")
	supToken := loginUser(t, "sup_qcf", "Supplier@12345678")

	supplierID := createSupplierDirect(t, "QCF Supplier")
	linkUserToSupplier(t, "sup_qcf", supplierID)
	orderID := createOrderViaAdmin(t, adminToken, supplierID)
	client := &http.Client{CheckRedirect: func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}

	advanceOrderToReceived(t, client, adminToken, supToken, orderID)

	qcData := url.Values{
		"inspected_units": {"100"},
		"defective_units": {"15"},
		"result":          {"FAIL"},
		"notes":           {"Too many defects"},
	}
	req, _ := http.NewRequest("POST", testServer.URL+"/supplier/orders/"+orderID+"/qc", strings.NewReader(qcData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session_token", Value: adminToken})
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var status string
	testPool.QueryRow(context.Background(), "SELECT status FROM supplier_orders WHERE id = $1", orderID).Scan(&status)
	assert.Equal(t, "QC_FAILED", status)
}

// TestCloseOrder_Success — admin closes QC_PASSED → CLOSED
func TestCloseOrder_Success(t *testing.T) {
	truncate(t)

	registerUser(t, "admin_cls", "admin_cls@test.com", "Admin@12345678")
	makeAdmin(t, "admin_cls")
	adminToken := loginUser(t, "admin_cls", "Admin@12345678")

	registerUser(t, "sup_cls", "sup_cls@test.com", "Supplier@12345678")
	makeSupplierRole(t, "sup_cls")
	supToken := loginUser(t, "sup_cls", "Supplier@12345678")

	supplierID := createSupplierDirect(t, "CLS Supplier")
	linkUserToSupplier(t, "sup_cls", supplierID)
	orderID := createOrderViaAdmin(t, adminToken, supplierID)
	client := &http.Client{CheckRedirect: func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}

	advanceOrderToReceived(t, client, adminToken, supToken, orderID)
	submitQCPass(t, client, adminToken, orderID)

	// Close
	req, _ := http.NewRequest("POST", testServer.URL+"/supplier/orders/"+orderID+"/close", nil)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: adminToken})
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var status string
	testPool.QueryRow(context.Background(), "SELECT status FROM supplier_orders WHERE id = $1", orderID).Scan(&status)
	assert.Equal(t, "CLOSED", status)
}

// TestCancelOrder_Success — admin cancels CREATED → CANCELLED
func TestCancelOrder_Success(t *testing.T) {
	truncate(t)

	registerUser(t, "admin_can", "admin_can@test.com", "Admin@12345678")
	makeAdmin(t, "admin_can")
	adminToken := loginUser(t, "admin_can", "Admin@12345678")

	supplierID := createSupplierDirect(t, "CAN Supplier")
	orderID := createOrderViaAdmin(t, adminToken, supplierID)

	client := &http.Client{CheckRedirect: func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	req, _ := http.NewRequest("POST", testServer.URL+"/supplier/orders/"+orderID+"/cancel", nil)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: adminToken})
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var status string
	testPool.QueryRow(context.Background(), "SELECT status FROM supplier_orders WHERE id = $1", orderID).Scan(&status)
	assert.Equal(t, "CANCELLED", status)
}

// TestKPIRecalculation_Gold — 10 on-time orders with QC pass → Gold
func TestKPIRecalculation_Gold(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	repo := supplierrepo.New(testPool)
	kpiSvc := supplierservice.NewKPIService(repo, nil)

	// Create supplier
	svc := supplierservice.NewSupplierService(repo, nil, []byte("test-encryption-key-32-bytes!!!!"))
	supplier, err := svc.CreateSupplier(ctx, uuid.Nil, "Gold Supplier", "gold@test.com", "gol****@****.***")
	require.NoError(t, err)

	// Register a user for QC submission
	registerUser(t, "admin_kpi_gold", "admin_kpi_gold@test.com", "Admin@12345678")
	var adminID uuid.UUID
	testPool.QueryRow(ctx, "SELECT id FROM users WHERE username = $1", "admin_kpi_gold").Scan(&adminID)

	now := time.Now().UTC()
	deliveryDate := now.Add(7 * 24 * time.Hour)

	// Create 10 orders all on time with QC pass
	for i := 0; i < 10; i++ {
		orderID := uuid.New()
		orderNumber := fmt.Sprintf("ORD-GOLD-%02d", i)
		shippedAt := deliveryDate.Add(-1 * time.Hour) // shipped before delivery date

		_, err = testPool.Exec(ctx, `
			INSERT INTO supplier_orders
			(id, supplier_id, order_number, order_lines, status, delivery_date_confirmed, delivery_date_confirmed_at, received_at, version, created_at, updated_at)
			VALUES ($1, $2, $3, '[]'::jsonb, 'RECEIVED', $4, $5, $6, 1, $7, $7)`,
			orderID, supplier.ID, orderNumber, deliveryDate, now, now, now,
		)
		require.NoError(t, err)

		asnID := uuid.New()
		_, err = testPool.Exec(ctx, `
			INSERT INTO supplier_asns (id, order_id, tracking_info, shipped_at, submitted_at)
			VALUES ($1, $2, 'TRACK', $3, $4)`,
			asnID, orderID, shippedAt, now,
		)
		require.NoError(t, err)

		qcID := uuid.New()
		_, err = testPool.Exec(ctx, `
			INSERT INTO supplier_qc_results
			(id, order_id, inspected_units, defective_units, defect_rate_pct, result, notes, submitted_at, submitted_by)
			VALUES ($1, $2, 100, 0, 0, 'PASS', '', $3, $4)`,
			qcID, orderID, now, adminID,
		)
		require.NoError(t, err)
	}

	kpi, err := kpiSvc.RecalculateKPIs(ctx, uuid.Nil, supplier.ID)
	require.NoError(t, err)

	assert.GreaterOrEqual(t, kpi.OnTimeDeliveryPct, 95.0, "OTD should be >= 95")
	assert.LessOrEqual(t, kpi.StockoutRatePct, 2.0, "Stockout should be <= 2")
	assert.LessOrEqual(t, kpi.ReturnRatePct, 1.0, "Return rate should be <= 1")
	assert.LessOrEqual(t, kpi.DefectRatePct, 0.5, "Defect rate should be <= 0.5")
	assert.Equal(t, model.SupplierTierGold, kpi.TierAssigned, "Should be Gold tier")
}

// TestKPIRecalculation_Bronze — orders with many cancellations/failures → Bronze
func TestKPIRecalculation_Bronze(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	repo := supplierrepo.New(testPool)
	kpiSvc := supplierservice.NewKPIService(repo, nil)

	svc := supplierservice.NewSupplierService(repo, nil, []byte("test-encryption-key-32-bytes!!!!"))
	supplier, err := svc.CreateSupplier(ctx, uuid.Nil, "Bronze Supplier", "bronze@test.com", "bro****@****.***")
	require.NoError(t, err)

	registerUser(t, "admin_kpi_bronze", "admin_kpi_bronze@test.com", "Admin@12345678")
	var adminID uuid.UUID
	testPool.QueryRow(ctx, "SELECT id FROM users WHERE username = $1", "admin_kpi_bronze").Scan(&adminID)

	now := time.Now().UTC()

	// Create 5 cancelled orders (high stockout)
	for i := 0; i < 5; i++ {
		orderID := uuid.New()
		_, err = testPool.Exec(ctx, `
			INSERT INTO supplier_orders
			(id, supplier_id, order_number, order_lines, status, version, created_at, updated_at)
			VALUES ($1, $2, $3, '[]'::jsonb, 'CANCELLED', 1, $4, $4)`,
			orderID, supplier.ID, fmt.Sprintf("ORD-BRONZE-C%02d", i), now,
		)
		require.NoError(t, err)
	}

	// Create 5 orders with QC fail and high defect rate
	for i := 0; i < 5; i++ {
		orderID := uuid.New()
		_, err = testPool.Exec(ctx, `
			INSERT INTO supplier_orders
			(id, supplier_id, order_number, order_lines, status, received_at, version, created_at, updated_at)
			VALUES ($1, $2, $3, '[]'::jsonb, 'QC_FAILED', $4, 1, $4, $4)`,
			orderID, supplier.ID, fmt.Sprintf("ORD-BRONZE-F%02d", i), now,
		)
		require.NoError(t, err)

		qcID := uuid.New()
		_, err = testPool.Exec(ctx, `
			INSERT INTO supplier_qc_results
			(id, order_id, inspected_units, defective_units, defect_rate_pct, result, notes, submitted_at, submitted_by)
			VALUES ($1, $2, 100, 30, 30, 'FAIL', 'Many defects', $3, $4)`,
			qcID, orderID, now, adminID,
		)
		require.NoError(t, err)
	}

	kpi, err := kpiSvc.RecalculateKPIs(ctx, uuid.Nil, supplier.ID)
	require.NoError(t, err)

	assert.Equal(t, model.SupplierTierBronze, kpi.TierAssigned, "Should be Bronze tier")
	assert.Greater(t, kpi.StockoutRatePct, 2.0, "Stockout rate should be above target")
}

// TestDeliveryEscalation_FlagsOrders — old CREATED order → run escalation → flag created
func TestDeliveryEscalation_FlagsOrders(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	repo := supplierrepo.New(testPool)
	svc := supplierservice.NewSupplierService(repo, nil, []byte("test-encryption-key-32-bytes!!!!"))
	supplier, err := svc.CreateSupplier(ctx, uuid.Nil, "Escalation Supplier", "esc@test.com", "esc****@****.***")
	require.NoError(t, err)

	// Create order that is 50h old in CREATED status
	orderID := uuid.New()
	createdAt := time.Now().UTC().Add(-50 * time.Hour)
	_, err = testPool.Exec(ctx, `
		INSERT INTO supplier_orders
		(id, supplier_id, order_number, order_lines, status, version, created_at, updated_at)
		VALUES ($1, $2, 'ORD-ESC-001', '[]'::jsonb, 'CREATED', 1, $3, $3)`,
		orderID, supplier.ID, createdAt,
	)
	require.NoError(t, err)

	// Count flags before
	var flagsBefore int
	testPool.QueryRow(ctx, "SELECT COUNT(*) FROM anomaly_flags WHERE evidence_json->>'type' = 'delivery_confirmation_overdue'").Scan(&flagsBefore)

	// Set up cron scheduler and run escalation
	gamRepo := gamificationrepo.New(testPool)
	rankSvc := gamificationservice.NewRankingService(gamRepo)
	engRepo := engagementrepo.New(testPool)
	kpiSvc := supplierservice.NewKPIService(repo, nil)

	scheduler := appcron.New(rankSvc, engRepo, testPool, kpiSvc, repo, nil, nil, nil)
	scheduler.RunDeliveryEscalation()

	var flagsAfter int
	testPool.QueryRow(ctx, "SELECT COUNT(*) FROM anomaly_flags WHERE evidence_json->>'type' = 'delivery_confirmation_overdue'").Scan(&flagsAfter)

	assert.Greater(t, flagsAfter, flagsBefore, "Should have created at least one escalation flag")
}

// TestSupplierUserSeesOwnOrdersOnly — two suppliers each see only their own orders
func TestSupplierUserSeesOwnOrdersOnly(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	repo := supplierrepo.New(testPool)

	// Create admin
	registerUser(t, "admin_iso", "admin_iso@test.com", "Admin@12345678")
	makeAdmin(t, "admin_iso")
	adminToken := loginUser(t, "admin_iso", "Admin@12345678")

	// Create supplier 1 user
	registerUser(t, "sup_iso1", "sup_iso1@test.com", "Supplier@12345678")
	makeSupplierRole(t, "sup_iso1")
	sup1Token := loginUser(t, "sup_iso1", "Supplier@12345678")

	// Create supplier 2 user
	registerUser(t, "sup_iso2", "sup_iso2@test.com", "Supplier@12345678")
	makeSupplierRole(t, "sup_iso2")
	_ = loginUser(t, "sup_iso2", "Supplier@12345678")

	// Create two supplier entities
	svc := supplierservice.NewSupplierService(repo, nil, []byte("test-encryption-key-32-bytes!!!!"))
	sup1, _ := svc.CreateSupplier(ctx, uuid.Nil, "Supplier 1", "s1@test.com", "s1****@****.***")
	sup2, _ := svc.CreateSupplier(ctx, uuid.Nil, "Supplier 2", "s2@test.com", "s2****@****.***")

	linkUserToSupplier(t, "sup_iso1", sup1.ID.String())
	linkUserToSupplier(t, "sup_iso2", sup2.ID.String())

	// Create order for supplier 1
	_ = createOrderViaAdmin(t, adminToken, sup1.ID.String())
	// Create order for supplier 2
	_ = createOrderViaAdmin(t, adminToken, sup2.ID.String())

	// Supplier 1 sees only their orders
	client := &http.Client{CheckRedirect: func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	req, _ := http.NewRequest("GET", testServer.URL+"/supplier/orders", nil)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: sup1Token})
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Orders []model.SupplierOrder `json:"orders"`
		Total  int                   `json:"total"`
	}
	err = json.Unmarshal(body, &result)
	require.NoError(t, err)

	assert.Equal(t, 1, result.Total, "Supplier 1 should see only 1 order")
	for _, o := range result.Orders {
		assert.Equal(t, sup1.ID, o.SupplierID, "All orders should belong to supplier 1")
	}
}

// ── Helper functions ──────────────────────────────────────────────────────────

func advanceOrderToReceived(t *testing.T, client *http.Client, adminToken, supToken, orderID string) {
	t.Helper()

	// Confirm delivery date
	deliveryDate := time.Now().Add(7 * 24 * time.Hour).Format("2006-01-02")
	req, _ := http.NewRequest("POST", testServer.URL+"/supplier/orders/"+orderID+"/confirm",
		strings.NewReader(url.Values{"delivery_date": {deliveryDate}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session_token", Value: supToken})
	resp, err := client.Do(req)
	require.NoError(t, err)
	resp.Body.Close()

	// Submit ASN
	asnData := url.Values{
		"tracking_info": {"TRACK-ADV"},
		"shipped_at":    {time.Now().Format("2006-01-02")},
	}
	req2, _ := http.NewRequest("POST", testServer.URL+"/supplier/orders/"+orderID+"/asn", strings.NewReader(asnData.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req2.AddCookie(&http.Cookie{Name: "session_token", Value: supToken})
	resp2, err := client.Do(req2)
	require.NoError(t, err)
	resp2.Body.Close()

	// Confirm receipt
	req3, _ := http.NewRequest("POST", testServer.URL+"/supplier/orders/"+orderID+"/receive", nil)
	req3.AddCookie(&http.Cookie{Name: "session_token", Value: adminToken})
	resp3, err := client.Do(req3)
	require.NoError(t, err)
	resp3.Body.Close()
}

func submitQCPass(t *testing.T, client *http.Client, adminToken, orderID string) {
	t.Helper()
	qcData := url.Values{
		"inspected_units": {"100"},
		"defective_units": {"0"},
		"result":          {"PASS"},
		"notes":           {"All good"},
	}
	req, _ := http.NewRequest("POST", testServer.URL+"/supplier/orders/"+orderID+"/qc", strings.NewReader(qcData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "session_token", Value: adminToken})
	resp, err := client.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
}

// ─── Validation error regression tests ───────────────────────────────────────
// These tests guard against the recursive handleServiceError bug (Finding 1).
// An ErrValidation must return 422 Unprocessable Entity, never cause a stack
// overflow or return a 500.

// TestCreateSupplier_EmptyName_Returns422 verifies that creating a supplier
// with a blank name returns 422 (not 500 or a stack overflow).
func TestCreateSupplier_EmptyName_Returns422(t *testing.T) {
	truncate(t)

	registerUser(t, "sup_val1", "sup_val1@example.com", "SecurePass1!")
	makeAdmin(t, "sup_val1")
	adminToken := loginUser(t, "sup_val1", "SecurePass1!")

	resp, err := authedClient(adminToken).PostForm(testServer.URL+"/suppliers", url.Values{
		"name":    {""}, // empty — triggers ErrValidation
		"contact": {"test@example.com"},
	})
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode,
		"empty supplier name must return 422, not 500 or stack overflow")
}

// ensure bytes import is used
var _ = bytes.NewBuffer
