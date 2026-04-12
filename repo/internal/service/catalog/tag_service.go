package catalogservice

import (
	"context"
	"strings"

	"github.com/eduexchange/eduexchange/internal/audit"
	"github.com/eduexchange/eduexchange/internal/model"
	catalogrepo "github.com/eduexchange/eduexchange/internal/repository/catalog"
	"github.com/google/uuid"
)

// TagService handles tag CRUD. Authors and above can create tags.
type TagService struct {
	repo     catalogrepo.CatalogRepository
	auditSvc *audit.Service
}

func NewTagService(repo catalogrepo.CatalogRepository, auditSvc *audit.Service) *TagService {
	return &TagService{repo: repo, auditSvc: auditSvc}
}

func (s *TagService) ListTags(ctx context.Context) ([]model.Tag, error) {
	return s.repo.ListTags(ctx, "")
}

func (s *TagService) GetTag(ctx context.Context, id uuid.UUID) (*model.Tag, error) {
	return s.repo.GetTagByID(ctx, id)
}

func (s *TagService) CreateTag(ctx context.Context, actorID uuid.UUID, name string) (*model.Tag, error) {
	if ve := validateTagName(name); ve.HasErrors() {
		return nil, ve
	}

	name = strings.TrimSpace(name)

	// Return existing tag if name already exists (idempotent for authors).
	if existing, err := s.repo.GetTagByName(ctx, name); err == nil {
		return existing, nil
	}

	t := &model.Tag{
		ID:   uuid.New(),
		Name: name,
	}
	if err := s.repo.CreateTag(ctx, t); err != nil {
		return nil, err
	}

	s.auditSvc.Record(ctx, audit.Entry{ //nolint:errcheck
		ActorID: actorID, Action: "tag.create",
		EntityType: "tag", EntityID: t.ID,
		AfterData: map[string]string{"name": t.Name},
	})

	return t, nil
}

func (s *TagService) DeleteTag(ctx context.Context, actorID, id uuid.UUID) error {
	t, err := s.repo.GetTagByID(ctx, id)
	if err != nil {
		return err
	}

	if err := s.repo.DeleteTag(ctx, id); err != nil {
		return err
	}

	s.auditSvc.Record(ctx, audit.Entry{ //nolint:errcheck
		ActorID: actorID, Action: "tag.delete",
		EntityType: "tag", EntityID: id,
		BeforeData: map[string]string{"name": t.Name},
	})
	return nil
}

func validateTagName(name string) *model.ValidationErrors {
	ve := model.NewValidationErrors()
	name = strings.TrimSpace(name)
	if name == "" {
		ve.Add("name", "Tag name is required.")
	} else if len(name) > 100 {
		ve.Add("name", "Tag name must be at most 100 characters.")
	}
	return ve
}
