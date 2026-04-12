package recommendations

import (
	"context"
	"fmt"
	"strings"

	"github.com/eduexchange/eduexchange/internal/model"
	gamificationrepo "github.com/eduexchange/eduexchange/internal/repository/gamification"
	engagementrepo "github.com/eduexchange/eduexchange/internal/repository/engagement"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── MostEngagedCategories ──────────────────────────────────────────────────────

// MostEngagedCategories recommends PUBLISHED resources in the categories
// the user has engaged with (voted/favorited) most.
type MostEngagedCategories struct {
	gamRepo gamificationrepo.GamificationRepository
	pool    *pgxpool.Pool
}

func NewMostEngagedCategories(gamRepo gamificationrepo.GamificationRepository, pool *pgxpool.Pool) *MostEngagedCategories {
	return &MostEngagedCategories{gamRepo: gamRepo, pool: pool}
}

func (s *MostEngagedCategories) Key() string { return "MostEngagedCategories" }

func (s *MostEngagedCategories) Recommend(ctx context.Context, userID uuid.UUID, limit int) ([]model.Resource, error) {
	catIDs, err := s.gamRepo.GetTopEngagedCategoryIDs(ctx, userID, 5)
	if err != nil || len(catIDs) == 0 {
		return nil, err
	}

	// Build placeholders for IN clause
	placeholders := make([]string, len(catIDs))
	args := make([]interface{}, len(catIDs)+1)
	args[0] = limit
	for i, id := range catIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args[i+1] = id
	}

	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT
			res.id, res.title, res.description, res.content_body,
			res.author_id, res.category_id, res.status,
			res.current_version_number, res.version, res.created_at, res.updated_at,
			COALESCE(u.username,'') AS author_name,
			COALESCE(c.name,'') AS category_name
		FROM resources res
		LEFT JOIN users u ON u.id = res.author_id
		LEFT JOIN categories c ON c.id = res.category_id
		WHERE res.status = 'PUBLISHED'
		  AND res.category_id IN (%s)
		ORDER BY res.updated_at DESC
		LIMIT $1`,
		strings.Join(placeholders, ",")),
		args[:len(catIDs)+1]...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanResources(rows)
}

// ── FollowedAuthorNewContent ──────────────────────────────────────────────────

// FollowedAuthorNewContent recommends recent PUBLISHED resources from authors the user follows.
type FollowedAuthorNewContent struct {
	engRepo engagementrepo.EngagementRepository
	pool    *pgxpool.Pool
}

func NewFollowedAuthorNewContent(engRepo engagementrepo.EngagementRepository, pool *pgxpool.Pool) *FollowedAuthorNewContent {
	return &FollowedAuthorNewContent{engRepo: engRepo, pool: pool}
}

func (s *FollowedAuthorNewContent) Key() string { return "FollowedAuthorNewContent" }

func (s *FollowedAuthorNewContent) Recommend(ctx context.Context, userID uuid.UUID, limit int) ([]model.Resource, error) {
	authorIDs, err := s.engRepo.ListFollowedAuthorIDs(ctx, userID)
	if err != nil || len(authorIDs) == 0 {
		return nil, err
	}

	placeholders := make([]string, len(authorIDs))
	args := make([]interface{}, len(authorIDs)+1)
	args[0] = limit
	for i, id := range authorIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args[i+1] = id
	}

	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT
			res.id, res.title, res.description, res.content_body,
			res.author_id, res.category_id, res.status,
			res.current_version_number, res.version, res.created_at, res.updated_at,
			COALESCE(u.username,'') AS author_name,
			COALESCE(c.name,'') AS category_name
		FROM resources res
		LEFT JOIN users u ON u.id = res.author_id
		LEFT JOIN categories c ON c.id = res.category_id
		WHERE res.status = 'PUBLISHED'
		  AND res.author_id IN (%s)
		ORDER BY res.created_at DESC
		LIMIT $1`,
		strings.Join(placeholders, ",")),
		args[:len(authorIDs)+1]...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanResources(rows)
}

// ── SimilarTagAffinity ────────────────────────────────────────────────────────

// SimilarTagAffinity recommends resources sharing tags with the user's favorites.
type SimilarTagAffinity struct {
	gamRepo gamificationrepo.GamificationRepository
	pool    *pgxpool.Pool
}

func NewSimilarTagAffinity(gamRepo gamificationrepo.GamificationRepository, pool *pgxpool.Pool) *SimilarTagAffinity {
	return &SimilarTagAffinity{gamRepo: gamRepo, pool: pool}
}

func (s *SimilarTagAffinity) Key() string { return "SimilarTagAffinity" }

func (s *SimilarTagAffinity) Recommend(ctx context.Context, userID uuid.UUID, limit int) ([]model.Resource, error) {
	tagNames, err := s.gamRepo.GetFavoritedTagNames(ctx, userID, 10)
	if err != nil || len(tagNames) == 0 {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT ON (res.id)
			res.id, res.title, res.description, res.content_body,
			res.author_id, res.category_id, res.status,
			res.current_version_number, res.version, res.created_at, res.updated_at,
			COALESCE(u.username,'') AS author_name,
			COALESCE(c.name,'') AS category_name
		FROM resources res
		JOIN resource_tags rt ON rt.resource_id = res.id
		JOIN tags t ON t.id = rt.tag_id
		LEFT JOIN users u ON u.id = res.author_id
		LEFT JOIN categories c ON c.id = res.category_id
		WHERE res.status = 'PUBLISHED'
		  AND t.name = ANY($1)
		ORDER BY res.id, res.created_at DESC
		LIMIT $2`,
		tagNames, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanResources(rows)
}

// ── helper ────────────────────────────────────────────────────────────────────

func scanResources(rows interface{ Next() bool; Scan(...interface{}) error; Err() error }) ([]model.Resource, error) {
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
			return nil, err
		}
		res.Status, _ = model.ParseResourceStatus(statusStr)
		resources = append(resources, res)
	}
	return resources, rows.Err()
}
