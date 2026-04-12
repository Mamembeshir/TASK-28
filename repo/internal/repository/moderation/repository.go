package moderationrepo

import (
	"context"
	"time"

	"github.com/eduexchange/eduexchange/internal/model"
	"github.com/google/uuid"
)

// ModerationRepository is the persistence interface for the moderation domain.
type ModerationRepository interface {
	// ── Reports ─────────────────────────────────────────────────────────────
	CreateReport(ctx context.Context, r *model.Report) error
	GetReport(ctx context.Context, id uuid.UUID) (*model.Report, error)
	UpdateReport(ctx context.Context, r *model.Report) error
	ListReports(ctx context.Context, status string, page, pageSize int) ([]model.Report, int, error)

	// ── Moderation Actions ──────────────────────────────────────────────────
	CreateModerationAction(ctx context.Context, a *model.ModerationAction) error
	ListModerationActions(ctx context.Context, targetType string, targetID uuid.UUID) ([]model.ModerationAction, error)

	// ── User Bans ────────────────────────────────────────────────────────────
	CreateUserBan(ctx context.Context, b *model.UserBan) error
	GetActiveUserBan(ctx context.Context, userID uuid.UUID) (*model.UserBan, error)
	DeactivateBan(ctx context.Context, userID uuid.UUID) error

	// ── Anomaly Flags ────────────────────────────────────────────────────────
	UpdateAnomalyFlag(ctx context.Context, flagID uuid.UUID, status string) error

	// ── Vote Cleanup ─────────────────────────────────────────────────────────
	DeleteVotesByUsers(ctx context.Context, userIDs []uuid.UUID) error

	// ── Rate Limit ───────────────────────────────────────────────────────────
	IncrementRateLimit(ctx context.Context, userID uuid.UUID, actionType string, windowStart time.Time) error
	GetRateLimitCount(ctx context.Context, userID uuid.UUID, actionType string, windowStart time.Time) (int, error)

	// ── User Status ──────────────────────────────────────────────────────────
	SetUserStatus(ctx context.Context, userID uuid.UUID, status string) error
}
