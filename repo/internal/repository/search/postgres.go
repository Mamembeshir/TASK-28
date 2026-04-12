package searchrepo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/eduexchange/eduexchange/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresRepo struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) SearchRepository {
	return &postgresRepo{pool: pool}
}

// ── Full-text Search ──────────────────────────────────────────────────────────

func (r *postgresRepo) SearchResources(ctx context.Context, f model.SearchFilter, expandedTerms []string) ([]model.Resource, int, error) {
	if f.Page <= 0 {
		f.Page = 1
	}
	if f.PageSize <= 0 {
		f.PageSize = 20
	}
	offset := (f.Page - 1) * f.PageSize

	// Build tsquery from query + expanded terms.
	var tsQueryStr string
	allTerms := []string{f.Query}
	allTerms = append(allTerms, expandedTerms...)
	// Deduplicate and filter empty
	seen := map[string]bool{}
	var queryParts []string
	for _, t := range allTerms {
		t = strings.TrimSpace(t)
		if t != "" && !seen[t] {
			seen[t] = true
			queryParts = append(queryParts, t)
		}
	}

	args := []interface{}{}
	argN := 1

	var rankExpr, whereClause string
	if len(queryParts) > 0 {
		// Use plainto_tsquery for each term, OR them together for broad matching.
		tsQueryStr = fmt.Sprintf("plainto_tsquery('english', $%d)", argN)
		args = append(args, strings.Join(queryParts, " "))
		argN++

		rankExpr = fmt.Sprintf("ts_rank(si.tsvector_content, %s)", tsQueryStr)
		whereClause = fmt.Sprintf(`
			(si.tsvector_content @@ %s
			 OR si.pinyin_content ILIKE $%d
			 OR si.tag_content ILIKE $%d)`,
			tsQueryStr, argN, argN+1)
		likePattern := "%" + strings.Join(queryParts, "%") + "%"
		args = append(args, likePattern, likePattern)
		argN += 2
	} else {
		rankExpr = "res.created_at"
		whereClause = "TRUE"
	}

	// Status filter
	statusFilter := "res.status = 'PUBLISHED'"
	if f.Status != "" {
		statusFilter = fmt.Sprintf("res.status = $%d", argN)
		args = append(args, f.Status)
		argN++
	}

	// Optional filters
	var extraFilters []string
	if f.AuthorID != nil {
		extraFilters = append(extraFilters, fmt.Sprintf("res.author_id = $%d", argN))
		args = append(args, *f.AuthorID)
		argN++
	}
	if f.CategoryID != nil {
		extraFilters = append(extraFilters, fmt.Sprintf("res.category_id = $%d", argN))
		args = append(args, *f.CategoryID)
		argN++
	}
	if f.DateFrom != nil {
		extraFilters = append(extraFilters, fmt.Sprintf("res.created_at >= $%d", argN))
		args = append(args, *f.DateFrom)
		argN++
	}
	if f.DateTo != nil {
		extraFilters = append(extraFilters, fmt.Sprintf("res.created_at <= $%d", argN))
		args = append(args, *f.DateTo)
		argN++
	}

	extraWhere := ""
	if len(extraFilters) > 0 {
		extraWhere = " AND " + strings.Join(extraFilters, " AND ")
	}

	// Tag filter (multi-select): resource must have ALL specified tags
	tagJoin := ""
	if len(f.Tags) > 0 {
		tagJoin = `
		JOIN (
			SELECT rt.resource_id
			FROM resource_tags rt
			JOIN tags t ON t.id = rt.tag_id
			WHERE t.name = ANY($` + fmt.Sprintf("%d", argN) + `)
			GROUP BY rt.resource_id
			HAVING COUNT(DISTINCT t.name) = $` + fmt.Sprintf("%d", argN+1) + `
		) tag_filter ON tag_filter.resource_id = res.id`
		args = append(args, f.Tags, len(f.Tags))
		argN += 2
	}

	// Sort
	orderBy := fmt.Sprintf("%s DESC", rankExpr) // default: relevance
	switch f.Sort {
	case "newest":
		orderBy = "res.created_at DESC"
	case "most_upvoted":
		orderBy = `(SELECT COUNT(*) FROM votes v WHERE v.resource_id=res.id AND v.vote_type='UP') DESC`
	case "most_favorited":
		orderBy = `(SELECT COUNT(*) FROM favorites fav WHERE fav.resource_id=res.id) DESC`
	}

	query := fmt.Sprintf(`
		SELECT
			res.id, res.title, res.description, res.content_body,
			res.author_id, res.category_id, res.status,
			res.current_version_number, res.version, res.created_at, res.updated_at,
			COALESCE(u.username, '') AS author_name,
			COALESCE(c.name, '') AS category_name
		FROM resources res
		LEFT JOIN search_index si ON si.resource_id = res.id
		LEFT JOIN users u ON u.id = res.author_id
		LEFT JOIN categories c ON c.id = res.category_id
		%s
		WHERE %s AND %s %s
		ORDER BY %s
		LIMIT $%d OFFSET $%d`,
		tagJoin, whereClause, statusFilter, extraWhere,
		orderBy, argN, argN+1)

	args = append(args, f.PageSize, offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("search query: %w", err)
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

	// Count query (reuse same filters minus ORDER/LIMIT/OFFSET)
	countQuery := fmt.Sprintf(`
		SELECT COUNT(*) FROM resources res
		LEFT JOIN search_index si ON si.resource_id = res.id
		%s
		WHERE %s AND %s %s`,
		tagJoin, whereClause, statusFilter, extraWhere)

	// Count args = same but without pageSize/offset at end
	countArgs := args[:len(args)-2]
	var total int
	_ = r.pool.QueryRow(ctx, countQuery, countArgs...).Scan(&total)

	return resources, total, nil
}

func (r *postgresRepo) FindDidYouMean(ctx context.Context, query string) (string, error) {
	if len(query) < 2 {
		return "", nil
	}
	var suggestion string
	err := r.pool.QueryRow(ctx, `
		SELECT term FROM search_terms
		WHERE similarity(term, $1) > 0.3 AND term != $1
		ORDER BY similarity(term, $1) DESC, usage_count DESC
		LIMIT 1`, query).Scan(&suggestion)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return suggestion, err
}

// ── Search Index ──────────────────────────────────────────────────────────────

func (r *postgresRepo) UpsertSearchIndex(ctx context.Context, resourceID uuid.UUID, pinyinContent, tagContent string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE search_index
		SET pinyin_content=$2, tag_content=$3, updated_at=NOW()
		WHERE resource_id=$1`,
		resourceID, pinyinContent, tagContent)
	return err
}

// ── Pinyin Mapping ────────────────────────────────────────────────────────────

func (r *postgresRepo) GetPinyinMappings(ctx context.Context) (map[string]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT character, pinyin FROM pinyin_mapping`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	m := make(map[string]string)
	for rows.Next() {
		var char, pinyin string
		if err := rows.Scan(&char, &pinyin); err != nil {
			return nil, err
		}
		m[char] = pinyin
	}
	return m, rows.Err()
}

// ── Synonyms ──────────────────────────────────────────────────────────────────

func (r *postgresRepo) ListSynonymGroups(ctx context.Context) ([]model.SynonymGroup, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, canonical_term, synonyms, created_at, updated_at
		FROM synonym_groups ORDER BY canonical_term`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []model.SynonymGroup
	for rows.Next() {
		var sg model.SynonymGroup
		if err := rows.Scan(&sg.ID, &sg.CanonicalTerm, &sg.Synonyms, &sg.CreatedAt, &sg.UpdatedAt); err != nil {
			return nil, err
		}
		groups = append(groups, sg)
	}
	return groups, rows.Err()
}

func (r *postgresRepo) CreateSynonymGroup(ctx context.Context, sg *model.SynonymGroup) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO synonym_groups (id, canonical_term, synonyms, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())`,
		sg.ID, sg.CanonicalTerm, sg.Synonyms)
	return err
}

func (r *postgresRepo) UpdateSynonymGroup(ctx context.Context, sg *model.SynonymGroup) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE synonym_groups SET canonical_term=$2, synonyms=$3, updated_at=NOW()
		WHERE id=$1`, sg.ID, sg.CanonicalTerm, sg.Synonyms)
	return err
}

func (r *postgresRepo) DeleteSynonymGroup(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM synonym_groups WHERE id=$1`, id)
	return err
}

func (r *postgresRepo) ExpandQuery(ctx context.Context, query string) ([]string, error) {
	queryLower := strings.ToLower(strings.TrimSpace(query))
	rows, err := r.pool.Query(ctx, `
		SELECT canonical_term, synonyms FROM synonym_groups
		WHERE canonical_term ILIKE $1
		   OR $1 = ANY(synonyms)`,
		queryLower)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seen := map[string]bool{queryLower: true}
	var expanded []string
	for rows.Next() {
		var canonical string
		var synonyms []string
		if err := rows.Scan(&canonical, &synonyms); err != nil {
			return nil, err
		}
		if !seen[canonical] {
			seen[canonical] = true
			expanded = append(expanded, canonical)
		}
		for _, s := range synonyms {
			if !seen[s] {
				seen[s] = true
				expanded = append(expanded, s)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return expanded, nil
}

// ── Type-ahead ────────────────────────────────────────────────────────────────

func (r *postgresRepo) TypeAhead(ctx context.Context, prefix string, limit int) ([]model.SearchSuggestion, error) {
	if len(prefix) < 2 {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT term, usage_count FROM search_terms
		WHERE term ILIKE $1
		ORDER BY usage_count DESC, term ASC
		LIMIT $2`,
		prefix+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var suggestions []model.SearchSuggestion
	for rows.Next() {
		var s model.SearchSuggestion
		if err := rows.Scan(&s.Term, &s.UsageCount); err != nil {
			return nil, err
		}
		suggestions = append(suggestions, s)
	}
	return suggestions, rows.Err()
}

func (r *postgresRepo) IncrementSearchTerm(ctx context.Context, term string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO search_terms (id, term, usage_count, created_at, updated_at)
		VALUES (uuid_generate_v4(), $1, 1, NOW(), NOW())
		ON CONFLICT (term) DO UPDATE SET
			usage_count = search_terms.usage_count + 1,
			updated_at = NOW()`,
		strings.ToLower(strings.TrimSpace(term)))
	return err
}

// ── Search History ────────────────────────────────────────────────────────────

func (r *postgresRepo) AddSearchHistory(ctx context.Context, h *model.UserSearchHistory) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO user_search_history (id, user_id, query, filters_json, searched_at)
		VALUES ($1, $2, $3, '{}', NOW())`,
		h.ID, h.UserID, h.Query)
	return err
}

func (r *postgresRepo) GetSearchHistory(ctx context.Context, userID uuid.UUID, limit int) ([]model.UserSearchHistory, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, query, searched_at
		FROM user_search_history
		WHERE user_id=$1
		ORDER BY searched_at DESC
		LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []model.UserSearchHistory
	for rows.Next() {
		var h model.UserSearchHistory
		if err := rows.Scan(&h.ID, &h.UserID, &h.Query, &h.SearchedAt); err != nil {
			return nil, err
		}
		history = append(history, h)
	}
	return history, rows.Err()
}

func (r *postgresRepo) ClearSearchHistory(ctx context.Context, userID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM user_search_history WHERE user_id=$1`, userID)
	return err
}

func (r *postgresRepo) TrimSearchHistory(ctx context.Context, userID uuid.UUID, limit int) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM user_search_history
		WHERE user_id=$1
		  AND id NOT IN (
			SELECT id FROM user_search_history
			WHERE user_id=$1
			ORDER BY searched_at DESC
			LIMIT $2
		  )`, userID, limit)
	return err
}
