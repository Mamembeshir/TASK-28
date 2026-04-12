package supplierhandler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/eduexchange/eduexchange/internal/middleware"
	"github.com/eduexchange/eduexchange/internal/model"
	supplierrepo "github.com/eduexchange/eduexchange/internal/repository/supplier"
	supplierservice "github.com/eduexchange/eduexchange/internal/service/supplier"
	supplierpages "github.com/eduexchange/eduexchange/internal/templ/pages/supplier"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Handler handles supplier HTTP endpoints.
type Handler struct {
	svc     *supplierservice.SupplierService
	kpiSvc  *supplierservice.KPIService
	repo    supplierrepo.SupplierRepository
}

// New creates a new supplier Handler.
func New(svc *supplierservice.SupplierService, kpiSvc *supplierservice.KPIService, repo supplierrepo.SupplierRepository) *Handler {
	return &Handler{svc: svc, kpiSvc: kpiSvc, repo: repo}
}

func respondError(c *gin.Context, status int, msg string) {
	if isHTMLRequest(c) {
		c.Data(status, "text/html; charset=utf-8", []byte("<p>Error: "+msg+"</p>"))
		return
	}
	c.JSON(status, gin.H{"error": msg})
}

func isHTMLRequest(c *gin.Context) bool {
	return c.GetHeader("HX-Request") == "true" || strings.Contains(c.GetHeader("Accept"), "text/html")
}

func isAdmin(c *gin.Context) bool {
	user := middleware.GetAuthUser(c)
	if user == nil {
		return false
	}
	for _, r := range user.Roles {
		if r == "ADMIN" {
			return true
		}
	}
	return false
}

func isSupplierUser(c *gin.Context) bool {
	user := middleware.GetAuthUser(c)
	if user == nil {
		return false
	}
	for _, r := range user.Roles {
		if r == "SUPPLIER" {
			return true
		}
	}
	return false
}

// ── Supplier CRUD ─────────────────────────────────────────────────────────────

// GetSupplierList handles GET /suppliers
func (h *Handler) GetSupplierList(c *gin.Context) {
	user := middleware.GetAuthUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	suppliers, err := h.svc.ListSuppliers(c.Request.Context())
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	if isHTMLRequest(c) {
		data := supplierpages.SupplierListData{
			Suppliers: suppliers,
			AuthUser:  user,
		}
		c.Status(http.StatusOK)
		supplierpages.SupplierListPage(data).Render(c.Request.Context(), c.Writer)
		return
	}
	c.JSON(http.StatusOK, suppliers)
}

// PostCreateSupplier handles POST /suppliers
func (h *Handler) PostCreateSupplier(c *gin.Context) {
	if !isAdmin(c) {
		respondError(c, http.StatusForbidden, "admin only")
		return
	}

	name := c.PostForm("name")
	contactPlain := c.PostForm("contact")
	contactMask := supplierservice.MaskContact(contactPlain)

	supplier, err := h.svc.CreateSupplier(c.Request.Context(), name, contactPlain, contactMask)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	c.JSON(http.StatusCreated, supplier)
}

// GetSupplierDetail handles GET /suppliers/:id
func (h *Handler) GetSupplierDetail(c *gin.Context) {
	user := middleware.GetAuthUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	supplierID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid supplier id")
		return
	}

	admin := isAdmin(c)
	supplier, err := h.svc.GetSupplier(c.Request.Context(), supplierID, admin)
	if err != nil {
		if err == model.ErrNotFound {
			respondError(c, http.StatusNotFound, "supplier not found")
			return
		}
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	orders, _, err := h.svc.ListOrders(c.Request.Context(), &supplierID, "", 1, 50)
	if err != nil {
		orders = []model.SupplierOrder{}
	}

	kpi, _ := h.repo.GetLatestKPI(c.Request.Context(), supplierID)

	if isHTMLRequest(c) {
		data := supplierpages.SupplierDetailData{
			Supplier: supplier,
			Orders:   orders,
			KPI:      kpi,
			IsAdmin:  admin,
			AuthUser: user,
		}
		c.Status(http.StatusOK)
		supplierpages.SupplierDetailPage(data).Render(c.Request.Context(), c.Writer)
		return
	}
	c.JSON(http.StatusOK, supplier)
}

// ── Order operations ───────────────────────────────────────────────────────────

// GetOrderList handles GET /supplier/orders
func (h *Handler) GetOrderList(c *gin.Context) {
	user := middleware.GetAuthUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	status := c.Query("status")

	var supplierID *uuid.UUID

	// Supplier users only see their own orders
	if isSupplierUser(c) && !isAdmin(c) {
		supplier, err := h.repo.GetSupplierByUserID(c.Request.Context(), user.ID)
		if err != nil {
			respondError(c, http.StatusForbidden, "no supplier linked to this account")
			return
		}
		supplierID = &supplier.ID
	} else if sid := c.Query("supplier_id"); sid != "" {
		parsed, err := uuid.Parse(sid)
		if err == nil {
			supplierID = &parsed
		}
	}

	orders, total, err := h.svc.ListOrders(c.Request.Context(), supplierID, status, page, pageSize)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	if isHTMLRequest(c) {
		data := supplierpages.OrderListData{
			Orders:   orders,
			Total:    total,
			Page:     page,
			AuthUser: user,
		}
		c.Status(http.StatusOK)
		supplierpages.OrderListPage(data).Render(c.Request.Context(), c.Writer)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"orders": orders,
		"total":  total,
		"page":   page,
	})
}

// PostCreateOrder handles POST /supplier/orders
func (h *Handler) PostCreateOrder(c *gin.Context) {
	if !isAdmin(c) {
		respondError(c, http.StatusForbidden, "admin only")
		return
	}

	supplierIDStr := c.PostForm("supplier_id")
	supplierID, err := uuid.Parse(supplierIDStr)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid supplier_id")
		return
	}

	// Parse order lines from JSON body or form
	var lines []model.OrderLine
	if err := c.ShouldBindJSON(&struct{ Lines *[]model.OrderLine }{Lines: &lines}); err != nil {
		// Try form data with basic single line
		sku := c.PostForm("sku")
		desc := c.PostForm("description")
		qty, _ := strconv.Atoi(c.PostForm("quantity"))
		price, _ := strconv.ParseFloat(c.PostForm("unit_price"), 64)
		if sku != "" {
			lines = []model.OrderLine{{SKU: sku, Description: desc, Quantity: qty, UnitPrice: price}}
		}
	}

	order, err := h.svc.CreateOrder(c.Request.Context(), supplierID, lines)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusCreated, order)
}

// GetOrderDetail handles GET /supplier/orders/:id
func (h *Handler) GetOrderDetail(c *gin.Context) {
	user := middleware.GetAuthUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	orderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid order id")
		return
	}

	order, err := h.svc.GetOrder(c.Request.Context(), orderID)
	if err != nil {
		if err == model.ErrNotFound {
			respondError(c, http.StatusNotFound, "order not found")
			return
		}
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	// Supplier users can only see their own orders
	if isSupplierUser(c) && !isAdmin(c) {
		supplier, err := h.repo.GetSupplierByUserID(c.Request.Context(), user.ID)
		if err != nil || supplier.ID != order.SupplierID {
			respondError(c, http.StatusForbidden, "forbidden")
			return
		}
	}

	if isHTMLRequest(c) {
		data := supplierpages.OrderDetailData{
			Order:      order,
			IsAdmin:    isAdmin(c),
			IsSupplier: isSupplierUser(c),
			AuthUser:   user,
		}
		c.Status(http.StatusOK)
		supplierpages.OrderDetailPage(data).Render(c.Request.Context(), c.Writer)
		return
	}
	c.JSON(http.StatusOK, order)
}

// PutConfirmDeliveryDate handles PUT /supplier/orders/:id/confirm
func (h *Handler) PutConfirmDeliveryDate(c *gin.Context) {
	user := middleware.GetAuthUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	orderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid order id")
		return
	}

	dateStr := c.PostForm("delivery_date")
	deliveryDate, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		// Try full datetime
		deliveryDate, err = time.Parse(time.RFC3339, dateStr)
		if err != nil {
			respondError(c, http.StatusBadRequest, "invalid delivery_date format, use YYYY-MM-DD")
			return
		}
	}

	// Get the supplier linked to this user (if supplier role)
	var supplierID uuid.UUID
	if isAdmin(c) {
		// Admin can act on behalf - get from order
		order, err := h.svc.GetOrder(c.Request.Context(), orderID)
		if err != nil {
			respondError(c, http.StatusNotFound, "order not found")
			return
		}
		supplierID = order.SupplierID
	} else {
		supplier, err := h.repo.GetSupplierByUserID(c.Request.Context(), user.ID)
		if err != nil {
			respondError(c, http.StatusForbidden, "no supplier linked to this account")
			return
		}
		supplierID = supplier.ID
	}

	if err := h.svc.ConfirmDeliveryDate(c.Request.Context(), orderID, deliveryDate, supplierID); err != nil {
		if err == model.ErrForbidden {
			respondError(c, http.StatusForbidden, "forbidden")
			return
		}
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "confirmed"})
}

// PostSubmitASN handles POST /supplier/orders/:id/asn
func (h *Handler) PostSubmitASN(c *gin.Context) {
	user := middleware.GetAuthUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	orderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid order id")
		return
	}

	trackingInfo := c.PostForm("tracking_info")
	shippedAtStr := c.PostForm("shipped_at")
	shippedAt, err := time.Parse("2006-01-02", shippedAtStr)
	if err != nil {
		shippedAt, err = time.Parse(time.RFC3339, shippedAtStr)
		if err != nil {
			respondError(c, http.StatusBadRequest, "invalid shipped_at format")
			return
		}
	}

	var expectedArrival *time.Time
	if ea := c.PostForm("expected_arrival"); ea != "" {
		t, err := time.Parse("2006-01-02", ea)
		if err == nil {
			expectedArrival = &t
		}
	}

	var supplierID uuid.UUID
	if isAdmin(c) {
		order, err := h.svc.GetOrder(c.Request.Context(), orderID)
		if err != nil {
			respondError(c, http.StatusNotFound, "order not found")
			return
		}
		supplierID = order.SupplierID
	} else {
		supplier, err := h.repo.GetSupplierByUserID(c.Request.Context(), user.ID)
		if err != nil {
			respondError(c, http.StatusForbidden, "no supplier linked to this account")
			return
		}
		supplierID = supplier.ID
	}

	if err := h.svc.SubmitASN(c.Request.Context(), orderID, trackingInfo, shippedAt, expectedArrival, supplierID); err != nil {
		if err == model.ErrForbidden {
			respondError(c, http.StatusForbidden, "forbidden")
			return
		}
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "shipped"})
}

// PostConfirmReceipt handles POST /supplier/orders/:id/receive
func (h *Handler) PostConfirmReceipt(c *gin.Context) {
	if !isAdmin(c) {
		respondError(c, http.StatusForbidden, "admin only")
		return
	}

	user := middleware.GetAuthUser(c)
	orderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid order id")
		return
	}

	if err := h.svc.ConfirmReceipt(c.Request.Context(), orderID, user.ID); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "received"})
}

// PostSubmitQCResult handles POST /supplier/orders/:id/qc
func (h *Handler) PostSubmitQCResult(c *gin.Context) {
	if !isAdmin(c) {
		respondError(c, http.StatusForbidden, "admin only")
		return
	}

	user := middleware.GetAuthUser(c)
	orderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid order id")
		return
	}

	inspected, _ := strconv.Atoi(c.PostForm("inspected_units"))
	defective, _ := strconv.Atoi(c.PostForm("defective_units"))
	resultStr := c.PostForm("result")
	notes := c.PostForm("notes")

	result, err := model.ParseQCResultType(resultStr)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid result: must be PASS or FAIL")
		return
	}

	if err := h.svc.SubmitQCResult(c.Request.Context(), orderID, inspected, defective, result, notes, user.ID); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "qc_submitted"})
}

// PostCloseOrder handles POST /supplier/orders/:id/close
func (h *Handler) PostCloseOrder(c *gin.Context) {
	if !isAdmin(c) {
		respondError(c, http.StatusForbidden, "admin only")
		return
	}

	orderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid order id")
		return
	}

	if err := h.svc.CloseOrder(c.Request.Context(), orderID); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "closed"})
}

// PostCancelOrder handles POST /supplier/orders/:id/cancel
func (h *Handler) PostCancelOrder(c *gin.Context) {
	if !isAdmin(c) {
		respondError(c, http.StatusForbidden, "admin only")
		return
	}

	user := middleware.GetAuthUser(c)
	orderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid order id")
		return
	}

	if err := h.svc.CancelOrder(c.Request.Context(), orderID, user.ID); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "cancelled"})
}

// ── KPI operations ────────────────────────────────────────────────────────────

// GetKPIDashboard handles GET /suppliers/:id/kpis
func (h *Handler) GetKPIDashboard(c *gin.Context) {
	user := middleware.GetAuthUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	supplierID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid supplier id")
		return
	}

	supplier, err := h.svc.GetSupplier(c.Request.Context(), supplierID, isAdmin(c))
	if err != nil {
		respondError(c, http.StatusNotFound, "supplier not found")
		return
	}

	kpi, _ := h.repo.GetLatestKPI(c.Request.Context(), supplierID)

	if isHTMLRequest(c) {
		data := supplierpages.KPIDashboardData{
			KPI:      kpi,
			Supplier: supplier,
			AuthUser: user,
		}
		c.Status(http.StatusOK)
		supplierpages.KPIDashboardPage(data).Render(c.Request.Context(), c.Writer)
		return
	}
	c.JSON(http.StatusOK, kpi)
}

// PostRecalculateKPIs handles POST /suppliers/:id/kpis/recalculate
func (h *Handler) PostRecalculateKPIs(c *gin.Context) {
	if !isAdmin(c) {
		respondError(c, http.StatusForbidden, "admin only")
		return
	}

	supplierID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid supplier id")
		return
	}

	kpi, err := h.kpiSvc.RecalculateKPIs(c.Request.Context(), supplierID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusOK, kpi)
}

// ── Portal (Supplier User View) ────────────────────────────────────────────────

// GetPortal handles GET /supplier/portal
func (h *Handler) GetPortal(c *gin.Context) {
	user := middleware.GetAuthUser(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	supplier, err := h.repo.GetSupplierByUserID(c.Request.Context(), user.ID)
	if err != nil {
		respondError(c, http.StatusNotFound, "no supplier linked to this account")
		return
	}

	orders, _, err := h.svc.ListOrders(c.Request.Context(), &supplier.ID, "", 1, 50)
	if err != nil {
		orders = []model.SupplierOrder{}
	}

	kpi, _ := h.repo.GetLatestKPI(c.Request.Context(), supplier.ID)

	if isHTMLRequest(c) {
		data := supplierpages.PortalData{
			Supplier: supplier,
			Orders:   orders,
			KPI:      kpi,
			AuthUser: user,
		}
		c.Status(http.StatusOK)
		supplierpages.PortalPage(data).Render(c.Request.Context(), c.Writer)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"supplier": supplier,
		"orders":   orders,
		"kpi":      kpi,
	})
}

// GetOrderForm handles GET /supplier/orders/new
func (h *Handler) GetOrderForm(c *gin.Context) {
	if !isAdmin(c) {
		respondError(c, http.StatusForbidden, "admin only")
		return
	}

	user := middleware.GetAuthUser(c)
	supplierIDStr := c.Query("supplier_id")
	supplierID, _ := uuid.Parse(supplierIDStr)

	data := supplierpages.OrderFormData{
		SupplierID: supplierID,
		AuthUser:   user,
	}
	c.Status(http.StatusOK)
	supplierpages.OrderFormPage(data).Render(c.Request.Context(), c.Writer)
}
