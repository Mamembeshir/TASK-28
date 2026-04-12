package catalogservice

import (
	"context"
	"strings"

	"github.com/eduexchange/eduexchange/internal/audit"
	"github.com/eduexchange/eduexchange/internal/model"
	catalogrepo "github.com/eduexchange/eduexchange/internal/repository/catalog"
	"github.com/google/uuid"
)

// CategoryService handles category CRUD (Admin only, enforced at handler level).
type CategoryService struct {
	repo     catalogrepo.CatalogRepository
	auditSvc *audit.Service
}

func NewCategoryService(repo catalogrepo.CatalogRepository, auditSvc *audit.Service) *CategoryService {
	return &CategoryService{repo: repo, auditSvc: auditSvc}
}

func (s *CategoryService) ListCategories(ctx context.Context) ([]model.Category, error) {
	return s.repo.ListCategories(ctx)
}

func (s *CategoryService) GetCategory(ctx context.Context, id uuid.UUID) (*model.Category, error) {
	return s.repo.GetCategory(ctx, id)
}

func (s *CategoryService) CreateCategory(ctx context.Context, actorID uuid.UUID, name string, parentID *uuid.UUID, sortOrder int) (*model.Category, error) {
	if ve := validateCategoryName(name); ve.HasErrors() {
		return nil, ve
	}

	level := 1
	if parentID != nil {
		parent, err := s.repo.GetCategory(ctx, *parentID)
		if err != nil {
			return nil, err
		}
		level = parent.Level + 1
		if level > 3 {
			ve := model.NewValidationErrors()
			ve.Add("parent_id", "Categories can only be nested up to 3 levels deep.")
			return nil, ve
		}
	}

	c := &model.Category{
		ID:        uuid.New(),
		Name:      strings.TrimSpace(name),
		ParentID:  parentID,
		Level:     level,
		SortOrder: sortOrder,
	}
	if err := s.repo.CreateCategory(ctx, c); err != nil {
		return nil, err
	}

	s.auditSvc.Record(ctx, audit.Entry{ //nolint:errcheck
		ActorID: actorID, Action: "category.create",
		EntityType: "category", EntityID: c.ID,
		AfterData: map[string]string{"name": c.Name},
	})

	return c, nil
}

func (s *CategoryService) UpdateCategory(ctx context.Context, actorID, id uuid.UUID, name string, sortOrder int) (*model.Category, error) {
	if ve := validateCategoryName(name); ve.HasErrors() {
		return nil, ve
	}

	c, err := s.repo.GetCategory(ctx, id)
	if err != nil {
		return nil, err
	}

	c.Name = strings.TrimSpace(name)
	c.SortOrder = sortOrder
	if err := s.repo.UpdateCategory(ctx, c); err != nil {
		return nil, err
	}

	s.auditSvc.Record(ctx, audit.Entry{ //nolint:errcheck
		ActorID: actorID, Action: "category.update",
		EntityType: "category", EntityID: id,
		AfterData: map[string]string{"name": c.Name},
	})

	return c, nil
}

func (s *CategoryService) DeleteCategory(ctx context.Context, actorID, id uuid.UUID) error {
	if _, err := s.repo.GetCategory(ctx, id); err != nil {
		return err
	}

	if err := s.repo.DeleteCategory(ctx, id); err != nil {
		return err
	}

	s.auditSvc.Record(ctx, audit.Entry{ //nolint:errcheck
		ActorID: actorID, Action: "category.delete",
		EntityType: "category", EntityID: id,
	})
	return nil
}

func validateCategoryName(name string) *model.ValidationErrors {
	ve := model.NewValidationErrors()
	name = strings.TrimSpace(name)
	if name == "" {
		ve.Add("name", "Category name is required.")
	} else if len(name) > 200 {
		ve.Add("name", "Category name must be at most 200 characters.")
	}
	return ve
}
