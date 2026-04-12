package supplierrepo

import (
	"context"
	"time"

	"github.com/eduexchange/eduexchange/internal/model"
	"github.com/google/uuid"
)

// SupplierRepository defines the data access interface for supplier operations.
type SupplierRepository interface {
	// Supplier CRUD
	CreateSupplier(ctx context.Context, s *model.Supplier) error
	GetSupplier(ctx context.Context, id uuid.UUID) (*model.Supplier, error)
	UpdateSupplier(ctx context.Context, s *model.Supplier) error
	UpdateSupplierTier(ctx context.Context, supplierID uuid.UUID, tier model.SupplierTier) error
	ListSuppliers(ctx context.Context) ([]model.Supplier, error)
	GetSupplierByUserID(ctx context.Context, userID uuid.UUID) (*model.Supplier, error)
	GetAllActiveSuppliers(ctx context.Context) ([]model.Supplier, error)

	// Orders
	CreateOrder(ctx context.Context, o *model.SupplierOrder) error
	GetOrder(ctx context.Context, id uuid.UUID) (*model.SupplierOrder, error)
	UpdateOrder(ctx context.Context, o *model.SupplierOrder) error
	ListOrders(ctx context.Context, supplierID *uuid.UUID, status string, page, pageSize int) ([]model.SupplierOrder, int, error)
	GetOrdersInPeriod(ctx context.Context, supplierID uuid.UUID, start, end time.Time) ([]model.SupplierOrder, error)
	GetOrdersAwaitingConfirmation(ctx context.Context, deadline time.Time) ([]model.SupplierOrder, error)
	GetOrdersAwaitingQC(ctx context.Context, deadline time.Time) ([]model.SupplierOrder, error)

	// ASN
	CreateASN(ctx context.Context, asn *model.SupplierASN) error
	GetASNByOrderID(ctx context.Context, orderID uuid.UUID) (*model.SupplierASN, error)

	// QC Results
	CreateQCResult(ctx context.Context, qc *model.SupplierQCResult) error
	GetQCResultByOrderID(ctx context.Context, orderID uuid.UUID) (*model.SupplierQCResult, error)

	// KPIs
	SaveKPI(ctx context.Context, kpi *model.SupplierKPI) error
	GetLatestKPI(ctx context.Context, supplierID uuid.UUID) (*model.SupplierKPI, error)
	ListKPIs(ctx context.Context, supplierID uuid.UUID, limit int) ([]model.SupplierKPI, error)
}
