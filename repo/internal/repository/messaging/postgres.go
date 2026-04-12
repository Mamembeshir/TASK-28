package messagingrepo

import (
	"context"
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

// New returns a MessagingRepository backed by PostgreSQL.
func New(pool *pgxpool.Pool) MessagingRepository {
	return &postgresRepo{pool: pool}
}

// ── Notifications ─────────────────────────────────────────────────────────────

func (r *postgresRepo) CreateNotification(ctx context.Context, n *model.Notification) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO notifications (id, user_id, event_type, title, body, resource_id, is_read, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, false, NOW())`,
		n.ID, n.UserID, string(n.EventType), n.Title, n.Body, n.ResourceID)
	return err
}

func (r *postgresRepo) GetNotification(ctx context.Context, id uuid.UUID) (*model.Notification, error) {
	n := &model.Notification{}
	var eventType string
	err := r.pool.QueryRow(ctx, `
		SELECT id, user_id, event_type, title, body, resource_id, is_read, created_at, read_at
		FROM notifications WHERE id = $1`, id).
		Scan(&n.ID, &n.UserID, &eventType, &n.Title, &n.Body, &n.ResourceID,
			&n.IsRead, &n.CreatedAt, &n.ReadAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	n.EventType = model.EventType(eventType)
	return n, nil
}

func (r *postgresRepo) ListNotifications(ctx context.Context, userID uuid.UUID, filter NotificationFilter) ([]model.Notification, int, error) {
	// Build WHERE clause dynamically
	args := []interface{}{userID}
	where := "WHERE user_id = $1"
	idx := 2

	if filter.EventType != nil {
		where += fmt.Sprintf(" AND event_type = $%d", idx)
		args = append(args, string(*filter.EventType))
		idx++
	}
	if filter.IsRead != nil {
		where += fmt.Sprintf(" AND is_read = $%d", idx)
		args = append(args, *filter.IsRead)
		idx++
	}

	// Count query
	var total int
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM notifications %s", where)
	err := r.pool.QueryRow(ctx, countSQL, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Pagination
	pageSize := filter.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	page := filter.Page
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * pageSize

	listSQL := fmt.Sprintf(`
		SELECT id, user_id, event_type, title, body, resource_id, is_read, created_at, read_at
		FROM notifications %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, where, idx, idx+1)
	args = append(args, pageSize, offset)

	rows, err := r.pool.Query(ctx, listSQL, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var notifications []model.Notification
	for rows.Next() {
		var n model.Notification
		var eventType string
		if err := rows.Scan(&n.ID, &n.UserID, &eventType, &n.Title, &n.Body,
			&n.ResourceID, &n.IsRead, &n.CreatedAt, &n.ReadAt); err != nil {
			return nil, 0, err
		}
		n.EventType = model.EventType(eventType)
		notifications = append(notifications, n)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return notifications, total, nil
}

func (r *postgresRepo) GetUnreadCount(ctx context.Context, userID uuid.UUID) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND is_read = false`,
		userID).Scan(&count)
	return count, err
}

func (r *postgresRepo) MarkRead(ctx context.Context, notificationID, userID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE notifications SET is_read = true, read_at = NOW()
		 WHERE id = $1 AND user_id = $2`,
		notificationID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return model.ErrNotFound
	}
	return nil
}

func (r *postgresRepo) BulkMarkRead(ctx context.Context, userID uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE notifications SET is_read = true, read_at = NOW()
		 WHERE user_id = $1 AND is_read = false`,
		userID)
	return err
}

// ── Subscriptions ─────────────────────────────────────────────────────────────

func (r *postgresRepo) GetSubscription(ctx context.Context, userID uuid.UUID, eventType model.EventType) (*model.NotificationSubscription, error) {
	sub := &model.NotificationSubscription{}
	var et string
	err := r.pool.QueryRow(ctx,
		`SELECT user_id, event_type, enabled, updated_at
		 FROM notification_subscriptions WHERE user_id = $1 AND event_type = $2`,
		userID, string(eventType)).
		Scan(&sub.UserID, &et, &sub.Enabled, &sub.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	sub.EventType = model.EventType(et)
	return sub, nil
}

func (r *postgresRepo) ListSubscriptions(ctx context.Context, userID uuid.UUID) ([]model.NotificationSubscription, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT user_id, event_type, enabled, updated_at
		 FROM notification_subscriptions WHERE user_id = $1`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []model.NotificationSubscription
	for rows.Next() {
		var sub model.NotificationSubscription
		var et string
		if err := rows.Scan(&sub.UserID, &et, &sub.Enabled, &sub.UpdatedAt); err != nil {
			return nil, err
		}
		sub.EventType = model.EventType(et)
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

func (r *postgresRepo) UpsertSubscription(ctx context.Context, sub *model.NotificationSubscription) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO notification_subscriptions (user_id, event_type, enabled, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (user_id, event_type) DO UPDATE SET
			enabled = EXCLUDED.enabled,
			updated_at = NOW()`,
		sub.UserID, string(sub.EventType), sub.Enabled)
	return err
}

// ── Retry Queue ───────────────────────────────────────────────────────────────

func (r *postgresRepo) CreateRetryQueueItem(ctx context.Context, item *model.NotificationRetryQueue) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO notification_retry_queue
			(id, user_id, event_type, title, body, resource_id, attempts, next_retry_at, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())`,
		item.ID, item.UserID, string(item.EventType), item.Title, item.Body,
		item.ResourceID, item.Attempts, item.NextRetryAt, item.Status.String())
	return err
}

func (r *postgresRepo) GetPendingRetries(ctx context.Context, now time.Time) ([]model.NotificationRetryQueue, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, event_type, title, body, resource_id, attempts, next_retry_at, status, created_at, updated_at
		FROM notification_retry_queue
		WHERE status = 'PENDING' AND next_retry_at <= $1
		ORDER BY next_retry_at ASC`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []model.NotificationRetryQueue
	for rows.Next() {
		var item model.NotificationRetryQueue
		var eventType, statusStr string
		if err := rows.Scan(&item.ID, &item.UserID, &eventType, &item.Title, &item.Body,
			&item.ResourceID, &item.Attempts, &item.NextRetryAt, &statusStr,
			&item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.EventType = model.EventType(eventType)
		if s, err := model.ParseNotificationRetryStatus(statusStr); err == nil {
			item.Status = s
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *postgresRepo) UpdateRetryItem(ctx context.Context, item *model.NotificationRetryQueue) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE notification_retry_queue
		SET status = $1, attempts = $2, next_retry_at = $3, updated_at = NOW()
		WHERE id = $4`,
		item.Status.String(), item.Attempts, item.NextRetryAt, item.ID)
	return err
}

func (r *postgresRepo) DeleteRetryItem(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM notification_retry_queue WHERE id = $1`, id)
	return err
}
