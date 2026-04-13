package supplierrepo

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/eduexchange/eduexchange/internal/model"
	"github.com/eduexchange/eduexchange/internal/sanitize"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresRepo struct {
	pool *pgxpool.Pool
}

// New creates a new PostgreSQL-backed SupplierRepository.
func New(pool *pgxpool.Pool) SupplierRepository {
	return &postgresRepo{pool: pool}
}

// ── Supplier CRUD ─────────────────────────────────────────────────────────────

func (r *postgresRepo) CreateSupplier(ctx context.Context, s *model.Supplier) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO suppliers (id, name, contact_json, contact_mask, tier, status, user_id, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		s.ID, s.Name, s.ContactJSON, s.ContactMask,
		s.Tier.String(), s.Status.String(),
		s.UserID, s.Version, s.CreatedAt, s.UpdatedAt,
	)
	return err
}

func (r *postgresRepo) GetSupplier(ctx context.Context, id uuid.UUID) (*model.Supplier, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, name, contact_json, contact_mask, tier, status, user_id, version, created_at, updated_at
		FROM suppliers WHERE id = $1`, id)
	return scanSupplier(row)
}

func (r *postgresRepo) UpdateSupplier(ctx context.Context, s *model.Supplier) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE suppliers SET name=$1, contact_json=$2, contact_mask=$3, tier=$4, status=$5,
		user_id=$6, version=version+1, updated_at=NOW()
		WHERE id=$7 AND version=$8`,
		s.Name, s.ContactJSON, s.ContactMask,
		s.Tier.String(), s.Status.String(),
		s.UserID, s.ID, s.Version,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return model.ErrStaleVersion
	}
	return nil
}

func (r *postgresRepo) UpdateSupplierTier(ctx context.Context, supplierID uuid.UUID, tier model.SupplierTier) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE suppliers SET tier=$1, version=version+1, updated_at=NOW() WHERE id=$2`,
		tier.String(), supplierID,
	)
	return err
}

func (r *postgresRepo) ListSuppliers(ctx context.Context) ([]model.Supplier, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, contact_json, contact_mask, tier, status, user_id, version, created_at, updated_at
		FROM suppliers ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var suppliers []model.Supplier
	for rows.Next() {
		s, err := scanSupplier(rows)
		if err != nil {
			return nil, err
		}
		suppliers = append(suppliers, *s)
	}
	return suppliers, rows.Err()
}

func (r *postgresRepo) GetSupplierByUserID(ctx context.Context, userID uuid.UUID) (*model.Supplier, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, name, contact_json, contact_mask, tier, status, user_id, version, created_at, updated_at
		FROM suppliers WHERE user_id = $1`, userID)
	return scanSupplier(row)
}

func (r *postgresRepo) GetAllActiveSuppliers(ctx context.Context) ([]model.Supplier, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, contact_json, contact_mask, tier, status, user_id, version, created_at, updated_at
		FROM suppliers WHERE status = 'ACTIVE' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var suppliers []model.Supplier
	for rows.Next() {
		s, err := scanSupplier(rows)
		if err != nil {
			return nil, err
		}
		suppliers = append(suppliers, *s)
	}
	return suppliers, rows.Err()
}

// ── Orders ────────────────────────────────────────────────────────────────────

func (r *postgresRepo) CreateOrder(ctx context.Context, o *model.SupplierOrder) error {
	linesJSON, err := sanitize.JSON(o.OrderLines)
	if err != nil {
		return fmt.Errorf("marshal order lines: %w", err)
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO supplier_orders
		(id, supplier_id, order_number, order_lines, status, delivery_date_confirmed, delivery_date_confirmed_at, received_at, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		o.ID, o.SupplierID, o.OrderNumber, string(linesJSON),
		o.Status.String(), o.DeliveryDateConfirmed, o.DeliveryDateConfirmedAt,
		o.ReceivedAt, o.Version, o.CreatedAt, o.UpdatedAt,
	)
	return err
}

func (r *postgresRepo) GetOrder(ctx context.Context, id uuid.UUID) (*model.SupplierOrder, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT o.id, o.supplier_id, o.order_number, o.order_lines, o.status,
		       o.delivery_date_confirmed, o.delivery_date_confirmed_at, o.received_at,
		       o.version, o.created_at, o.updated_at, s.name AS supplier_name,
		       a.id, a.tracking_info, a.shipped_at, a.expected_arrival, a.submitted_at,
		       q.id, q.inspected_units, q.defective_units, q.defect_rate_pct, q.result, q.notes, q.submitted_at, q.submitted_by
		FROM supplier_orders o
		JOIN suppliers s ON s.id = o.supplier_id
		LEFT JOIN supplier_asns a ON a.order_id = o.id
		LEFT JOIN supplier_qc_results q ON q.order_id = o.id
		WHERE o.id = $1`, id)
	return scanOrderFull(row)
}

func (r *postgresRepo) UpdateOrder(ctx context.Context, o *model.SupplierOrder) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE supplier_orders SET
		status=$1, delivery_date_confirmed=$2, delivery_date_confirmed_at=$3,
		received_at=$4, version=version+1, updated_at=NOW()
		WHERE id=$5 AND version=$6`,
		o.Status.String(), o.DeliveryDateConfirmed, o.DeliveryDateConfirmedAt,
		o.ReceivedAt, o.ID, o.Version,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return model.ErrStaleVersion
	}
	return nil
}

func (r *postgresRepo) ListOrders(ctx context.Context, supplierID *uuid.UUID, status string, page, pageSize int) ([]model.SupplierOrder, int, error) {
	args := []interface{}{}
	where := "WHERE 1=1"
	argIdx := 1

	if supplierID != nil {
		where += fmt.Sprintf(" AND o.supplier_id = $%d", argIdx)
		args = append(args, *supplierID)
		argIdx++
	}
	if status != "" {
		where += fmt.Sprintf(" AND o.status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}

	// Count
	var total int
	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	err := r.pool.QueryRow(ctx,
		fmt.Sprintf("SELECT COUNT(*) FROM supplier_orders o %s", where),
		countArgs...,
	).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Fetch
	offset := (page - 1) * pageSize
	queryArgs := make([]interface{}, len(args))
	copy(queryArgs, args)
	queryArgs = append(queryArgs, pageSize, offset)

	q := fmt.Sprintf(`
		SELECT o.id, o.supplier_id, o.order_number, o.order_lines, o.status,
		       o.delivery_date_confirmed, o.delivery_date_confirmed_at, o.received_at,
		       o.version, o.created_at, o.updated_at, s.name AS supplier_name
		FROM supplier_orders o
		JOIN suppliers s ON s.id = o.supplier_id
		%s
		ORDER BY o.created_at DESC
		LIMIT $%d OFFSET $%d`, where, argIdx, argIdx+1)

	rows, err := r.pool.Query(ctx, q, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var orders []model.SupplierOrder
	for rows.Next() {
		o, err := scanOrderSimple(rows)
		if err != nil {
			return nil, 0, err
		}
		orders = append(orders, *o)
	}
	return orders, total, rows.Err()
}

func (r *postgresRepo) GetOrdersInPeriod(ctx context.Context, supplierID uuid.UUID, start, end time.Time) ([]model.SupplierOrder, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT o.id, o.supplier_id, o.order_number, o.order_lines, o.status,
		       o.delivery_date_confirmed, o.delivery_date_confirmed_at, o.received_at,
		       o.version, o.created_at, o.updated_at, s.name AS supplier_name,
		       a.id, a.tracking_info, a.shipped_at, a.expected_arrival, a.submitted_at,
		       q.id, q.inspected_units, q.defective_units, q.defect_rate_pct, q.result, q.notes, q.submitted_at, q.submitted_by
		FROM supplier_orders o
		JOIN suppliers s ON s.id = o.supplier_id
		LEFT JOIN supplier_asns a ON a.order_id = o.id
		LEFT JOIN supplier_qc_results q ON q.order_id = o.id
		WHERE o.supplier_id = $1 AND o.created_at >= $2 AND o.created_at <= $3
		ORDER BY o.created_at`,
		supplierID, start, end,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []model.SupplierOrder
	for rows.Next() {
		o, err := scanOrderFull(rows)
		if err != nil {
			return nil, err
		}
		orders = append(orders, *o)
	}
	return orders, rows.Err()
}

func (r *postgresRepo) GetOrdersAwaitingConfirmation(ctx context.Context, deadline time.Time) ([]model.SupplierOrder, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT o.id, o.supplier_id, o.order_number, o.order_lines, o.status,
		       o.delivery_date_confirmed, o.delivery_date_confirmed_at, o.received_at,
		       o.version, o.created_at, o.updated_at, s.name AS supplier_name
		FROM supplier_orders o
		JOIN suppliers s ON s.id = o.supplier_id
		WHERE o.status = 'CREATED' AND o.created_at < $1
		ORDER BY o.created_at`,
		deadline,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []model.SupplierOrder
	for rows.Next() {
		o, err := scanOrderSimple(rows)
		if err != nil {
			return nil, err
		}
		orders = append(orders, *o)
	}
	return orders, rows.Err()
}

func (r *postgresRepo) GetOrdersAwaitingQC(ctx context.Context, deadline time.Time) ([]model.SupplierOrder, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT o.id, o.supplier_id, o.order_number, o.order_lines, o.status,
		       o.delivery_date_confirmed, o.delivery_date_confirmed_at, o.received_at,
		       o.version, o.created_at, o.updated_at, s.name AS supplier_name
		FROM supplier_orders o
		JOIN suppliers s ON s.id = o.supplier_id
		WHERE o.status = 'RECEIVED' AND o.received_at < $1
		ORDER BY o.received_at`,
		deadline,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []model.SupplierOrder
	for rows.Next() {
		o, err := scanOrderSimple(rows)
		if err != nil {
			return nil, err
		}
		orders = append(orders, *o)
	}
	return orders, rows.Err()
}

// ── ASN ───────────────────────────────────────────────────────────────────────

func (r *postgresRepo) CreateASN(ctx context.Context, asn *model.SupplierASN) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO supplier_asns (id, order_id, tracking_info, shipped_at, expected_arrival, submitted_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		asn.ID, asn.OrderID, asn.TrackingInfo, asn.ShippedAt, asn.ExpectedArrival, asn.SubmittedAt,
	)
	return err
}

func (r *postgresRepo) GetASNByOrderID(ctx context.Context, orderID uuid.UUID) (*model.SupplierASN, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, order_id, tracking_info, shipped_at, expected_arrival, submitted_at
		FROM supplier_asns WHERE order_id = $1`, orderID)

	var asn model.SupplierASN
	err := row.Scan(&asn.ID, &asn.OrderID, &asn.TrackingInfo, &asn.ShippedAt, &asn.ExpectedArrival, &asn.SubmittedAt)
	if err != nil {
		return nil, model.ErrNotFound
	}
	return &asn, nil
}

// ── QC Results ────────────────────────────────────────────────────────────────

func (r *postgresRepo) CreateQCResult(ctx context.Context, qc *model.SupplierQCResult) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO supplier_qc_results
		(id, order_id, inspected_units, defective_units, defect_rate_pct, result, notes, submitted_at, submitted_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		qc.ID, qc.OrderID, qc.InspectedUnits, qc.DefectiveUnits, qc.DefectRatePct,
		qc.Result.String(), qc.Notes, qc.SubmittedAt, qc.SubmittedBy,
	)
	return err
}

func (r *postgresRepo) GetQCResultByOrderID(ctx context.Context, orderID uuid.UUID) (*model.SupplierQCResult, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, order_id, inspected_units, defective_units, defect_rate_pct, result, notes, submitted_at, submitted_by
		FROM supplier_qc_results WHERE order_id = $1`, orderID)

	var qc model.SupplierQCResult
	var resultStr string
	err := row.Scan(&qc.ID, &qc.OrderID, &qc.InspectedUnits, &qc.DefectiveUnits,
		&qc.DefectRatePct, &resultStr, &qc.Notes, &qc.SubmittedAt, &qc.SubmittedBy)
	if err != nil {
		return nil, model.ErrNotFound
	}
	qc.Result, err = model.ParseQCResultType(resultStr)
	if err != nil {
		return nil, err
	}
	return &qc, nil
}

// ── KPIs ──────────────────────────────────────────────────────────────────────

func (r *postgresRepo) SaveKPI(ctx context.Context, kpi *model.SupplierKPI) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO supplier_kpis
		(id, supplier_id, period_start, period_end, on_time_delivery_pct, stockout_rate_pct, return_rate_pct, defect_rate_pct, tier_assigned, computed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		kpi.ID, kpi.SupplierID, kpi.PeriodStart, kpi.PeriodEnd,
		kpi.OnTimeDeliveryPct, kpi.StockoutRatePct, kpi.ReturnRatePct, kpi.DefectRatePct,
		kpi.TierAssigned.String(), kpi.ComputedAt,
	)
	return err
}

func (r *postgresRepo) GetLatestKPI(ctx context.Context, supplierID uuid.UUID) (*model.SupplierKPI, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, supplier_id, period_start, period_end, on_time_delivery_pct,
		       stockout_rate_pct, return_rate_pct, defect_rate_pct, tier_assigned, computed_at
		FROM supplier_kpis WHERE supplier_id = $1
		ORDER BY computed_at DESC LIMIT 1`, supplierID)
	return scanKPI(row)
}

func (r *postgresRepo) ListKPIs(ctx context.Context, supplierID uuid.UUID, limit int) ([]model.SupplierKPI, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, supplier_id, period_start, period_end, on_time_delivery_pct,
		       stockout_rate_pct, return_rate_pct, defect_rate_pct, tier_assigned, computed_at
		FROM supplier_kpis WHERE supplier_id = $1
		ORDER BY computed_at DESC LIMIT $2`, supplierID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var kpis []model.SupplierKPI
	for rows.Next() {
		k, err := scanKPI(rows)
		if err != nil {
			return nil, err
		}
		kpis = append(kpis, *k)
	}
	return kpis, rows.Err()
}

// ── Scan helpers ──────────────────────────────────────────────────────────────

type scanner interface {
	Scan(dest ...interface{}) error
}

func scanSupplier(row scanner) (*model.Supplier, error) {
	var s model.Supplier
	var tierStr, statusStr string
	err := row.Scan(
		&s.ID, &s.Name, &s.ContactJSON, &s.ContactMask,
		&tierStr, &statusStr, &s.UserID, &s.Version, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, model.ErrNotFound
	}
	s.Tier, err = model.ParseSupplierTier(tierStr)
	if err != nil {
		return nil, err
	}
	s.Status, err = model.ParseSupplierStatus(statusStr)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func scanOrderSimple(row scanner) (*model.SupplierOrder, error) {
	var o model.SupplierOrder
	var statusStr string
	var linesJSON string

	err := row.Scan(
		&o.ID, &o.SupplierID, &o.OrderNumber, &linesJSON, &statusStr,
		&o.DeliveryDateConfirmed, &o.DeliveryDateConfirmedAt, &o.ReceivedAt,
		&o.Version, &o.CreatedAt, &o.UpdatedAt, &o.SupplierName,
	)
	if err != nil {
		return nil, model.ErrNotFound
	}

	o.Status, err = model.ParseOrderStatus(statusStr)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(linesJSON), &o.OrderLines); err != nil {
		o.OrderLines = []model.OrderLine{}
	}
	return &o, nil
}

func scanOrderFull(row scanner) (*model.SupplierOrder, error) {
	var o model.SupplierOrder
	var statusStr string
	var linesJSON string

	// ASN nullable fields
	var asnID *uuid.UUID
	var asnTrackingInfo *string
	var asnShippedAt *time.Time
	var asnExpectedArrival *time.Time
	var asnSubmittedAt *time.Time

	// QC nullable fields
	var qcID *uuid.UUID
	var qcInspected *int
	var qcDefective *int
	var qcDefectRate *float64
	var qcResult *string
	var qcNotes *string
	var qcSubmittedAt *time.Time
	var qcSubmittedBy *uuid.UUID

	err := row.Scan(
		&o.ID, &o.SupplierID, &o.OrderNumber, &linesJSON, &statusStr,
		&o.DeliveryDateConfirmed, &o.DeliveryDateConfirmedAt, &o.ReceivedAt,
		&o.Version, &o.CreatedAt, &o.UpdatedAt, &o.SupplierName,
		&asnID, &asnTrackingInfo, &asnShippedAt, &asnExpectedArrival, &asnSubmittedAt,
		&qcID, &qcInspected, &qcDefective, &qcDefectRate, &qcResult, &qcNotes, &qcSubmittedAt, &qcSubmittedBy,
	)
	if err != nil {
		return nil, model.ErrNotFound
	}

	o.Status, err = model.ParseOrderStatus(statusStr)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(linesJSON), &o.OrderLines); err != nil {
		o.OrderLines = []model.OrderLine{}
	}

	if asnID != nil {
		o.ASN = &model.SupplierASN{
			ID:              *asnID,
			OrderID:         o.ID,
			TrackingInfo:    safeStr(asnTrackingInfo),
			ShippedAt:       safeTime(asnShippedAt),
			ExpectedArrival: asnExpectedArrival,
			SubmittedAt:     safeTime(asnSubmittedAt),
		}
	}

	if qcID != nil && qcResult != nil {
		result, _ := model.ParseQCResultType(*qcResult)
		o.QCResult = &model.SupplierQCResult{
			ID:             *qcID,
			OrderID:        o.ID,
			InspectedUnits: safeInt(qcInspected),
			DefectiveUnits: safeInt(qcDefective),
			DefectRatePct:  safeFloat64(qcDefectRate),
			Result:         result,
			Notes:          safeStr(qcNotes),
			SubmittedAt:    safeTime(qcSubmittedAt),
			SubmittedBy:    safeUUID(qcSubmittedBy),
		}
	}

	return &o, nil
}

func scanKPI(row scanner) (*model.SupplierKPI, error) {
	var k model.SupplierKPI
	var tierStr string
	err := row.Scan(
		&k.ID, &k.SupplierID, &k.PeriodStart, &k.PeriodEnd,
		&k.OnTimeDeliveryPct, &k.StockoutRatePct, &k.ReturnRatePct, &k.DefectRatePct,
		&tierStr, &k.ComputedAt,
	)
	if err != nil {
		return nil, model.ErrNotFound
	}
	k.TierAssigned, err = model.ParseSupplierTier(tierStr)
	if err != nil {
		return nil, err
	}
	return &k, nil
}

func safeStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func safeTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

func safeInt(i *int) int {
	if i == nil {
		return 0
	}
	return *i
}

func safeFloat64(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}

func safeUUID(u *uuid.UUID) uuid.UUID {
	if u == nil {
		return uuid.UUID{}
	}
	return *u
}
