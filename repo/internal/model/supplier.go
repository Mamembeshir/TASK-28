package model

import (
	"time"

	"github.com/google/uuid"
)

type OrderLine struct {
	SKU         string  `json:"sku"`
	Description string  `json:"description"`
	Quantity    int     `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
}

type Supplier struct {
	ID          uuid.UUID      `json:"id"`
	Name        string         `json:"name"`
	ContactJSON string         `json:"contact_json,omitempty"` // base64 encoded, admin only
	ContactMask string         `json:"contact_mask"`
	Tier        SupplierTier   `json:"tier"`
	Status      SupplierStatus `json:"status"`
	UserID      *uuid.UUID     `json:"user_id,omitempty"`
	Version     int            `json:"version"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type SupplierOrder struct {
	ID                      uuid.UUID   `json:"id"`
	SupplierID              uuid.UUID   `json:"supplier_id"`
	OrderNumber             string      `json:"order_number"`
	OrderLines              []OrderLine `json:"order_lines"`
	Status                  OrderStatus `json:"status"`
	DeliveryDateConfirmed   *time.Time  `json:"delivery_date_confirmed,omitempty"`
	DeliveryDateConfirmedAt *time.Time  `json:"delivery_date_confirmed_at,omitempty"`
	ReceivedAt              *time.Time  `json:"received_at,omitempty"`
	Version                 int         `json:"version"`
	CreatedAt               time.Time   `json:"created_at"`
	UpdatedAt               time.Time   `json:"updated_at"`
	// Populated on reads
	SupplierName string            `json:"supplier_name,omitempty"`
	ASN          *SupplierASN      `json:"asn,omitempty"`
	QCResult     *SupplierQCResult `json:"qc_result,omitempty"`
}

type SupplierASN struct {
	ID              uuid.UUID  `json:"id"`
	OrderID         uuid.UUID  `json:"order_id"`
	TrackingInfo    string     `json:"tracking_info"`
	ShippedAt       time.Time  `json:"shipped_at"`
	ExpectedArrival *time.Time `json:"expected_arrival,omitempty"`
	SubmittedAt     time.Time  `json:"submitted_at"`
}

type SupplierQCResult struct {
	ID             uuid.UUID    `json:"id"`
	OrderID        uuid.UUID    `json:"order_id"`
	InspectedUnits int          `json:"inspected_units"`
	DefectiveUnits int          `json:"defective_units"`
	DefectRatePct  float64      `json:"defect_rate_pct"`
	Result         QCResultType `json:"result"`
	Notes          string       `json:"notes"`
	SubmittedAt    time.Time    `json:"submitted_at"`
	SubmittedBy    uuid.UUID    `json:"submitted_by"`
}

type SupplierKPI struct {
	ID                uuid.UUID    `json:"id"`
	SupplierID        uuid.UUID    `json:"supplier_id"`
	PeriodStart       time.Time    `json:"period_start"`
	PeriodEnd         time.Time    `json:"period_end"`
	OnTimeDeliveryPct float64      `json:"on_time_delivery_pct"`
	StockoutRatePct   float64      `json:"stockout_rate_pct"`
	ReturnRatePct     float64      `json:"return_rate_pct"`
	DefectRatePct     float64      `json:"defect_rate_pct"`
	TierAssigned      SupplierTier `json:"tier_assigned"`
	ComputedAt        time.Time    `json:"computed_at"`
}
