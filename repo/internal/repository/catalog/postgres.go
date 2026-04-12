package catalogrepo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/eduexchange/eduexchange/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresRepo struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) CatalogRepository {
	return &postgresRepo{pool: pool}
}

// ─── Resources ────────────────────────────────────────────────────────────────

func (r *postgresRepo) CreateResource(ctx context.Context, res *model.Resource) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO resources (id, title, description, content_body, author_id, category_id,
		  status, current_version_number, version, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,1,NOW(),NOW())`,
		res.ID, res.Title, res.Description, res.ContentBody,
		res.AuthorID, res.CategoryID, res.Status.String(),
		res.CurrentVersionNumber,
	)
	return err
}

func (r *postgresRepo) GetResource(ctx context.Context, id uuid.UUID) (*model.Resource, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT r.id, r.title, r.description, r.content_body, r.author_id, r.category_id,
		        r.status, r.current_version_number, r.version, r.created_at, r.updated_at,
		        COALESCE(u.username,'') AS author_name,
		        COALESCE(c.name,'') AS category_name
		 FROM resources r
		 LEFT JOIN users u ON u.id = r.author_id
		 LEFT JOIN categories c ON c.id = r.category_id
		 WHERE r.id = $1`, id)

	res := &model.Resource{}
	var statusStr string
	err := row.Scan(
		&res.ID, &res.Title, &res.Description, &res.ContentBody,
		&res.AuthorID, &res.CategoryID, &statusStr,
		&res.CurrentVersionNumber, &res.Version, &res.CreatedAt, &res.UpdatedAt,
		&res.AuthorName, &res.CategoryName,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	res.Status, err = model.ParseResourceStatus(statusStr)
	if err != nil {
		return nil, err
	}

	res.Tags, _ = r.GetTags(ctx, res.ID)
	res.Files, _ = r.ListFiles(ctx, res.ID)
	return res, nil
}

func (r *postgresRepo) UpdateResource(ctx context.Context, res *model.Resource) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE resources
		 SET title=$1, description=$2, content_body=$3, category_id=$4, status=$5,
		     current_version_number=$6, version=version+1, updated_at=NOW()
		 WHERE id=$7 AND version=$8`,
		res.Title, res.Description, res.ContentBody, res.CategoryID,
		res.Status.String(), res.CurrentVersionNumber, res.ID, res.Version,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return model.ErrStaleVersion
	}
	return nil
}

func (r *postgresRepo) ListResources(ctx context.Context, f ResourceFilter) ([]model.Resource, int, error) {
	if f.PageSize <= 0 {
		f.PageSize = 25
	}
	if f.Page <= 0 {
		f.Page = 1
	}
	if f.Order != "asc" {
		f.Order = "desc"
	}
	allowedSort := map[string]bool{"created_at": true, "title": true, "updated_at": true}
	if !allowedSort[f.Sort] {
		f.Sort = "created_at"
	}

	where := "WHERE 1=1"
	args := []interface{}{}
	n := 1

	if f.Status != "" {
		where += fmt.Sprintf(" AND r.status = $%d", n)
		args = append(args, f.Status)
		n++
	}
	if f.AuthorID != nil {
		where += fmt.Sprintf(" AND r.author_id = $%d", n)
		args = append(args, *f.AuthorID)
		n++
	}
	if f.CategoryID != nil {
		where += fmt.Sprintf(" AND r.category_id = $%d", n)
		args = append(args, *f.CategoryID)
		n++
	}
	if f.Tag != "" {
		where += fmt.Sprintf(` AND EXISTS (
			SELECT 1 FROM resource_tags rt
			JOIN tags t ON t.id = rt.tag_id
			WHERE rt.resource_id = r.id AND t.name = $%d)`, n)
		args = append(args, f.Tag)
		n++
	}
	if f.Search != "" {
		where += fmt.Sprintf(" AND (r.title ILIKE $%d OR r.description ILIKE $%d)", n, n)
		args = append(args, "%"+f.Search+"%")
		n++
	}

	var total int
	if err := r.pool.QueryRow(ctx,
		fmt.Sprintf("SELECT COUNT(*) FROM resources r %s", where), args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	orderClause := fmt.Sprintf("ORDER BY r.%s %s", f.Sort, strings.ToUpper(f.Order))
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	dataSQL := fmt.Sprintf(`
		SELECT r.id, r.title, r.description, r.content_body, r.author_id, r.category_id,
		       r.status, r.current_version_number, r.version, r.created_at, r.updated_at,
		       COALESCE(u.username,'') AS author_name,
		       COALESCE(c.name,'') AS category_name
		FROM resources r
		LEFT JOIN users u ON u.id = r.author_id
		LEFT JOIN categories c ON c.id = r.category_id
		%s %s LIMIT $%d OFFSET $%d`, where, orderClause, n, n+1)

	rows, err := r.pool.Query(ctx, dataSQL, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var results []model.Resource
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
		res.Tags, _ = r.GetTags(ctx, res.ID)
		results = append(results, res)
	}
	return results, total, rows.Err()
}

func (r *postgresRepo) DeleteResource(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM resources WHERE id = $1`, id)
	return err
}

// ─── Resource Versions ────────────────────────────────────────────────────────

func (r *postgresRepo) CreateVersion(ctx context.Context, v *model.ResourceVersion) error {
	snap, err := json.Marshal(v.DataSnapshot)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO resource_versions (id, resource_id, version_number, data_snapshot, changed_by, created_at)
		 VALUES ($1,$2,$3,$4,$5,NOW())`,
		v.ID, v.ResourceID, v.VersionNumber, snap, v.ChangedBy,
	)
	return err
}

func (r *postgresRepo) ListVersions(ctx context.Context, resourceID uuid.UUID) ([]model.ResourceVersion, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, resource_id, version_number, data_snapshot, changed_by, created_at
		 FROM resource_versions WHERE resource_id = $1 ORDER BY version_number DESC`, resourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []model.ResourceVersion
	for rows.Next() {
		var v model.ResourceVersion
		if err := rows.Scan(&v.ID, &v.ResourceID, &v.VersionNumber, &v.DataSnapshot, &v.ChangedBy, &v.CreatedAt); err != nil {
			return nil, err
		}
		results = append(results, v)
	}
	return results, rows.Err()
}

// ─── Resource Tags ────────────────────────────────────────────────────────────

func (r *postgresRepo) SetTags(ctx context.Context, resourceID uuid.UUID, tagIDs []uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, `DELETE FROM resource_tags WHERE resource_id = $1`, resourceID); err != nil {
		return err
	}
	for _, tid := range tagIDs {
		if _, err := tx.Exec(ctx,
			`INSERT INTO resource_tags (resource_id, tag_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`,
			resourceID, tid); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *postgresRepo) GetTags(ctx context.Context, resourceID uuid.UUID) ([]model.Tag, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT t.id, t.name, t.created_at
		 FROM tags t
		 JOIN resource_tags rt ON rt.tag_id = t.id
		 WHERE rt.resource_id = $1
		 ORDER BY t.name`, resourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []model.Tag
	for rows.Next() {
		var t model.Tag
		if err := rows.Scan(&t.ID, &t.Name, &t.CreatedAt); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

// ─── Resource Files ───────────────────────────────────────────────────────────

func (r *postgresRepo) AddFile(ctx context.Context, f *model.ResourceFile) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO resource_files (id, resource_id, original_name, stored_path, mime_type, size_bytes, sha256, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,NOW())`,
		f.ID, f.ResourceID, f.OriginalName, f.StoredPath, f.MIMEType, f.SizeBytes, f.SHA256,
	)
	return err
}

func (r *postgresRepo) GetFile(ctx context.Context, fileID uuid.UUID) (*model.ResourceFile, error) {
	f := &model.ResourceFile{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, resource_id, original_name, stored_path, mime_type, size_bytes, sha256, created_at
		 FROM resource_files WHERE id = $1`, fileID,
	).Scan(&f.ID, &f.ResourceID, &f.OriginalName, &f.StoredPath, &f.MIMEType, &f.SizeBytes, &f.SHA256, &f.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	return f, nil
}

func (r *postgresRepo) ListFiles(ctx context.Context, resourceID uuid.UUID) ([]model.ResourceFile, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, resource_id, original_name, stored_path, mime_type, size_bytes, sha256, created_at
		 FROM resource_files WHERE resource_id = $1 ORDER BY created_at`, resourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []model.ResourceFile
	for rows.Next() {
		var f model.ResourceFile
		if err := rows.Scan(&f.ID, &f.ResourceID, &f.OriginalName, &f.StoredPath, &f.MIMEType, &f.SizeBytes, &f.SHA256, &f.CreatedAt); err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

func (r *postgresRepo) DeleteFile(ctx context.Context, fileID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM resource_files WHERE id = $1`, fileID)
	return err
}

func (r *postgresRepo) CountFiles(ctx context.Context, resourceID uuid.UUID) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM resource_files WHERE resource_id = $1`, resourceID).Scan(&count)
	return count, err
}

// ─── Reviews ─────────────────────────────────────────────────────────────────

func (r *postgresRepo) CreateReview(ctx context.Context, rv *model.ResourceReview) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO resource_reviews (id, resource_id, reviewer_id, action, notes, created_at)
		 VALUES ($1,$2,$3,$4,$5,NOW())`,
		rv.ID, rv.ResourceID, rv.ReviewerID, rv.Action, rv.Notes,
	)
	return err
}

func (r *postgresRepo) ListReviews(ctx context.Context, resourceID uuid.UUID) ([]model.ResourceReview, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT rv.id, rv.resource_id, rv.reviewer_id, rv.action, rv.notes, rv.created_at,
		        COALESCE(u.username,'') AS reviewer_name
		 FROM resource_reviews rv
		 LEFT JOIN users u ON u.id = rv.reviewer_id
		 WHERE rv.resource_id = $1
		 ORDER BY rv.created_at DESC`, resourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reviews []model.ResourceReview
	for rows.Next() {
		var rv model.ResourceReview
		if err := rows.Scan(&rv.ID, &rv.ResourceID, &rv.ReviewerID, &rv.Action, &rv.Notes, &rv.CreatedAt, &rv.ReviewerName); err != nil {
			return nil, err
		}
		reviews = append(reviews, rv)
	}
	return reviews, rows.Err()
}

// ─── Categories ───────────────────────────────────────────────────────────────

func (r *postgresRepo) CreateCategory(ctx context.Context, c *model.Category) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO categories (id, name, parent_id, level, sort_order, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,NOW(),NOW())`,
		c.ID, c.Name, c.ParentID, c.Level, c.SortOrder,
	)
	return err
}

func (r *postgresRepo) GetCategory(ctx context.Context, id uuid.UUID) (*model.Category, error) {
	c := &model.Category{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, parent_id, level, sort_order, created_at, updated_at
		 FROM categories WHERE id = $1`, id,
	).Scan(&c.ID, &c.Name, &c.ParentID, &c.Level, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	return c, nil
}

func (r *postgresRepo) UpdateCategory(ctx context.Context, c *model.Category) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE categories SET name=$1, parent_id=$2, level=$3, sort_order=$4, updated_at=NOW() WHERE id=$5`,
		c.Name, c.ParentID, c.Level, c.SortOrder, c.ID,
	)
	return err
}

func (r *postgresRepo) DeleteCategory(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM categories WHERE id = $1`, id)
	return err
}

func (r *postgresRepo) ListCategories(ctx context.Context) ([]model.Category, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, parent_id, level, sort_order, created_at, updated_at
		 FROM categories ORDER BY level, sort_order, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cats []model.Category
	for rows.Next() {
		var c model.Category
		if err := rows.Scan(&c.ID, &c.Name, &c.ParentID, &c.Level, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		cats = append(cats, c)
	}
	return cats, rows.Err()
}

// ─── Tags ─────────────────────────────────────────────────────────────────────

func (r *postgresRepo) CreateTag(ctx context.Context, t *model.Tag) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO tags (id, name, created_at) VALUES ($1,$2,NOW()) ON CONFLICT (name) DO NOTHING`,
		t.ID, t.Name,
	)
	return err
}

func (r *postgresRepo) GetTagByName(ctx context.Context, name string) (*model.Tag, error) {
	t := &model.Tag{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, created_at FROM tags WHERE name = $1`, name,
	).Scan(&t.ID, &t.Name, &t.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	return t, nil
}

func (r *postgresRepo) GetTagByID(ctx context.Context, id uuid.UUID) (*model.Tag, error) {
	t := &model.Tag{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, created_at FROM tags WHERE id = $1`, id,
	).Scan(&t.ID, &t.Name, &t.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	return t, nil
}

func (r *postgresRepo) ListTags(ctx context.Context, search string) ([]model.Tag, error) {
	query := `SELECT id, name, created_at FROM tags`
	args := []interface{}{}
	if search != "" {
		query += ` WHERE name ILIKE $1`
		args = append(args, "%"+search+"%")
	}
	query += ` ORDER BY name LIMIT 100`

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []model.Tag
	for rows.Next() {
		var t model.Tag
		if err := rows.Scan(&t.ID, &t.Name, &t.CreatedAt); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

func (r *postgresRepo) DeleteTag(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM tags WHERE id = $1`, id)
	return err
}

// ─── Bulk Import Jobs ────────────────────────────────────────────────────────

func (r *postgresRepo) CreateImportJob(ctx context.Context, job *model.BulkImportJob) error {
	resultsJSON, _ := json.Marshal(job.Results)
	_, err := r.pool.Exec(ctx,
		`INSERT INTO bulk_import_jobs (id, uploaded_by, file_path, original_filename, status,
		  total_rows, valid_rows, invalid_rows, results_json, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NOW())`,
		job.ID, job.UploadedBy, job.FilePath, job.OriginalFilename, job.Status.String(),
		job.TotalRows, job.ValidRows, job.InvalidRows, resultsJSON,
	)
	return err
}

func (r *postgresRepo) GetImportJob(ctx context.Context, id uuid.UUID) (*model.BulkImportJob, error) {
	job := &model.BulkImportJob{}
	var statusStr string
	var resultsJSON []byte
	err := r.pool.QueryRow(ctx,
		`SELECT id, uploaded_by, file_path, original_filename, status, total_rows, valid_rows,
		        invalid_rows, results_json, created_at, completed_at
		 FROM bulk_import_jobs WHERE id = $1`, id,
	).Scan(&job.ID, &job.UploadedBy, &job.FilePath, &job.OriginalFilename, &statusStr,
		&job.TotalRows, &job.ValidRows, &job.InvalidRows, &resultsJSON, &job.CreatedAt, &job.CompletedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	job.Status, _ = model.ParseBulkImportStatus(statusStr)
	if len(resultsJSON) > 0 {
		json.Unmarshal(resultsJSON, &job.Results) //nolint:errcheck
	}
	return job, nil
}

func (r *postgresRepo) UpdateImportJob(ctx context.Context, job *model.BulkImportJob) error {
	resultsJSON, _ := json.Marshal(job.Results)
	_, err := r.pool.Exec(ctx,
		`UPDATE bulk_import_jobs
		 SET status=$1, total_rows=$2, valid_rows=$3, invalid_rows=$4, results_json=$5, completed_at=$6
		 WHERE id=$7`,
		job.Status.String(), job.TotalRows, job.ValidRows, job.InvalidRows, resultsJSON, job.CompletedAt, job.ID,
	)
	return err
}

// ─── Rate Limit ───────────────────────────────────────────────────────────────

// CheckAndIncrementRateLimit returns true if the action is allowed (under limit).
// Uses a fixed clock-hour window per MOD-01 / questions.md 4.2.
func (r *postgresRepo) CheckAndIncrementRateLimit(ctx context.Context, userID uuid.UUID, action string, limit int) (bool, error) {
	windowStart := time.Now().UTC().Truncate(time.Hour)

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Upsert the counter
	var count int
	err = tx.QueryRow(ctx, `
		INSERT INTO rate_limit_counters (user_id, action_type, window_start, count)
		VALUES ($1, $2, $3, 1)
		ON CONFLICT (user_id, action_type, window_start)
		DO UPDATE SET count = rate_limit_counters.count + 1
		RETURNING count`,
		userID, action, windowStart,
	).Scan(&count)
	if err != nil {
		return false, err
	}

	if count > limit {
		// Don't commit — keep the count at limit (rollback the extra increment)
		tx.Rollback(ctx) //nolint:errcheck
		return false, nil
	}

	return true, tx.Commit(ctx)
}
