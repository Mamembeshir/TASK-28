package gamificationrepo

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

func New(pool *pgxpool.Pool) GamificationRepository {
	return &postgresRepo{pool: pool}
}

// ── User Points ───────────────────────────────────────────────────────────────

func (r *postgresRepo) GetUserPoints(ctx context.Context, userID uuid.UUID) (*model.UserPoints, error) {
	up := &model.UserPoints{}
	err := r.pool.QueryRow(ctx, `
		SELECT user_id, total_points, level, updated_at
		FROM user_points WHERE user_id=$1`, userID).
		Scan(&up.UserID, &up.TotalPoints, &up.Level, &up.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		// Return zero state if no row yet
		return &model.UserPoints{UserID: userID, TotalPoints: 0, Level: 0, UpdatedAt: time.Now()}, nil
	}
	return up, err
}

func (r *postgresRepo) UpsertUserPoints(ctx context.Context, up *model.UserPoints) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO user_points (user_id, total_points, level, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (user_id) DO UPDATE SET
			total_points = EXCLUDED.total_points,
			level = EXCLUDED.level,
			updated_at = NOW()`,
		up.UserID, up.TotalPoints, up.Level)
	return err
}

// ── Point Transactions ────────────────────────────────────────────────────────

func (r *postgresRepo) CreatePointTransaction(ctx context.Context, tx *model.PointTransaction) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO point_transactions (id, user_id, points, reason, source_type, source_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())`,
		tx.ID, tx.UserID, tx.Points, tx.Reason, tx.SourceType, tx.SourceID)
	return err
}

func (r *postgresRepo) ListPointTransactions(ctx context.Context, userID uuid.UUID, limit int) ([]model.PointTransaction, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, points, reason, source_type, source_id, created_at
		FROM point_transactions WHERE user_id=$1
		ORDER BY created_at DESC LIMIT $2`,
		userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var txs []model.PointTransaction
	for rows.Next() {
		var tx model.PointTransaction
		if err := rows.Scan(&tx.ID, &tx.UserID, &tx.Points, &tx.Reason, &tx.SourceType, &tx.SourceID, &tx.CreatedAt); err != nil {
			return nil, err
		}
		txs = append(txs, tx)
	}
	return txs, rows.Err()
}

// ── Point Rules ───────────────────────────────────────────────────────────────

func (r *postgresRepo) ListPointRules(ctx context.Context) ([]model.PointRule, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, event_type, points, description, is_active, created_at, updated_at
		FROM point_rules ORDER BY event_type`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []model.PointRule
	for rows.Next() {
		var rule model.PointRule
		if err := rows.Scan(&rule.ID, &rule.EventType, &rule.Points, &rule.Description,
			&rule.IsActive, &rule.CreatedAt, &rule.UpdatedAt); err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func (r *postgresRepo) GetPointRuleByEvent(ctx context.Context, eventType string) (*model.PointRule, error) {
	rule := &model.PointRule{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, event_type, points, description, is_active, created_at, updated_at
		FROM point_rules WHERE event_type=$1 AND is_active=TRUE`, eventType).
		Scan(&rule.ID, &rule.EventType, &rule.Points, &rule.Description,
			&rule.IsActive, &rule.CreatedAt, &rule.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	return rule, err
}

func (r *postgresRepo) UpdatePointRuleByID(ctx context.Context, rule *model.PointRule) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE point_rules
		SET points=$2, description=$3, is_active=$4, updated_at=NOW()
		WHERE id=$1`,
		rule.ID, rule.Points, rule.Description, rule.IsActive)
	return err
}

func (r *postgresRepo) UpsertPointRule(ctx context.Context, rule *model.PointRule) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO point_rules (id, event_type, points, description, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		ON CONFLICT (event_type) DO UPDATE SET
			points = EXCLUDED.points,
			description = EXCLUDED.description,
			is_active = EXCLUDED.is_active,
			updated_at = NOW()`,
		rule.ID, rule.EventType, rule.Points, rule.Description, rule.IsActive)
	return err
}

// ── Badges ────────────────────────────────────────────────────────────────────

func (r *postgresRepo) ListBadges(ctx context.Context) ([]model.Badge, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, code, name, description, threshold_type, threshold_value, created_at
		FROM badges ORDER BY threshold_value`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var badges []model.Badge
	for rows.Next() {
		var b model.Badge
		if err := rows.Scan(&b.ID, &b.Code, &b.Name, &b.Description,
			&b.ThresholdType, &b.ThresholdValue, &b.CreatedAt); err != nil {
			return nil, err
		}
		badges = append(badges, b)
	}
	return badges, rows.Err()
}

func (r *postgresRepo) GetBadgeByCode(ctx context.Context, code string) (*model.Badge, error) {
	b := &model.Badge{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, code, name, description, threshold_type, threshold_value, created_at
		FROM badges WHERE code=$1`, code).
		Scan(&b.ID, &b.Code, &b.Name, &b.Description,
			&b.ThresholdType, &b.ThresholdValue, &b.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	return b, err
}

func (r *postgresRepo) GetUserBadge(ctx context.Context, userID, badgeID uuid.UUID) (*model.UserBadge, error) {
	ub := &model.UserBadge{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, user_id, badge_id, awarded_at
		FROM user_badges WHERE user_id=$1 AND badge_id=$2`,
		userID, badgeID).Scan(&ub.ID, &ub.UserID, &ub.BadgeID, &ub.AwardedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	return ub, err
}

func (r *postgresRepo) CreateUserBadge(ctx context.Context, ub *model.UserBadge) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO user_badges (id, user_id, badge_id, awarded_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (user_id, badge_id) DO NOTHING`,
		ub.ID, ub.UserID, ub.BadgeID)
	return err
}

func (r *postgresRepo) ListUserBadges(ctx context.Context, userID uuid.UUID) ([]model.UserBadge, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT ub.id, ub.user_id, ub.badge_id, ub.awarded_at,
		       b.id, b.code, b.name, b.description, b.threshold_type, b.threshold_value, b.created_at
		FROM user_badges ub
		JOIN badges b ON b.id = ub.badge_id
		WHERE ub.user_id=$1
		ORDER BY ub.awarded_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ubs []model.UserBadge
	for rows.Next() {
		var ub model.UserBadge
		ub.Badge = &model.Badge{}
		if err := rows.Scan(
			&ub.ID, &ub.UserID, &ub.BadgeID, &ub.AwardedAt,
			&ub.Badge.ID, &ub.Badge.Code, &ub.Badge.Name, &ub.Badge.Description,
			&ub.Badge.ThresholdType, &ub.Badge.ThresholdValue, &ub.Badge.CreatedAt,
		); err != nil {
			return nil, err
		}
		ubs = append(ubs, ub)
	}
	return ubs, rows.Err()
}

// ── Rankings ──────────────────────────────────────────────────────────────────

func (r *postgresRepo) GetBestsellers(ctx context.Context, limit int) ([]model.RankingEntry, error) {
	// ISO week boundary for the current week (Monday 00:00)
	rows, err := r.pool.Query(ctx, `
		SELECT
			row_number() OVER (ORDER BY COUNT(*) FILTER (WHERE v.vote_type='UP') DESC) AS rank,
			res.id, res.title, COALESCE(u.username, '') AS author_name,
			COUNT(*) FILTER (WHERE v.vote_type='UP') AS score
		FROM resources res
		LEFT JOIN votes v ON v.resource_id = res.id
		LEFT JOIN users u ON u.id = res.author_id
		WHERE res.status = 'PUBLISHED'
		  AND (v.updated_at IS NULL OR v.updated_at >= date_trunc('week', NOW()))
		GROUP BY res.id, res.title, u.username
		ORDER BY score DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []model.RankingEntry
	for rows.Next() {
		var e model.RankingEntry
		if err := rows.Scan(&e.Rank, &e.ResourceID, &e.Title, &e.AuthorName, &e.Score); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (r *postgresRepo) GetNewReleases(ctx context.Context, limit int) ([]model.RankingEntry, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			row_number() OVER (ORDER BY res.created_at DESC) AS rank,
			res.id, res.title, COALESCE(u.username, '') AS author_name,
			0 AS score
		FROM resources res
		LEFT JOIN users u ON u.id = res.author_id
		WHERE res.status = 'PUBLISHED'
		  AND res.created_at >= date_trunc('week', NOW())
		ORDER BY res.created_at DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []model.RankingEntry
	for rows.Next() {
		var e model.RankingEntry
		if err := rows.Scan(&e.Rank, &e.ResourceID, &e.Title, &e.AuthorName, &e.Score); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (r *postgresRepo) CreateRankingArchive(ctx context.Context, archive *model.RankingArchive) error {
	entriesJSON, err := sanitize.JSON(archive.Entries)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO ranking_archives (id, week_number, year, ranking_type, entries_json, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (week_number, year, ranking_type) DO UPDATE SET
			entries_json = EXCLUDED.entries_json`,
		archive.ID, archive.WeekNumber, archive.Year, archive.RankingType, entriesJSON)
	return err
}

func (r *postgresRepo) ListRankingArchives(ctx context.Context, rankingType string, limit int) ([]model.RankingArchive, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, week_number, year, ranking_type, entries_json, created_at
		FROM ranking_archives
		WHERE ranking_type=$1
		ORDER BY year DESC, week_number DESC
		LIMIT $2`, rankingType, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var archives []model.RankingArchive
	for rows.Next() {
		var a model.RankingArchive
		var entriesJSON []byte
		if err := rows.Scan(&a.ID, &a.WeekNumber, &a.Year, &a.RankingType, &entriesJSON, &a.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(entriesJSON, &a.Entries)
		archives = append(archives, a)
	}
	return archives, rows.Err()
}

// ── Recommendation Strategy Config ────────────────────────────────────────────

func (r *postgresRepo) ListStrategyConfigs(ctx context.Context) ([]model.RecommendationStrategyConfig, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, strategy_key, label, sort_order, is_active, created_at, updated_at
		FROM recommendation_strategy_config
		ORDER BY sort_order ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cfgs []model.RecommendationStrategyConfig
	for rows.Next() {
		var c model.RecommendationStrategyConfig
		if err := rows.Scan(&c.ID, &c.StrategyKey, &c.Label, &c.SortOrder,
			&c.IsActive, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		cfgs = append(cfgs, c)
	}
	return cfgs, rows.Err()
}

func (r *postgresRepo) GetStrategyConfig(ctx context.Context, key string) (*model.RecommendationStrategyConfig, error) {
	c := &model.RecommendationStrategyConfig{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, strategy_key, label, sort_order, is_active, created_at, updated_at
		FROM recommendation_strategy_config WHERE strategy_key=$1`, key).
		Scan(&c.ID, &c.StrategyKey, &c.Label, &c.SortOrder, &c.IsActive, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	return c, err
}

func (r *postgresRepo) UpdateStrategyConfig(ctx context.Context, cfg *model.RecommendationStrategyConfig) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE recommendation_strategy_config
		SET label=$1, sort_order=$2, is_active=$3, updated_at=NOW()
		WHERE id=$4`,
		cfg.Label, cfg.SortOrder, cfg.IsActive, cfg.ID)
	return err
}

// ── Leaderboard ───────────────────────────────────────────────────────────────

func (r *postgresRepo) GetLeaderboard(ctx context.Context, limit int) ([]model.LeaderboardEntry, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			row_number() OVER (ORDER BY up.total_points DESC) AS rank,
			up.user_id, u.username, up.total_points, up.level
		FROM user_points up
		JOIN users u ON u.id = up.user_id
		ORDER BY up.total_points DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []model.LeaderboardEntry
	for rows.Next() {
		var e model.LeaderboardEntry
		if err := rows.Scan(&e.Rank, &e.UserID, &e.Username, &e.TotalPoints, &e.Level); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// ── Engagement Counts ─────────────────────────────────────────────────────────

func (r *postgresRepo) GetTopEngagedCategoryIDs(ctx context.Context, userID uuid.UUID, limit int) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT res.category_id
		FROM (
			SELECT resource_id FROM votes WHERE user_id=$1
			UNION ALL
			SELECT resource_id FROM favorites WHERE user_id=$1
		) eng
		JOIN resources res ON res.id = eng.resource_id
		WHERE res.category_id IS NOT NULL
		GROUP BY res.category_id
		ORDER BY COUNT(*) DESC
		LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *postgresRepo) GetFavoritedTagNames(ctx context.Context, userID uuid.UUID, limit int) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT t.name
		FROM favorites f
		JOIN resource_tags rt ON rt.resource_id = f.resource_id
		JOIN tags t ON t.id = rt.tag_id
		WHERE f.user_id = $1
		LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

func (r *postgresRepo) CountUpvotesReceived(ctx context.Context, authorID uuid.UUID) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM votes v
		JOIN resources res ON res.id = v.resource_id
		WHERE res.author_id=$1 AND v.vote_type='UP'`, authorID).Scan(&count)
	return count, err
}

func (r *postgresRepo) CountEntriesApproved(ctx context.Context, authorID uuid.UUID) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM resources
		WHERE author_id=$1 AND status IN ('APPROVED','PUBLISHED')`, authorID).Scan(&count)
	return count, err
}
