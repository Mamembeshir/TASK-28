package analyticsrepo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/eduexchange/eduexchange/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresRepo struct {
	pool *pgxpool.Pool
}

// New returns a new AnalyticsRepository backed by PostgreSQL.
func New(pool *pgxpool.Pool) AnalyticsRepository {
	return &postgresRepo{pool: pool}
}

// ─── AnalyticsSummary ────────────────────────────────────────────────────────

func (r *postgresRepo) UpsertAnalyticsSummary(ctx context.Context, summary *model.AnalyticsSummary) error {
	if summary.ID == uuid.Nil {
		summary.ID = uuid.New()
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO analytics_summary
		   (id, metric_type, metric_key, metric_value, metric_label, computed_at, period_start, period_end)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		 ON CONFLICT (metric_type, metric_key)
		 DO UPDATE SET
		   metric_value = EXCLUDED.metric_value,
		   metric_label = EXCLUDED.metric_label,
		   computed_at  = EXCLUDED.computed_at,
		   period_start = EXCLUDED.period_start,
		   period_end   = EXCLUDED.period_end`,
		summary.ID, summary.MetricType, summary.MetricKey,
		summary.MetricValue, summary.MetricLabel, summary.ComputedAt,
		summary.PeriodStart, summary.PeriodEnd,
	)
	return err
}

func (r *postgresRepo) ListAnalyticsSummaries(ctx context.Context, metricType string) ([]model.AnalyticsSummary, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, metric_type, metric_key, metric_value, metric_label, computed_at, period_start, period_end
		 FROM analytics_summary
		 WHERE metric_type = $1
		 ORDER BY metric_key`,
		metricType,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []model.AnalyticsSummary
	for rows.Next() {
		var s model.AnalyticsSummary
		if err := rows.Scan(&s.ID, &s.MetricType, &s.MetricKey, &s.MetricValue,
			&s.MetricLabel, &s.ComputedAt, &s.PeriodStart, &s.PeriodEnd); err != nil {
			return nil, err
		}
		summaries = append(summaries, s)
	}
	return summaries, rows.Err()
}

func (r *postgresRepo) DeleteOldSummaries(ctx context.Context, before time.Time) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM analytics_summary WHERE computed_at < $1`, before)
	return err
}

// ─── Computed analytics queries ───────────────────────────────────────────────

func (r *postgresRepo) GetUtilizationPeaks(ctx context.Context) ([]model.AnalyticsSummary, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT EXTRACT(HOUR FROM created_at)::int AS hour, COUNT(*) AS cnt
		 FROM votes
		 GROUP BY hour
		 ORDER BY hour`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	now := time.Now()
	var summaries []model.AnalyticsSummary
	for rows.Next() {
		var hour int
		var cnt float64
		if err := rows.Scan(&hour, &cnt); err != nil {
			return nil, err
		}
		summaries = append(summaries, model.AnalyticsSummary{
			MetricType:  "UTILIZATION_PEAKS",
			MetricKey:   fmt.Sprintf("%02d", hour),
			MetricValue: cnt,
			MetricLabel: fmt.Sprintf("Hour %02d:00", hour),
			ComputedAt:  now,
			PeriodStart: now.Add(-24 * time.Hour),
			PeriodEnd:   now,
		})
	}
	return summaries, rows.Err()
}

func (r *postgresRepo) GetDemandHotspots(ctx context.Context) ([]model.AnalyticsSummary, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT c.name, COUNT(r.id) AS cnt
		 FROM resources r
		 JOIN categories c ON c.id = r.category_id
		 GROUP BY c.id, c.name
		 ORDER BY COUNT(r.id) DESC
		 LIMIT 10`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	now := time.Now()
	var summaries []model.AnalyticsSummary
	for rows.Next() {
		var name string
		var cnt float64
		if err := rows.Scan(&name, &cnt); err != nil {
			return nil, err
		}
		summaries = append(summaries, model.AnalyticsSummary{
			MetricType:  "DEMAND_HOTSPOTS",
			MetricKey:   name,
			MetricValue: cnt,
			MetricLabel: name,
			ComputedAt:  now,
			PeriodStart: now.Add(-30 * 24 * time.Hour),
			PeriodEnd:   now,
		})
	}
	return summaries, rows.Err()
}

func (r *postgresRepo) GetApprovalCycleTime(ctx context.Context) (float64, error) {
	var avg *float64
	err := r.pool.QueryRow(ctx,
		`SELECT AVG(EXTRACT(EPOCH FROM (ra.created_at - r.created_at))/3600)
		 FROM resource_reviews ra
		 JOIN resources r ON r.id = ra.resource_id
		 WHERE ra.decision = 'APPROVED'`,
	).Scan(&avg)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	if avg == nil {
		return 0, nil
	}
	return *avg, nil
}

func (r *postgresRepo) GetViolationRate(ctx context.Context) (float64, error) {
	var rate float64
	err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(
		   100.0 * SUM(CASE WHEN status='TAKEN_DOWN' THEN 1 ELSE 0 END)
		         / NULLIF(SUM(CASE WHEN status='PUBLISHED' OR status='TAKEN_DOWN' THEN 1 ELSE 0 END), 0),
		 0)
		 FROM resources`,
	).Scan(&rate)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	return rate, nil
}

func (r *postgresRepo) GetCancellationRate(ctx context.Context) (float64, error) {
	var rate float64
	err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(
		   100.0 * SUM(CASE WHEN status='CANCELLED' THEN 1 ELSE 0 END)
		         / NULLIF(COUNT(*), 0),
		 0)
		 FROM supplier_orders`,
	).Scan(&rate)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	return rate, nil
}

func (r *postgresRepo) GetTotalCounts(ctx context.Context) (resources, users, orders int, err error) {
	err = r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM resources`).Scan(&resources)
	if err != nil {
		return
	}
	err = r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&users)
	if err != nil {
		return
	}
	err = r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM supplier_orders`).Scan(&orders)
	return
}

// ─── ScheduledReport ──────────────────────────────────────────────────────────

func (r *postgresRepo) CreateScheduledReport(ctx context.Context, report *model.ScheduledReport) error {
	if report.ID == uuid.Nil {
		report.ID = uuid.New()
	}
	paramsJSON, err := json.Marshal(report.Parameters)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO scheduled_reports
		   (id, report_type, parameters, file_path, status, generated_at, requested_by, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,NOW(),NOW())`,
		report.ID, report.ReportType, paramsJSON, report.FilePath,
		report.Status.String(), report.GeneratedAt, report.RequestedBy,
	)
	return err
}

func (r *postgresRepo) GetScheduledReport(ctx context.Context, id uuid.UUID) (*model.ScheduledReport, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, report_type, parameters, file_path, status, generated_at, requested_by, created_at, updated_at
		 FROM scheduled_reports WHERE id = $1`, id)

	report := &model.ScheduledReport{}
	var paramsJSON []byte
	var statusStr string
	err := row.Scan(
		&report.ID, &report.ReportType, &paramsJSON, &report.FilePath,
		&statusStr, &report.GeneratedAt, &report.RequestedBy,
		&report.CreatedAt, &report.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	if paramsJSON != nil {
		if err := json.Unmarshal(paramsJSON, &report.Parameters); err != nil {
			return nil, err
		}
	}
	report.Status, err = model.ParseScheduledReportStatus(statusStr)
	if err != nil {
		return nil, err
	}
	return report, nil
}

func (r *postgresRepo) ListScheduledReports(ctx context.Context, page, pageSize int) ([]model.ScheduledReport, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM scheduled_reports`).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.pool.Query(ctx,
		`SELECT id, report_type, parameters, file_path, status, generated_at, requested_by, created_at, updated_at
		 FROM scheduled_reports
		 ORDER BY created_at DESC
		 LIMIT $1 OFFSET $2`,
		pageSize, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var reports []model.ScheduledReport
	for rows.Next() {
		var rpt model.ScheduledReport
		var paramsJSON []byte
		var statusStr string
		if err := rows.Scan(
			&rpt.ID, &rpt.ReportType, &paramsJSON, &rpt.FilePath,
			&statusStr, &rpt.GeneratedAt, &rpt.RequestedBy,
			&rpt.CreatedAt, &rpt.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		if paramsJSON != nil {
			if err := json.Unmarshal(paramsJSON, &rpt.Parameters); err != nil {
				return nil, 0, err
			}
		}
		rpt.Status, err = model.ParseScheduledReportStatus(statusStr)
		if err != nil {
			return nil, 0, err
		}
		reports = append(reports, rpt)
	}
	return reports, total, rows.Err()
}

func (r *postgresRepo) UpdateScheduledReport(ctx context.Context, report *model.ScheduledReport) error {
	paramsJSON, err := json.Marshal(report.Parameters)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx,
		`UPDATE scheduled_reports
		 SET report_type=$2, parameters=$3, file_path=$4, status=$5,
		     generated_at=$6, requested_by=$7, updated_at=NOW()
		 WHERE id=$1`,
		report.ID, report.ReportType, paramsJSON, report.FilePath,
		report.Status.String(), report.GeneratedAt, report.RequestedBy,
	)
	return err
}

// ─── Audit log queries ────────────────────────────────────────────────────────

func (r *postgresRepo) ListAuditLogs(ctx context.Context, filter AnalyticsFilter) ([]model.AuditLogEntry, int, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 50
	}

	args := []interface{}{}
	argIdx := 1

	baseWhere := " WHERE 1=1"
	conditions := ""
	if filter.ActorID != nil {
		conditions += fmt.Sprintf(" AND al.actor_id=$%d", argIdx)
		args = append(args, *filter.ActorID)
		argIdx++
	}
	if filter.EntityType != "" {
		conditions += fmt.Sprintf(" AND al.entity_type=$%d", argIdx)
		args = append(args, filter.EntityType)
		argIdx++
	}
	if filter.Action != "" {
		conditions += fmt.Sprintf(" AND al.action=$%d", argIdx)
		args = append(args, filter.Action)
		argIdx++
	}
	if filter.From != nil {
		conditions += fmt.Sprintf(" AND al.timestamp>=$%d", argIdx)
		args = append(args, *filter.From)
		argIdx++
	}
	if filter.To != nil {
		conditions += fmt.Sprintf(" AND al.timestamp<=$%d", argIdx)
		args = append(args, *filter.To)
		argIdx++
	}

	countQuery := `SELECT COUNT(*) FROM audit_logs al` + baseWhere + conditions
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (filter.Page - 1) * filter.PageSize
	dataArgs := append(args, filter.PageSize, offset)
	dataQuery := `SELECT al.id, al.actor_id, COALESCE(u.username,'system'),
	               al.action, al.entity_type, al.entity_id,
	               al.before_data, al.after_data, al.ip_address, al.source, al.reason, al.timestamp
	              FROM audit_logs al
	              LEFT JOIN users u ON u.id = al.actor_id` +
		baseWhere + conditions +
		fmt.Sprintf(` ORDER BY al.timestamp DESC LIMIT $%d OFFSET $%d`, argIdx, argIdx+1)

	rows, err := r.pool.Query(ctx, dataQuery, dataArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var entries []model.AuditLogEntry
	for rows.Next() {
		e, err := scanAuditLogEntry(rows)
		if err != nil {
			return nil, 0, err
		}
		entries = append(entries, e)
	}
	return entries, total, rows.Err()
}

func (r *postgresRepo) ExportAuditLogs(ctx context.Context, filter AnalyticsFilter) ([]model.AuditLogEntry, error) {
	args := []interface{}{}
	argIdx := 1

	baseWhere := " WHERE 1=1"
	conditions := ""
	if filter.ActorID != nil {
		conditions += fmt.Sprintf(" AND al.actor_id=$%d", argIdx)
		args = append(args, *filter.ActorID)
		argIdx++
	}
	if filter.EntityType != "" {
		conditions += fmt.Sprintf(" AND al.entity_type=$%d", argIdx)
		args = append(args, filter.EntityType)
		argIdx++
	}
	if filter.Action != "" {
		conditions += fmt.Sprintf(" AND al.action=$%d", argIdx)
		args = append(args, filter.Action)
		argIdx++
	}
	if filter.From != nil {
		conditions += fmt.Sprintf(" AND al.timestamp>=$%d", argIdx)
		args = append(args, *filter.From)
		argIdx++
	}
	if filter.To != nil {
		conditions += fmt.Sprintf(" AND al.timestamp<=$%d", argIdx)
		args = append(args, *filter.To)
		argIdx++
	}
	_ = argIdx

	query := `SELECT al.id, al.actor_id, COALESCE(u.username,'system'),
	               al.action, al.entity_type, al.entity_id,
	               al.before_data, al.after_data, al.ip_address, al.source, al.reason, al.timestamp
	              FROM audit_logs al
	              LEFT JOIN users u ON u.id = al.actor_id` +
		baseWhere + conditions +
		` ORDER BY al.timestamp DESC`

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []model.AuditLogEntry
	for rows.Next() {
		e, err := scanAuditLogEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// scanAuditLogEntry scans a row into an AuditLogEntry, handling nullable JSONB fields.
func scanAuditLogEntry(rows pgx.Rows) (model.AuditLogEntry, error) {
	var e model.AuditLogEntry
	var beforeRaw, afterRaw []byte
	err := rows.Scan(
		&e.ID, &e.ActorID, &e.ActorUsername,
		&e.Action, &e.EntityType, &e.EntityID,
		&beforeRaw, &afterRaw,
		&e.IPAddress, &e.Source, &e.Reason, &e.Timestamp,
	)
	if err != nil {
		return e, err
	}
	if beforeRaw != nil {
		_ = json.Unmarshal(beforeRaw, &e.BeforeData)
	}
	if afterRaw != nil {
		_ = json.Unmarshal(afterRaw, &e.AfterData)
	}
	return e, nil
}
