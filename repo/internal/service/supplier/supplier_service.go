package supplierservice

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"time"

	"github.com/eduexchange/eduexchange/internal/audit"
	"github.com/eduexchange/eduexchange/internal/model"
	supplierrepo "github.com/eduexchange/eduexchange/internal/repository/supplier"
	"github.com/google/uuid"
)

// NotificationSender is the interface for sending notifications from the supplier service.
type NotificationSender interface {
	Send(ctx context.Context, userID uuid.UUID, eventType model.EventType, title, body string, resourceID *uuid.UUID) error
}

// SupplierService handles business logic for supplier management.
type SupplierService struct {
	repo          supplierrepo.SupplierRepository
	auditSvc      *audit.Service
	notifSvc      NotificationSender
	encryptionKey []byte // 32-byte AES-256 key
	// adminUserID is optionally set to notify admins of supplier shipments.
	adminFinderFn func(ctx context.Context) []uuid.UUID
}

// NewSupplierService creates a new SupplierService.
// encryptionKey must be exactly 32 bytes for AES-256-GCM.
func NewSupplierService(repo supplierrepo.SupplierRepository, auditSvc *audit.Service, encryptionKey []byte) *SupplierService {
	return &SupplierService{repo: repo, auditSvc: auditSvc, encryptionKey: encryptionKey}
}

// SetNotificationSender wires in the notification service after construction.
func (s *SupplierService) SetNotificationSender(n NotificationSender, adminFinderFn func(ctx context.Context) []uuid.UUID) {
	s.notifSvc = n
	s.adminFinderFn = adminFinderFn
}

// ── Contact helpers ────────────────────────────────────────────────────────────

// EncryptContact encrypts the plain contact string using AES-256-GCM.
// The output is base64(nonce || ciphertext || tag).
func (s *SupplierService) EncryptContact(plain string) (string, error) {
	block, err := aes.NewCipher(s.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("encrypt contact: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("encrypt contact: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("encrypt contact: %w", err)
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptContact decrypts a value produced by EncryptContact.
func (s *SupplierService) DecryptContact(enc string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", fmt.Errorf("decrypt contact: %w", err)
	}
	block, err := aes.NewCipher(s.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("decrypt contact: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("decrypt contact: %w", err)
	}
	if len(data) < gcm.NonceSize() {
		return "", fmt.Errorf("decrypt contact: ciphertext too short")
	}
	nonce, ciphertext := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt contact: %w", err)
	}
	return string(plaintext), nil
}

// MaskContact returns first 3 chars + "****@****.***".
func MaskContact(plain string) string {
	if len(plain) <= 3 {
		return "****"
	}
	return plain[:3] + "****@****.***"
}

// ── Supplier operations ────────────────────────────────────────────────────────

// CreateSupplier creates a new supplier entity.
// actorID is the admin operator creating the record, used for audit traceability.
func (s *SupplierService) CreateSupplier(ctx context.Context, actorID uuid.UUID, name, contactPlain, contactMask string) (*model.Supplier, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", model.ErrValidation)
	}

	encContact, err := s.EncryptContact(contactPlain)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	supplier := &model.Supplier{
		ID:          uuid.New(),
		Name:        name,
		ContactJSON: encContact,
		ContactMask: contactMask,
		Tier:        model.SupplierTierBronze,
		Status:      model.SupplierStatusActive,
		Version:     1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.repo.CreateSupplier(ctx, supplier); err != nil {
		return nil, err
	}
	if s.auditSvc != nil {
		_ = s.auditSvc.Record(ctx, audit.Entry{
			ActorID:    actorID,
			Action:     "supplier.create",
			EntityType: "supplier",
			EntityID:   supplier.ID,
			AfterData:  map[string]interface{}{"name": supplier.Name, "contact_mask": supplier.ContactMask, "tier": string(supplier.Tier)},
			Source:     "supplier",
		})
	}
	return supplier, nil
}

// GetSupplier retrieves a supplier by ID.
// For admin callers the contact is decrypted and returned in ContactJSON as plaintext.
// For non-admin callers ContactJSON is cleared.
func (s *SupplierService) GetSupplier(ctx context.Context, id uuid.UUID, isAdmin bool) (*model.Supplier, error) {
	supplier, err := s.repo.GetSupplier(ctx, id)
	if err != nil {
		return nil, err
	}
	if !isAdmin {
		supplier.ContactJSON = ""
		return supplier, nil
	}
	if supplier.ContactJSON != "" {
		plain, err := s.DecryptContact(supplier.ContactJSON)
		if err != nil {
			return nil, err
		}
		supplier.ContactJSON = plain
	}
	return supplier, nil
}

// UpdateSupplier persists supplier changes.
// actorID is the admin operator performing the update, used for audit traceability.
func (s *SupplierService) UpdateSupplier(ctx context.Context, actorID uuid.UUID, supplier *model.Supplier, isAdmin bool) error {
	if !isAdmin {
		return model.ErrForbidden
	}
	before, _ := s.repo.GetSupplier(ctx, supplier.ID)
	if err := s.repo.UpdateSupplier(ctx, supplier); err != nil {
		return err
	}
	if s.auditSvc != nil {
		var beforeData interface{}
		if before != nil {
			beforeData = map[string]interface{}{"name": before.Name, "tier": string(before.Tier), "status": string(before.Status)}
		}
		_ = s.auditSvc.Record(ctx, audit.Entry{
			ActorID:    actorID,
			Action:     "supplier.update",
			EntityType: "supplier",
			EntityID:   supplier.ID,
			BeforeData: beforeData,
			AfterData:  map[string]interface{}{"name": supplier.Name, "tier": string(supplier.Tier), "status": string(supplier.Status)},
			Source:     "supplier",
		})
	}
	return nil
}

// ListSuppliers returns all suppliers.
func (s *SupplierService) ListSuppliers(ctx context.Context) ([]model.Supplier, error) {
	return s.repo.ListSuppliers(ctx)
}

// ── Order operations ───────────────────────────────────────────────────────────

// CreateOrder creates a new supplier order with generated order number.
// actorID is the operator (admin or supplier user) initiating the order, used for audit traceability.
func (s *SupplierService) CreateOrder(ctx context.Context, actorID, supplierID uuid.UUID, lines []model.OrderLine) (*model.SupplierOrder, error) {
	now := time.Now().UTC()
	id := uuid.New()
	orderNumber := fmt.Sprintf("ORD-%s-%s", now.Format("20060102"), id.String()[:8])

	order := &model.SupplierOrder{
		ID:          id,
		SupplierID:  supplierID,
		OrderNumber: orderNumber,
		OrderLines:  lines,
		Status:      model.OrderStatusCreated,
		Version:     1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.repo.CreateOrder(ctx, order); err != nil {
		return nil, err
	}
	_ = s.auditSvc.Record(ctx, audit.Entry{
		ActorID:    actorID,
		Action:     "order.create",
		EntityType: "supplier_order",
		EntityID:   order.ID,
		AfterData:  map[string]interface{}{"order_number": order.OrderNumber, "status": string(order.Status)},
		Source:     "supplier",
	})
	return order, nil
}

// GetOrder retrieves an order by ID with ASN and QC results populated.
func (s *SupplierService) GetOrder(ctx context.Context, orderID uuid.UUID) (*model.SupplierOrder, error) {
	return s.repo.GetOrder(ctx, orderID)
}

// ListOrders lists orders with optional filtering.
func (s *SupplierService) ListOrders(ctx context.Context, supplierID *uuid.UUID, status string, page, pageSize int) ([]model.SupplierOrder, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	return s.repo.ListOrders(ctx, supplierID, status, page, pageSize)
}

// ConfirmDeliveryDate transitions an order from CREATED → CONFIRMED.
// All suppliers must confirm within the universal 48-hour SLA window.
func (s *SupplierService) ConfirmDeliveryDate(ctx context.Context, orderID uuid.UUID, deliveryDate time.Time, supplierID uuid.UUID) error {
	order, err := s.repo.GetOrder(ctx, orderID)
	if err != nil {
		return err
	}
	if order.Status != model.OrderStatusCreated {
		return fmt.Errorf("%w: order must be in CREATED status", model.ErrValidation)
	}
	if order.SupplierID != supplierID {
		return model.ErrForbidden
	}

	// Universal 48-hour SLA: all tiers must confirm within 48h of order creation.
	const confirmWindowHours = 48
	if time.Since(order.CreatedAt) > confirmWindowHours*time.Hour {
		return fmt.Errorf("%w: confirmation window of 48h has passed; contact support for override",
			model.ErrValidation)
	}

	now := time.Now().UTC()
	order.Status = model.OrderStatusConfirmed
	order.DeliveryDateConfirmed = &deliveryDate
	order.DeliveryDateConfirmedAt = &now

	if err := s.repo.UpdateOrder(ctx, order); err != nil {
		return err
	}
	_ = s.auditSvc.Record(ctx, audit.Entry{
		ActorID:    supplierID,
		Action:     "order.confirm_delivery_date",
		EntityType: "supplier_order",
		EntityID:   orderID,
		AfterData:  map[string]interface{}{"status": string(order.Status), "delivery_date": deliveryDate},
		Source:     "supplier",
	})
	return nil
}

// SubmitASN transitions an order from CONFIRMED → SHIPPED and creates ASN.
func (s *SupplierService) SubmitASN(ctx context.Context, orderID uuid.UUID, trackingInfo string, shippedAt time.Time, expectedArrival *time.Time, supplierID uuid.UUID) error {
	order, err := s.repo.GetOrder(ctx, orderID)
	if err != nil {
		return err
	}
	if order.Status != model.OrderStatusConfirmed {
		return fmt.Errorf("%w: order must be in CONFIRMED status", model.ErrValidation)
	}
	if order.SupplierID != supplierID {
		return model.ErrForbidden
	}

	now := time.Now().UTC()
	asn := &model.SupplierASN{
		ID:              uuid.New(),
		OrderID:         orderID,
		TrackingInfo:    trackingInfo,
		ShippedAt:       shippedAt,
		ExpectedArrival: expectedArrival,
		SubmittedAt:     now,
	}
	if err := s.repo.CreateASN(ctx, asn); err != nil {
		return err
	}

	order.Status = model.OrderStatusShipped
	if err := s.repo.UpdateOrder(ctx, order); err != nil {
		return err
	}
	_ = s.auditSvc.Record(ctx, audit.Entry{
		ActorID:    supplierID,
		Action:     "order.submit_asn",
		EntityType: "supplier_order",
		EntityID:   orderID,
		AfterData:  map[string]interface{}{"status": string(order.Status), "tracking_info": trackingInfo},
		Source:     "supplier",
	})

	// Notify admins of new shipment.
	if s.notifSvc != nil && s.adminFinderFn != nil {
		adminIDs := s.adminFinderFn(ctx)
		for _, adminID := range adminIDs {
			_ = s.notifSvc.Send(ctx, adminID, model.EventSupplierShipment,
				"Supplier Shipment Update",
				fmt.Sprintf("Order %s has been shipped. Tracking: %s", order.OrderNumber, trackingInfo),
				nil)
		}
	}
	return nil
}

// ConfirmReceipt transitions an order from SHIPPED → RECEIVED.
func (s *SupplierService) ConfirmReceipt(ctx context.Context, orderID uuid.UUID, adminID uuid.UUID) error {
	order, err := s.repo.GetOrder(ctx, orderID)
	if err != nil {
		return err
	}
	if order.Status != model.OrderStatusShipped {
		return fmt.Errorf("%w: order must be in SHIPPED status", model.ErrValidation)
	}

	now := time.Now().UTC()
	order.Status = model.OrderStatusReceived
	order.ReceivedAt = &now

	if err := s.repo.UpdateOrder(ctx, order); err != nil {
		return err
	}
	_ = s.auditSvc.Record(ctx, audit.Entry{
		ActorID:    adminID,
		Action:     "order.confirm_receipt",
		EntityType: "supplier_order",
		EntityID:   orderID,
		AfterData:  map[string]interface{}{"status": string(order.Status)},
		Source:     "supplier",
	})
	return nil
}

// SubmitQCResult submits QC and transitions to QC_PASSED or QC_FAILED.
func (s *SupplierService) SubmitQCResult(ctx context.Context, orderID uuid.UUID, inspected, defective int, result model.QCResultType, notes string, submittedBy uuid.UUID) error {
	order, err := s.repo.GetOrder(ctx, orderID)
	if err != nil {
		return err
	}
	if order.Status != model.OrderStatusReceived {
		return fmt.Errorf("%w: order must be in RECEIVED status", model.ErrValidation)
	}

	// Must be within 24h of receipt
	if order.ReceivedAt != nil {
		deadline := order.ReceivedAt.Add(24 * time.Hour)
		if time.Now().UTC().After(deadline) {
			return fmt.Errorf("%w: QC result must be submitted within 24h of receipt", model.ErrValidation)
		}
	}

	defectRatePct := 0.0
	if inspected > 0 {
		defectRatePct = float64(defective) / float64(inspected) * 100
	}

	now := time.Now().UTC()
	qc := &model.SupplierQCResult{
		ID:             uuid.New(),
		OrderID:        orderID,
		InspectedUnits: inspected,
		DefectiveUnits: defective,
		DefectRatePct:  defectRatePct,
		Result:         result,
		Notes:          notes,
		SubmittedAt:    now,
		SubmittedBy:    submittedBy,
	}
	if err := s.repo.CreateQCResult(ctx, qc); err != nil {
		return err
	}

	if result == model.QCResultPass {
		order.Status = model.OrderStatusQCPassed
	} else {
		order.Status = model.OrderStatusQCFailed
	}
	if err := s.repo.UpdateOrder(ctx, order); err != nil {
		return err
	}
	_ = s.auditSvc.Record(ctx, audit.Entry{
		ActorID:    submittedBy,
		Action:     "order.submit_qc",
		EntityType: "supplier_order",
		EntityID:   orderID,
		AfterData:  map[string]interface{}{"status": string(order.Status), "result": string(result), "defect_rate_pct": defectRatePct},
		Source:     "supplier",
	})
	return nil
}

// CloseOrder transitions an order from QC_PASSED/QC_FAILED → CLOSED.
func (s *SupplierService) CloseOrder(ctx context.Context, orderID, adminID uuid.UUID) error {
	order, err := s.repo.GetOrder(ctx, orderID)
	if err != nil {
		return err
	}
	if order.Status != model.OrderStatusQCPassed && order.Status != model.OrderStatusQCFailed {
		return fmt.Errorf("%w: order must be in QC_PASSED or QC_FAILED status", model.ErrValidation)
	}

	order.Status = model.OrderStatusClosed
	if err := s.repo.UpdateOrder(ctx, order); err != nil {
		return err
	}
	_ = s.auditSvc.Record(ctx, audit.Entry{
		ActorID:    adminID,
		Action:     "order.close",
		EntityType: "supplier_order",
		EntityID:   orderID,
		AfterData:  map[string]interface{}{"status": string(order.Status)},
		Source:     "supplier",
	})
	return nil
}

// CancelOrder transitions an order from CREATED/CONFIRMED → CANCELLED.
func (s *SupplierService) CancelOrder(ctx context.Context, orderID uuid.UUID, adminID uuid.UUID) error {
	order, err := s.repo.GetOrder(ctx, orderID)
	if err != nil {
		return err
	}
	if order.Status != model.OrderStatusCreated && order.Status != model.OrderStatusConfirmed {
		return fmt.Errorf("%w: only CREATED or CONFIRMED orders can be cancelled", model.ErrValidation)
	}

	prevStatus := order.Status
	order.Status = model.OrderStatusCancelled
	if err := s.repo.UpdateOrder(ctx, order); err != nil {
		return err
	}
	_ = s.auditSvc.Record(ctx, audit.Entry{
		ActorID:    adminID,
		Action:     "order.cancel",
		EntityType: "supplier_order",
		EntityID:   orderID,
		BeforeData: map[string]interface{}{"status": string(prevStatus)},
		AfterData:  map[string]interface{}{"status": string(order.Status)},
		Source:     "supplier",
	})
	return nil
}
