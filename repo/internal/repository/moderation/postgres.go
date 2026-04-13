package moderationrepo

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/eduexchange/eduexchange/internal/model"
	"github.com/eduexchange/eduexchange/internal/sanitize"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresRepo struct {
	pool *pgxpool.Pool
}

// New creates a new ModerationRepository backed by PostgreSQL.
func New(pool *pgxpool.Pool) ModerationRepository {
	return &postgresRepo{pool: pool}
}

// ── Reports ───────────────────────────────────────────────────────────────────

func (r *postgresRepo) CreateReport(ctx context.Context, rep *model.Report) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO reports (id, reporter_id, resource_id, reason_type, description, status, reviewer_id, notes, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())`,
		rep.ID, rep.ReporterID, rep.ResourceID, rep.ReasonType, rep.Description,
		rep.Status, rep.ReviewerID, rep.Notes)
	return err
}

func (r *postgresRepo) GetReport(ctx context.Context, id uuid.UUID) (*model.Report, error) {
	rep := &model.Report{}
	var reviewerID *uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT r.id, r.reporter_id, r.resource_id, r.reason_type, r.description, r.status,
		       r.reviewer_id, r.notes, r.created_at, r.updated_at,
		       res.title AS resource_title,
		       COALESCE(u1.username,'') AS reporter_name,
		       COALESCE(u2.username,'') AS reviewer_name
		FROM reports r
		JOIN resources res ON res.id = r.resource_id
		JOIN users u1 ON u1.id = r.reporter_id
		LEFT JOIN users u2 ON u2.id = r.reviewer_id
		WHERE r.id = $1`, id).
		Scan(&rep.ID, &rep.ReporterID, &rep.ResourceID, &rep.ReasonType, &rep.Description,
			&rep.Status, &reviewerID, &rep.Notes, &rep.CreatedAt, &rep.UpdatedAt,
			&rep.ResourceTitle, &rep.ReporterName, &rep.ReviewerName)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rep.ReviewerID = reviewerID
	return rep, nil
}

func (r *postgresRepo) UpdateReport(ctx context.Context, rep *model.Report) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE reports SET
			status = $2,
			reviewer_id = $3,
			notes = $4,
			updated_at = NOW()
		WHERE id = $1`,
		rep.ID, rep.Status, rep.ReviewerID, rep.Notes)
	return err
}

func (r *postgresRepo) ListReports(ctx context.Context, status string, page, pageSize int) ([]model.Report, int, error) {
	offset := (page - 1) * pageSize

	var countRow int
	var rows pgx.Rows
	var err error

	if status == "" {
		err = r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM reports`).Scan(&countRow)
	} else {
		err = r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM reports WHERE status = $1`, status).Scan(&countRow)
	}
	if err != nil {
		return nil, 0, err
	}

	query := `
		SELECT r.id, r.reporter_id, r.resource_id, r.reason_type, r.description, r.status,
		       r.reviewer_id, r.notes, r.created_at, r.updated_at,
		       res.title AS resource_title,
		       COALESCE(u1.username,'') AS reporter_name,
		       COALESCE(u2.username,'') AS reviewer_name
		FROM reports r
		JOIN resources res ON res.id = r.resource_id
		JOIN users u1 ON u1.id = r.reporter_id
		LEFT JOIN users u2 ON u2.id = r.reviewer_id`

	if status == "" {
		rows, err = r.pool.Query(ctx, query+` ORDER BY r.created_at DESC LIMIT $1 OFFSET $2`, pageSize, offset)
	} else {
		rows, err = r.pool.Query(ctx, query+` WHERE r.status = $1 ORDER BY r.created_at DESC LIMIT $2 OFFSET $3`, status, pageSize, offset)
	}
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var reports []model.Report
	for rows.Next() {
		var rep model.Report
		var reviewerID *uuid.UUID
		if err := rows.Scan(&rep.ID, &rep.ReporterID, &rep.ResourceID, &rep.ReasonType,
			&rep.Description, &rep.Status, &reviewerID, &rep.Notes,
			&rep.CreatedAt, &rep.UpdatedAt,
			&rep.ResourceTitle, &rep.ReporterName, &rep.ReviewerName); err != nil {
			return nil, 0, err
		}
		rep.ReviewerID = reviewerID
		reports = append(reports, rep)
	}
	return reports, countRow, nil
}

// ── Moderation Actions ────────────────────────────────────────────────────────

func (r *postgresRepo) CreateModerationAction(ctx context.Context, a *model.ModerationAction) error {
	evidenceBytes, err := sanitize.JSON(a.EvidenceJSON)
	if err != nil {
		evidenceBytes = []byte("{}")
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO moderation_actions (id, moderator_id, action_type, target_type, target_id, report_id, notes, evidence_json, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())`,
		a.ID, a.ModeratorID, a.ActionType, a.TargetType, a.TargetID,
		a.ReportID, a.Notes, evidenceBytes)
	return err
}

func (r *postgresRepo) ListModerationActions(ctx context.Context, targetType string, targetID uuid.UUID) ([]model.ModerationAction, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT ma.id, ma.moderator_id, ma.action_type, ma.target_type, ma.target_id,
		       ma.report_id, ma.notes, ma.evidence_json, ma.created_at,
		       COALESCE(u.username,'') AS moderator_name
		FROM moderation_actions ma
		JOIN users u ON u.id = ma.moderator_id
		WHERE ma.target_type=$1 AND ma.target_id=$2
		ORDER BY ma.created_at ASC`,
		targetType, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var actions []model.ModerationAction
	for rows.Next() {
		var a model.ModerationAction
		var reportID *uuid.UUID
		var evidenceBytes []byte
		if err := rows.Scan(&a.ID, &a.ModeratorID, &a.ActionType, &a.TargetType, &a.TargetID,
			&reportID, &a.Notes, &evidenceBytes, &a.CreatedAt, &a.ModeratorName); err != nil {
			return nil, err
		}
		a.ReportID = reportID
		if len(evidenceBytes) > 0 {
			_ = json.Unmarshal(evidenceBytes, &a.EvidenceJSON)
		}
		if a.EvidenceJSON == nil {
			a.EvidenceJSON = map[string]interface{}{}
		}
		actions = append(actions, a)
	}
	return actions, nil
}

// ── User Bans ─────────────────────────────────────────────────────────────────

func (r *postgresRepo) CreateUserBan(ctx context.Context, b *model.UserBan) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO user_bans (id, user_id, ban_type, reason, banned_by, expires_at, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())`,
		b.ID, b.UserID, b.BanType, b.Reason, b.BannedBy, b.ExpiresAt, b.IsActive)
	return err
}

func (r *postgresRepo) GetActiveUserBan(ctx context.Context, userID uuid.UUID) (*model.UserBan, error) {
	b := &model.UserBan{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, user_id, ban_type, reason, banned_by, expires_at, is_active, created_at, updated_at
		FROM user_bans
		WHERE user_id=$1 AND is_active=TRUE AND (expires_at IS NULL OR expires_at > NOW())
		ORDER BY created_at DESC LIMIT 1`, userID).
		Scan(&b.ID, &b.UserID, &b.BanType, &b.Reason, &b.BannedBy, &b.ExpiresAt,
			&b.IsActive, &b.CreatedAt, &b.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	return b, err
}

func (r *postgresRepo) DeactivateBan(ctx context.Context, userID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE user_bans SET is_active=FALSE, updated_at=NOW()
		WHERE user_id=$1 AND is_active=TRUE`, userID)
	return err
}

// ── Anomaly Flags ─────────────────────────────────────────────────────────────

func (r *postgresRepo) UpdateAnomalyFlag(ctx context.Context, flagID uuid.UUID, status string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE anomaly_flags SET status=$2, updated_at=NOW()
		WHERE id=$1`, flagID, status)
	return err
}

// ── Vote Cleanup ──────────────────────────────────────────────────────────────

func (r *postgresRepo) DeleteVotesByUsers(ctx context.Context, userIDs []uuid.UUID) error {
	if len(userIDs) == 0 {
		return nil
	}
	// Convert []uuid.UUID to []string for ANY($1::uuid[])
	ids := make([]string, len(userIDs))
	for i, id := range userIDs {
		ids[i] = id.String()
	}
	_, err := r.pool.Exec(ctx, `DELETE FROM votes WHERE user_id = ANY($1::uuid[])`, ids)
	return err
}

// ── Rate Limit ────────────────────────────────────────────────────────────────

func (r *postgresRepo) IncrementRateLimit(ctx context.Context, userID uuid.UUID, actionType string, windowStart time.Time) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO rate_limit_counters (user_id, action_type, window_start, count)
		VALUES ($1, $2, $3, 1)
		ON CONFLICT (user_id, action_type, window_start) DO UPDATE
		SET count = rate_limit_counters.count + 1`,
		userID, actionType, windowStart)
	return err
}

func (r *postgresRepo) GetRateLimitCount(ctx context.Context, userID uuid.UUID, actionType string, windowStart time.Time) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT count FROM rate_limit_counters
		WHERE user_id=$1 AND action_type=$2 AND window_start=$3`,
		userID, actionType, windowStart).Scan(&count)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return count, err
}

// ── User Status ───────────────────────────────────────────────────────────────

func (r *postgresRepo) SetUserStatus(ctx context.Context, userID uuid.UUID, status string) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET status=$2, updated_at=NOW() WHERE id=$1`, userID, status)
	return err
}
