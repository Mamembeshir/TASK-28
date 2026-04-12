package engagementrepo

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/eduexchange/eduexchange/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresRepo struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) EngagementRepository {
	return &postgresRepo{pool: pool}
}

// ── Votes ─────────────────────────────────────────────────────────────────────

func (r *postgresRepo) UpsertVote(ctx context.Context, v *model.Vote) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO votes (id, user_id, resource_id, vote_type, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		ON CONFLICT (user_id, resource_id) DO UPDATE SET
			vote_type = EXCLUDED.vote_type,
			updated_at = NOW()`,
		v.ID, v.UserID, v.ResourceID, v.VoteType)
	return err
}

func (r *postgresRepo) DeleteVote(ctx context.Context, userID, resourceID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM votes WHERE user_id=$1 AND resource_id=$2`,
		userID, resourceID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return model.ErrNotFound
	}
	return nil
}

func (r *postgresRepo) GetVote(ctx context.Context, userID, resourceID uuid.UUID) (*model.Vote, error) {
	v := &model.Vote{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, user_id, resource_id, vote_type, created_at, updated_at
		FROM votes WHERE user_id=$1 AND resource_id=$2`,
		userID, resourceID).
		Scan(&v.ID, &v.UserID, &v.ResourceID, &v.VoteType, &v.CreatedAt, &v.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	return v, err
}

func (r *postgresRepo) GetVoteCounts(ctx context.Context, resourceID uuid.UUID) (*model.VoteCounts, error) {
	vc := &model.VoteCounts{ResourceID: resourceID}
	err := r.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE vote_type='UP')   AS upvotes,
			COUNT(*) FILTER (WHERE vote_type='DOWN') AS downvotes
		FROM votes WHERE resource_id=$1`,
		resourceID).
		Scan(&vc.Upvotes, &vc.Downvotes)
	if err != nil {
		return nil, err
	}
	vc.Score = vc.Upvotes - vc.Downvotes
	return vc, nil
}

func (r *postgresRepo) GetMutualVoteCount(ctx context.Context, userA, userB uuid.UUID, withinHours int) (int, error) {
	// Count votes where userA voted on userB's resources AND userB voted on userA's resources
	// in the given time window.
	cutoff := time.Now().Add(-time.Duration(withinHours) * time.Hour)
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM (
			SELECT v.id FROM votes v
			JOIN resources r ON r.id = v.resource_id
			WHERE v.user_id = $1 AND r.author_id = $2 AND v.updated_at >= $3
			UNION ALL
			SELECT v.id FROM votes v
			JOIN resources r ON r.id = v.resource_id
			WHERE v.user_id = $2 AND r.author_id = $1 AND v.updated_at >= $3
		) mutual`,
		userA, userB, cutoff).Scan(&count)
	return count, err
}

// ── Favorites ─────────────────────────────────────────────────────────────────

func (r *postgresRepo) UpsertFavorite(ctx context.Context, f *model.Favorite) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO favorites (id, user_id, resource_id, created_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (user_id, resource_id) DO NOTHING`,
		f.ID, f.UserID, f.ResourceID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (r *postgresRepo) DeleteFavorite(ctx context.Context, userID, resourceID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM favorites WHERE user_id=$1 AND resource_id=$2`,
		userID, resourceID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return model.ErrNotFound
	}
	return nil
}

func (r *postgresRepo) GetFavorite(ctx context.Context, userID, resourceID uuid.UUID) (*model.Favorite, error) {
	f := &model.Favorite{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, user_id, resource_id, created_at
		FROM favorites WHERE user_id=$1 AND resource_id=$2`,
		userID, resourceID).
		Scan(&f.ID, &f.UserID, &f.ResourceID, &f.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	return f, err
}

func (r *postgresRepo) ListFavorites(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]model.Resource, int, error) {
	offset := (page - 1) * pageSize
	rows, err := r.pool.Query(ctx, `
		SELECT
			res.id, res.title, res.description, res.content_body,
			res.author_id, res.category_id, res.status,
			res.current_version_number, res.version, res.created_at, res.updated_at,
			COALESCE(u.username, '') AS author_name,
			COALESCE(c.name, '') AS category_name
		FROM favorites f
		JOIN resources res ON res.id = f.resource_id
		LEFT JOIN users u ON u.id = res.author_id
		LEFT JOIN categories c ON c.id = res.category_id
		WHERE f.user_id = $1
		ORDER BY f.created_at DESC
		LIMIT $2 OFFSET $3`,
		userID, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var resources []model.Resource
	for rows.Next() {
		var res model.Resource
		var statusStr string
		if err := rows.Scan(
			&res.ID, &res.Title, &res.Description, &res.ContentBody,
			&res.AuthorID, &res.CategoryID, &statusStr,
			&res.CurrentVersionNumber, &res.Version, &res.CreatedAt, &res.UpdatedAt,
			&res.AuthorName, &res.CategoryName,
		); err != nil {
			return nil, 0, err
		}
		res.Status, _ = model.ParseResourceStatus(statusStr)
		resources = append(resources, res)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	var total int
	_ = r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM favorites WHERE user_id=$1`, userID).Scan(&total)
	return resources, total, nil
}

func (r *postgresRepo) CountFavoritesReceived(ctx context.Context, authorID uuid.UUID) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM favorites f
		JOIN resources res ON res.id = f.resource_id
		WHERE res.author_id = $1`, authorID).Scan(&count)
	return count, err
}

// ── Follows ───────────────────────────────────────────────────────────────────

func (r *postgresRepo) UpsertFollow(ctx context.Context, f *model.Follow) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO follows (id, follower_id, target_type, target_id, created_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (follower_id, target_type, target_id) DO NOTHING`,
		f.ID, f.FollowerID, f.TargetType, f.TargetID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (r *postgresRepo) DeleteFollow(ctx context.Context, followerID uuid.UUID, targetType model.FollowTargetType, targetID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM follows WHERE follower_id=$1 AND target_type=$2 AND target_id=$3`,
		followerID, targetType, targetID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return model.ErrNotFound
	}
	return nil
}

func (r *postgresRepo) GetFollow(ctx context.Context, followerID uuid.UUID, targetType model.FollowTargetType, targetID uuid.UUID) (*model.Follow, error) {
	f := &model.Follow{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, follower_id, target_type, target_id, created_at
		FROM follows WHERE follower_id=$1 AND target_type=$2 AND target_id=$3`,
		followerID, targetType, targetID).
		Scan(&f.ID, &f.FollowerID, &f.TargetType, &f.TargetID, &f.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	return f, err
}

func (r *postgresRepo) ListFollowedAuthorIDs(ctx context.Context, followerID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT target_id FROM follows
		WHERE follower_id=$1 AND target_type='AUTHOR'`,
		followerID)
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

func (r *postgresRepo) ListFollowerIDs(ctx context.Context, authorID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT follower_id FROM follows
		WHERE target_type='AUTHOR' AND target_id=$1`,
		authorID)
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

// ── Anomaly Flags ─────────────────────────────────────────────────────────────

func (r *postgresRepo) CreateAnomalyFlag(ctx context.Context, flag *model.AnomalyFlag) error {
	evidenceJSON, err := json.Marshal(flag.EvidenceJSON)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO anomaly_flags (id, flag_type, user_ids, evidence_json, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())`,
		flag.ID, flag.FlagType, flag.UserIDs, evidenceJSON, flag.Status)
	return err
}

func (r *postgresRepo) ListAnomalyFlags(ctx context.Context, status string) ([]model.AnomalyFlag, error) {
	query := `SELECT id, flag_type, user_ids, evidence_json, status, created_at, updated_at
	          FROM anomaly_flags`
	args := []interface{}{}
	if status != "" {
		query += ` WHERE status=$1`
		args = append(args, status)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var flags []model.AnomalyFlag
	for rows.Next() {
		var f model.AnomalyFlag
		var evidenceJSON []byte
		if err := rows.Scan(
			&f.ID, &f.FlagType, &f.UserIDs, &evidenceJSON, &f.Status, &f.CreatedAt, &f.UpdatedAt,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(evidenceJSON, &f.EvidenceJSON)
		flags = append(flags, f)
	}
	return flags, rows.Err()
}

func (r *postgresRepo) UpdateAnomalyFlag(ctx context.Context, flagID uuid.UUID, status string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE anomaly_flags SET status=$2, updated_at=NOW()
		WHERE id=$1`, flagID, status)
	return err
}
