package gamificationrepo

import (
	"context"

	"github.com/eduexchange/eduexchange/internal/model"
	"github.com/google/uuid"
)

// GamificationRepository is the persistence interface for gamification.
type GamificationRepository interface {
	// ── User Points ─────────────────────────────────────────────────────────
	GetUserPoints(ctx context.Context, userID uuid.UUID) (*model.UserPoints, error)
	// UpsertUserPoints creates or updates the user_points row.
	UpsertUserPoints(ctx context.Context, up *model.UserPoints) error

	// ── Point Transactions ──────────────────────────────────────────────────
	CreatePointTransaction(ctx context.Context, tx *model.PointTransaction) error
	ListPointTransactions(ctx context.Context, userID uuid.UUID, limit int) ([]model.PointTransaction, error)

	// ── Point Rules ─────────────────────────────────────────────────────────
	ListPointRules(ctx context.Context) ([]model.PointRule, error)
	GetPointRuleByEvent(ctx context.Context, eventType string) (*model.PointRule, error)
	UpsertPointRule(ctx context.Context, rule *model.PointRule) error
	UpdatePointRuleByID(ctx context.Context, rule *model.PointRule) error

	// ── Badges ──────────────────────────────────────────────────────────────
	ListBadges(ctx context.Context) ([]model.Badge, error)
	GetBadgeByCode(ctx context.Context, code string) (*model.Badge, error)
	GetUserBadge(ctx context.Context, userID, badgeID uuid.UUID) (*model.UserBadge, error)
	CreateUserBadge(ctx context.Context, ub *model.UserBadge) error
	ListUserBadges(ctx context.Context, userID uuid.UUID) ([]model.UserBadge, error)

	// ── Rankings ────────────────────────────────────────────────────────────
	GetBestsellers(ctx context.Context, limit int) ([]model.RankingEntry, error)
	GetNewReleases(ctx context.Context, limit int) ([]model.RankingEntry, error)
	CreateRankingArchive(ctx context.Context, archive *model.RankingArchive) error
	ListRankingArchives(ctx context.Context, rankingType string, limit int) ([]model.RankingArchive, error)

	// ── Recommendation Strategy Config ──────────────────────────────────────
	ListStrategyConfigs(ctx context.Context) ([]model.RecommendationStrategyConfig, error)
	GetStrategyConfig(ctx context.Context, key string) (*model.RecommendationStrategyConfig, error)
	UpdateStrategyConfig(ctx context.Context, cfg *model.RecommendationStrategyConfig) error

	// ── Leaderboard ─────────────────────────────────────────────────────────
	GetLeaderboard(ctx context.Context, limit int) ([]model.LeaderboardEntry, error)

	// ── Engagement Counts (used by recommendation strategies) ───────────────
	// GetTopEngagedCategoryIDs returns category IDs the user has voted/favorited most.
	GetTopEngagedCategoryIDs(ctx context.Context, userID uuid.UUID, limit int) ([]uuid.UUID, error)
	// GetFavoritedTagNames returns tag names from the user's favorited resources.
	GetFavoritedTagNames(ctx context.Context, userID uuid.UUID, limit int) ([]string, error)
	// CountUpvotesReceived returns total upvotes received on all of an author's resources.
	CountUpvotesReceived(ctx context.Context, authorID uuid.UUID) (int, error)
	// CountEntriesApproved returns how many resources an author has had approved.
	CountEntriesApproved(ctx context.Context, authorID uuid.UUID) (int, error)
}
