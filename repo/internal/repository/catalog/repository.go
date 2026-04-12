package catalogrepo

import (
	"context"

	"github.com/eduexchange/eduexchange/internal/model"
	"github.com/google/uuid"
)

// ResourceFilter controls list queries.
type ResourceFilter struct {
	Page       int
	PageSize   int
	Status     string
	AuthorID   *uuid.UUID
	CategoryID *uuid.UUID
	Tag        string
	Search     string
	Sort       string // "created_at" | "title"
	Order      string // "asc" | "desc"
}

// CatalogRepository is the persistence interface for the catalog domain.
type CatalogRepository interface {
	// ── Resources ──────────────────────────────────────────────────────────
	CreateResource(ctx context.Context, r *model.Resource) error
	GetResource(ctx context.Context, id uuid.UUID) (*model.Resource, error)
	UpdateResource(ctx context.Context, r *model.Resource) error
	ListResources(ctx context.Context, f ResourceFilter) ([]model.Resource, int, error)
	DeleteResource(ctx context.Context, id uuid.UUID) error

	// ── Resource Versions ──────────────────────────────────────────────────
	CreateVersion(ctx context.Context, v *model.ResourceVersion) error
	ListVersions(ctx context.Context, resourceID uuid.UUID) ([]model.ResourceVersion, error)

	// ── Resource Tags ──────────────────────────────────────────────────────
	SetTags(ctx context.Context, resourceID uuid.UUID, tagIDs []uuid.UUID) error
	GetTags(ctx context.Context, resourceID uuid.UUID) ([]model.Tag, error)

	// ── Resource Files ─────────────────────────────────────────────────────
	AddFile(ctx context.Context, f *model.ResourceFile) error
	GetFile(ctx context.Context, fileID uuid.UUID) (*model.ResourceFile, error)
	ListFiles(ctx context.Context, resourceID uuid.UUID) ([]model.ResourceFile, error)
	DeleteFile(ctx context.Context, fileID uuid.UUID) error
	CountFiles(ctx context.Context, resourceID uuid.UUID) (int, error)

	// ── Reviews ────────────────────────────────────────────────────────────
	CreateReview(ctx context.Context, rv *model.ResourceReview) error
	ListReviews(ctx context.Context, resourceID uuid.UUID) ([]model.ResourceReview, error)

	// ── Categories ─────────────────────────────────────────────────────────
	CreateCategory(ctx context.Context, c *model.Category) error
	GetCategory(ctx context.Context, id uuid.UUID) (*model.Category, error)
	UpdateCategory(ctx context.Context, c *model.Category) error
	DeleteCategory(ctx context.Context, id uuid.UUID) error
	ListCategories(ctx context.Context) ([]model.Category, error)

	// ── Tags ───────────────────────────────────────────────────────────────
	CreateTag(ctx context.Context, t *model.Tag) error
	GetTagByName(ctx context.Context, name string) (*model.Tag, error)
	GetTagByID(ctx context.Context, id uuid.UUID) (*model.Tag, error)
	ListTags(ctx context.Context, search string) ([]model.Tag, error)
	DeleteTag(ctx context.Context, id uuid.UUID) error

	// ── Bulk Import Jobs ───────────────────────────────────────────────────
	CreateImportJob(ctx context.Context, job *model.BulkImportJob) error
	GetImportJob(ctx context.Context, id uuid.UUID) (*model.BulkImportJob, error)
	UpdateImportJob(ctx context.Context, job *model.BulkImportJob) error

	// ── Rate Limit ─────────────────────────────────────────────────────────
	CheckAndIncrementRateLimit(ctx context.Context, userID uuid.UUID, action string, limit int) (bool, error)
}
