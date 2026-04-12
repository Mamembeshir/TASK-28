package frontend_test

import (
	"testing"
	"time"

	"github.com/eduexchange/eduexchange/internal/middleware"
	"github.com/eduexchange/eduexchange/internal/model"
	supplierpages "github.com/eduexchange/eduexchange/internal/templ/pages/supplier"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

var testAuthUser = &middleware.AuthUser{
	ID:       uuid.New(),
	Username: "testuser",
	Email:    "test@test.com",
	Roles:    []string{"ADMIN"},
}

// TestSupplierListPage_RendersTierBadge — Gold/Silver/Bronze badges render
func TestSupplierListPage_RendersTierBadge(t *testing.T) {
	suppliers := []model.Supplier{
		{ID: uuid.New(), Name: "Gold Corp", Tier: model.SupplierTierGold, Status: model.SupplierStatusActive},
		{ID: uuid.New(), Name: "Silver Co", Tier: model.SupplierTierSilver, Status: model.SupplierStatusActive},
		{ID: uuid.New(), Name: "Bronze Ltd", Tier: model.SupplierTierBronze, Status: model.SupplierStatusActive},
	}

	data := supplierpages.SupplierListData{
		Suppliers: suppliers,
		AuthUser:  testAuthUser,
	}
	html := render(t, supplierpages.SupplierListPage(data))

	assert.Contains(t, html, "tier-badge-gold", "Gold badge should be rendered")
	assert.Contains(t, html, "tier-badge-silver", "Silver badge should be rendered")
	assert.Contains(t, html, "tier-badge-bronze", "Bronze badge should be rendered")
	assert.Contains(t, html, "GOLD")
	assert.Contains(t, html, "SILVER")
	assert.Contains(t, html, "BRONZE")
}

// TestSupplierDetailPage_MasksContactForNonAdmin — non-admin sees masked contact
func TestSupplierDetailPage_MasksContactForNonAdmin(t *testing.T) {
	supplier := &model.Supplier{
		ID:          uuid.New(),
		Name:        "Test Supplier",
		ContactMask: "tes****@****.***",
		ContactJSON: "dGVzdEBleGFtcGxlLmNvbQ==", // base64 of "test@example.com"
		Tier:        model.SupplierTierBronze,
		Status:      model.SupplierStatusActive,
	}

	nonAdminUser := &middleware.AuthUser{
		ID:       uuid.New(),
		Username: "supplier1",
		Roles:    []string{"SUPPLIER"},
	}

	data := supplierpages.SupplierDetailData{
		Supplier: supplier,
		Orders:   []model.SupplierOrder{},
		IsAdmin:  false,
		AuthUser: nonAdminUser,
	}
	html := render(t, supplierpages.SupplierDetailPage(data))

	assert.Contains(t, html, "supplier-contact-masked", "Should show masked contact element")
	assert.Contains(t, html, "tes****@****.***", "Should show masked value")
	assert.NotContains(t, html, "test@example.com", "Should NOT show full contact for non-admin")
}

// TestSupplierDetailPage_ShowsFullContactForAdmin — admin sees full contact
func TestSupplierDetailPage_ShowsFullContactForAdmin(t *testing.T) {
	supplier := &model.Supplier{
		ID:          uuid.New(),
		Name:        "Test Supplier",
		ContactMask: "tes****@****.***",
		ContactJSON: "dGVzdEBleGFtcGxlLmNvbQ==", // base64 of "test@example.com"
		Tier:        model.SupplierTierBronze,
		Status:      model.SupplierStatusActive,
	}

	data := supplierpages.SupplierDetailData{
		Supplier: supplier,
		Orders:   []model.SupplierOrder{},
		IsAdmin:  true,
		AuthUser: testAuthUser,
	}
	html := render(t, supplierpages.SupplierDetailPage(data))

	assert.Contains(t, html, "supplier-contact-full", "Should show full contact element")
	assert.Contains(t, html, "test@example.com", "Admin should see decrypted contact")
}

// TestOrderDetailPage_ShowsConfirmForm_WhenCreated — CREATED order shows confirm form
func TestOrderDetailPage_ShowsConfirmForm_WhenCreated(t *testing.T) {
	now := time.Now()
	order := &model.SupplierOrder{
		ID:           uuid.New(),
		SupplierID:   uuid.New(),
		OrderNumber:  "ORD-20240101-ABCD1234",
		Status:       model.OrderStatusCreated,
		OrderLines:   []model.OrderLine{},
		Version:      1,
		CreatedAt:    now,
		UpdatedAt:    now,
		SupplierName: "Test Supplier",
	}

	supplierUser := &middleware.AuthUser{
		ID:       uuid.New(),
		Username: "supplier1",
		Roles:    []string{"SUPPLIER"},
	}

	data := supplierpages.OrderDetailData{
		Order:      order,
		IsAdmin:    false,
		IsSupplier: true,
		AuthUser:   supplierUser,
	}
	html := render(t, supplierpages.OrderDetailPage(data))

	assert.Contains(t, html, "confirm-delivery-btn", "Should show confirm delivery button for CREATED + supplier")
	assert.Contains(t, html, "delivery_date", "Should show delivery date input")
}

// TestOrderDetailPage_ShowsASNForm_WhenConfirmed — CONFIRMED order shows ASN form
func TestOrderDetailPage_ShowsASNForm_WhenConfirmed(t *testing.T) {
	now := time.Now()
	deliveryDate := now.Add(7 * 24 * time.Hour)
	order := &model.SupplierOrder{
		ID:                      uuid.New(),
		SupplierID:              uuid.New(),
		OrderNumber:             "ORD-20240101-ABCD5678",
		Status:                  model.OrderStatusConfirmed,
		OrderLines:              []model.OrderLine{},
		DeliveryDateConfirmed:   &deliveryDate,
		DeliveryDateConfirmedAt: &now,
		Version:                 1,
		CreatedAt:               now,
		UpdatedAt:               now,
		SupplierName:            "Test Supplier",
	}

	supplierUser := &middleware.AuthUser{
		ID:       uuid.New(),
		Username: "supplier1",
		Roles:    []string{"SUPPLIER"},
	}

	data := supplierpages.OrderDetailData{
		Order:      order,
		IsAdmin:    false,
		IsSupplier: true,
		AuthUser:   supplierUser,
	}
	html := render(t, supplierpages.OrderDetailPage(data))

	assert.Contains(t, html, "submit-asn-btn", "Should show ASN submit button for CONFIRMED + supplier")
	assert.Contains(t, html, "tracking_info", "Should show tracking info input")
	assert.Contains(t, html, "shipped_at", "Should show shipped_at input")
}

// TestOrderDetailPage_ShowsQCForm_WhenReceived — RECEIVED shows QC form for admin
func TestOrderDetailPage_ShowsQCForm_WhenReceived(t *testing.T) {
	now := time.Now()
	order := &model.SupplierOrder{
		ID:           uuid.New(),
		SupplierID:   uuid.New(),
		OrderNumber:  "ORD-20240101-ABCD9999",
		Status:       model.OrderStatusReceived,
		OrderLines:   []model.OrderLine{},
		ReceivedAt:   &now,
		Version:      1,
		CreatedAt:    now,
		UpdatedAt:    now,
		SupplierName: "Test Supplier",
	}

	data := supplierpages.OrderDetailData{
		Order:      order,
		IsAdmin:    true,
		IsSupplier: false,
		AuthUser:   testAuthUser,
	}
	html := render(t, supplierpages.OrderDetailPage(data))

	assert.Contains(t, html, "submit-qc-btn", "Should show QC submit button for RECEIVED + admin")
	assert.Contains(t, html, "inspected_units", "Should show inspected units input")
	assert.Contains(t, html, "defective_units", "Should show defective units input")
}

// TestOrderDetailPage_ShowsCloseButton_WhenQCPassed — QC_PASSED shows close button
func TestOrderDetailPage_ShowsCloseButton_WhenQCPassed(t *testing.T) {
	now := time.Now()
	order := &model.SupplierOrder{
		ID:           uuid.New(),
		SupplierID:   uuid.New(),
		OrderNumber:  "ORD-20240101-ABCDE001",
		Status:       model.OrderStatusQCPassed,
		OrderLines:   []model.OrderLine{},
		Version:      1,
		CreatedAt:    now,
		UpdatedAt:    now,
		SupplierName: "Test Supplier",
	}

	data := supplierpages.OrderDetailData{
		Order:      order,
		IsAdmin:    true,
		IsSupplier: false,
		AuthUser:   testAuthUser,
	}
	html := render(t, supplierpages.OrderDetailPage(data))

	assert.Contains(t, html, "close-order-btn", "Should show close order button for QC_PASSED + admin")
}

// TestKPIDashboardPage_GreenForOTD_WhenAboveTarget — OTD ≥ 95 renders green class
func TestKPIDashboardPage_GreenForOTD_WhenAboveTarget(t *testing.T) {
	supplier := &model.Supplier{
		ID:   uuid.New(),
		Name: "Green Supplier",
		Tier: model.SupplierTierGold,
	}

	kpi := &model.SupplierKPI{
		ID:                uuid.New(),
		SupplierID:        supplier.ID,
		OnTimeDeliveryPct: 98.5,
		StockoutRatePct:   1.0,
		ReturnRatePct:     0.5,
		DefectRatePct:     0.2,
		TierAssigned:      model.SupplierTierGold,
		PeriodStart:       time.Now().Add(-90 * 24 * time.Hour),
		PeriodEnd:         time.Now(),
		ComputedAt:        time.Now(),
	}

	data := supplierpages.KPIDashboardData{
		KPI:      kpi,
		Supplier: supplier,
		AuthUser: testAuthUser,
	}
	html := render(t, supplierpages.KPIDashboardPage(data))

	assert.Contains(t, html, "kpi-card-green", "OTD >= 95 should use green card class")
	assert.Contains(t, html, "kpi-otd-value", "Should have OTD value element")
	assert.Contains(t, html, "98.5%", "Should show the OTD percentage")
}

// TestKPIDashboardPage_RedForOTD_WhenBelowTarget — OTD < 95 renders red class
func TestKPIDashboardPage_RedForOTD_WhenBelowTarget(t *testing.T) {
	supplier := &model.Supplier{
		ID:   uuid.New(),
		Name: "Red Supplier",
		Tier: model.SupplierTierBronze,
	}

	kpi := &model.SupplierKPI{
		ID:                uuid.New(),
		SupplierID:        supplier.ID,
		OnTimeDeliveryPct: 80.0,
		StockoutRatePct:   5.0,
		ReturnRatePct:     3.0,
		DefectRatePct:     1.5,
		TierAssigned:      model.SupplierTierBronze,
		PeriodStart:       time.Now().Add(-90 * 24 * time.Hour),
		PeriodEnd:         time.Now(),
		ComputedAt:        time.Now(),
	}

	data := supplierpages.KPIDashboardData{
		KPI:      kpi,
		Supplier: supplier,
		AuthUser: testAuthUser,
	}
	html := render(t, supplierpages.KPIDashboardPage(data))

	assert.Contains(t, html, "kpi-card-red", "OTD < 95 should use red card class")
	assert.Contains(t, html, "80.0%", "Should show the OTD percentage")
}

// TestPortalPage_RendersOrders — portal shows orders
func TestPortalPage_RendersOrders(t *testing.T) {
	now := time.Now()
	supplier := &model.Supplier{
		ID:   uuid.New(),
		Name: "Portal Supplier",
		Tier: model.SupplierTierSilver,
	}

	orders := []model.SupplierOrder{
		{
			ID:           uuid.New(),
			SupplierID:   supplier.ID,
			OrderNumber:  "ORD-20240101-PORTAL1",
			Status:       model.OrderStatusCreated,
			OrderLines:   []model.OrderLine{},
			Version:      1,
			CreatedAt:    now,
			UpdatedAt:    now,
			SupplierName: "Portal Supplier",
		},
		{
			ID:           uuid.New(),
			SupplierID:   supplier.ID,
			OrderNumber:  "ORD-20240101-PORTAL2",
			Status:       model.OrderStatusConfirmed,
			OrderLines:   []model.OrderLine{},
			Version:      1,
			CreatedAt:    now,
			UpdatedAt:    now,
			SupplierName: "Portal Supplier",
		},
	}

	supplierUser := &middleware.AuthUser{
		ID:       uuid.New(),
		Username: "portal_supplier",
		Roles:    []string{"SUPPLIER"},
	}

	data := supplierpages.PortalData{
		Supplier: supplier,
		Orders:   orders,
		KPI:      nil,
		AuthUser: supplierUser,
	}
	html := render(t, supplierpages.PortalPage(data))

	assert.Contains(t, html, "ORD-20240101-PORTAL1", "Should show first order")
	assert.Contains(t, html, "ORD-20240101-PORTAL2", "Should show second order")
	assert.Contains(t, html, "Portal Supplier", "Should show supplier name")
}
