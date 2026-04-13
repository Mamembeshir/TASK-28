package catalogservice

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/eduexchange/eduexchange/internal/audit"
	"github.com/eduexchange/eduexchange/internal/model"
	catalogrepo "github.com/eduexchange/eduexchange/internal/repository/catalog"
	engagementrepo "github.com/eduexchange/eduexchange/internal/repository/engagement"
	"github.com/gabriel-vasile/mimetype"
	"github.com/google/uuid"
)

// NotificationSender is the interface used to fire SSE/DB notifications
// without importing the messaging service directly (avoids import cycles).
type NotificationSender interface {
	Send(ctx context.Context, userID uuid.UUID, eventType model.EventType, title, body string, resourceID *uuid.UUID) error
}

// SearchIndexUpdater is the interface for refreshing the pinyin/tag search index
// after a resource is created or updated, without importing the search service directly.
type SearchIndexUpdater interface {
	UpdateSearchIndex(ctx context.Context, r *model.Resource) error
}

const resourceRateLimit = 20 // posts per clock-hour (MOD-01)

// ResourceInput is the data for creating or updating a resource.
type ResourceInput struct {
	Title       string
	Description string
	ContentBody string
	CategoryID  *uuid.UUID
	TagNames    []string // tag names (created if missing)
}

// CatalogService handles the resource lifecycle.
type CatalogService struct {
	repo               catalogrepo.CatalogRepository
	auditSvc           *audit.Service
	uploadDir          string
	notifSvc           NotificationSender
	engRepo            engagementrepo.EngagementRepository
	searchIndexUpdater SearchIndexUpdater
}

func NewCatalogService(repo catalogrepo.CatalogRepository, auditSvc *audit.Service, uploadDir string) *CatalogService {
	return &CatalogService{repo: repo, auditSvc: auditSvc, uploadDir: uploadDir}
}

// SetNotificationSender wires in the notification sender and engagement repo
// (for publish→follow_new_content fan-out). Called after construction.
func (s *CatalogService) SetNotificationSender(n NotificationSender, engRepo engagementrepo.EngagementRepository) {
	s.notifSvc = n
	s.engRepo = engRepo
}

// SetSearchIndexUpdater wires in the search index updater. Called after construction.
func (s *CatalogService) SetSearchIndexUpdater(si SearchIndexUpdater) {
	s.searchIndexUpdater = si
}

// ── CreateDraft: Author creates a new DRAFT resource + version 1 ───────────────

func (s *CatalogService) CreateDraft(ctx context.Context, authorID uuid.UUID, callerRoles []string, input ResourceInput) (*model.Resource, error) {
	// Service-level defence-in-depth: only AUTHORS or ADMINs may create entries.
	if !containsRole(callerRoles, "AUTHOR") && !containsRole(callerRoles, "ADMIN") {
		return nil, model.ErrForbidden
	}

	if errs := ValidateResourceInput(input.Title, input.Description); errs.HasErrors() {
		return nil, errs
	}

	// Rate limit: 20 resource creates per hour (MOD-01)
	allowed, err := s.repo.CheckAndIncrementRateLimit(ctx, authorID, "resource_create", resourceRateLimit)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, &RateLimitError{RetryAfter: 3600}
	}

	res := &model.Resource{
		ID:                   uuid.New(),
		Title:                input.Title,
		Description:          input.Description,
		ContentBody:          input.ContentBody,
		AuthorID:             authorID,
		CategoryID:           input.CategoryID,
		Status:               model.ResourceStatusDraft,
		CurrentVersionNumber: 1,
	}

	if err := s.repo.CreateResource(ctx, res); err != nil {
		return nil, err
	}

	// Resolve + set tags
	tagIDs, err := s.resolveTagIDs(ctx, input.TagNames)
	if err != nil {
		return nil, err
	}
	if err := s.repo.SetTags(ctx, res.ID, tagIDs); err != nil {
		return nil, err
	}

	// Create immutable version 1
	if err := s.createVersionSnapshot(ctx, res, input.TagNames, authorID); err != nil {
		return nil, err
	}

	s.auditSvc.Record(ctx, audit.Entry{ //nolint:errcheck
		ActorID: authorID, Action: "resource.create",
		EntityType: "resource", EntityID: res.ID,
		AfterData: map[string]interface{}{"title": res.Title, "status": "DRAFT"},
	})

	created, err := s.repo.GetResource(ctx, res.ID)
	if err != nil {
		return nil, err
	}
	if s.searchIndexUpdater != nil {
		_ = s.searchIndexUpdater.UpdateSearchIndex(ctx, created)
	}
	return created, nil
}

// ── UpdateDraft: Creates a new version snapshot, resource stays DRAFT ─────────

func (s *CatalogService) UpdateDraft(ctx context.Context, resourceID, editorID uuid.UUID, input ResourceInput, version int) (*model.Resource, error) {
	res, err := s.repo.GetResource(ctx, resourceID)
	if err != nil {
		return nil, err
	}
	if res.AuthorID != editorID {
		return nil, model.ErrForbidden
	}
	if res.Status != model.ResourceStatusDraft && res.Status != model.ResourceStatusRejected {
		ve := model.NewValidationErrors()
		ve.Add("status", "Only DRAFT or REJECTED resources can be edited.")
		return nil, ve
	}
	if errs := ValidateResourceInput(input.Title, input.Description); errs.HasErrors() {
		return nil, errs
	}

	before := map[string]interface{}{"title": res.Title, "status": res.Status.String()}

	res.Title = input.Title
	res.Description = input.Description
	res.ContentBody = input.ContentBody
	res.CategoryID = input.CategoryID
	res.Version = version
	res.CurrentVersionNumber++

	if err := s.repo.UpdateResource(ctx, res); err != nil {
		return nil, err
	}

	tagIDs, err := s.resolveTagIDs(ctx, input.TagNames)
	if err != nil {
		return nil, err
	}
	if err := s.repo.SetTags(ctx, res.ID, tagIDs); err != nil {
		return nil, err
	}

	if err := s.createVersionSnapshot(ctx, res, input.TagNames, editorID); err != nil {
		return nil, err
	}

	s.auditSvc.Record(ctx, audit.Entry{ //nolint:errcheck
		ActorID: editorID, Action: "resource.update",
		EntityType: "resource", EntityID: res.ID,
		BeforeData: before, AfterData: map[string]interface{}{"title": res.Title},
		Source: "catalog", Reason: "author draft update",
	})

	updated, err := s.repo.GetResource(ctx, resourceID)
	if err != nil {
		return nil, err
	}
	if s.searchIndexUpdater != nil {
		_ = s.searchIndexUpdater.UpdateSearchIndex(ctx, updated)
	}
	return updated, nil
}

// ── SubmitForReview: DRAFT → PENDING_REVIEW ───────────────────────────────────

func (s *CatalogService) SubmitForReview(ctx context.Context, resourceID, authorID uuid.UUID, version int) (*model.Resource, error) {
	res, err := s.repo.GetResource(ctx, resourceID)
	if err != nil {
		return nil, err
	}
	if res.AuthorID != authorID {
		return nil, model.ErrForbidden
	}
	return s.transition(ctx, resourceID, authorID, model.ResourceStatusPendingReview, version, "resource.submit")
}

// ── EditPublished: questions.md 1.2 — new version, status→PENDING_REVIEW ──────
// Old published version remains visible until re-approval and re-publish.

func (s *CatalogService) EditPublished(ctx context.Context, resourceID, editorID uuid.UUID, input ResourceInput, version int) (*model.Resource, error) {
	res, err := s.repo.GetResource(ctx, resourceID)
	if err != nil {
		return nil, err
	}
	if res.Status != model.ResourceStatusPublished {
		ve := model.NewValidationErrors()
		ve.Add("status", "EditPublished is only valid for PUBLISHED resources.")
		return nil, ve
	}
	if res.AuthorID != editorID {
		return nil, model.ErrForbidden
	}
	if errs := ValidateResourceInput(input.Title, input.Description); errs.HasErrors() {
		return nil, errs
	}

	before := map[string]interface{}{"title": res.Title, "status": "PUBLISHED"}

	res.Title = input.Title
	res.Description = input.Description
	res.ContentBody = input.ContentBody
	res.CategoryID = input.CategoryID
	res.Version = version
	res.Status = model.ResourceStatusPendingReview
	res.CurrentVersionNumber++

	if err := s.repo.UpdateResource(ctx, res); err != nil {
		return nil, err
	}

	tagIDs, err := s.resolveTagIDs(ctx, input.TagNames)
	if err != nil {
		return nil, err
	}
	if err := s.repo.SetTags(ctx, res.ID, tagIDs); err != nil {
		return nil, err
	}

	if err := s.createVersionSnapshot(ctx, res, input.TagNames, editorID); err != nil {
		return nil, err
	}

	s.auditSvc.Record(ctx, audit.Entry{ //nolint:errcheck
		ActorID: editorID, Action: "resource.edit_published",
		EntityType: "resource", EntityID: res.ID,
		BeforeData: before, AfterData: map[string]interface{}{"status": "PENDING_REVIEW"},
		Source: "catalog", Reason: "edit published resource — re-enters review",
	})

	edited, err := s.repo.GetResource(ctx, resourceID)
	if err != nil {
		return nil, err
	}
	if s.searchIndexUpdater != nil {
		_ = s.searchIndexUpdater.UpdateSearchIndex(ctx, edited)
	}
	return edited, nil
}

// ── Approve: PENDING_REVIEW → APPROVED ───────────────────────────────────────

func (s *CatalogService) Approve(ctx context.Context, resourceID, reviewerID uuid.UUID, version int) (*model.Resource, error) {
	res, err := s.transition(ctx, resourceID, reviewerID, model.ResourceStatusApproved, version, "resource.approve")
	if err != nil {
		return nil, err
	}
	s.repo.CreateReview(ctx, &model.ResourceReview{ //nolint:errcheck
		ID: uuid.New(), ResourceID: resourceID, ReviewerID: reviewerID, Action: "APPROVED",
	})
	if s.notifSvc != nil {
		s.notifSvc.Send(ctx, res.AuthorID, model.EventReviewDecision, //nolint:errcheck
			"Your resource was approved",
			fmt.Sprintf(`"%s" has been approved and is ready to publish.`, res.Title),
			&resourceID)
	}
	return res, nil
}

// ── Reject: PENDING_REVIEW → REJECTED ────────────────────────────────────────

func (s *CatalogService) Reject(ctx context.Context, resourceID, reviewerID uuid.UUID, notes string, version int) (*model.Resource, error) {
	res, err := s.transition(ctx, resourceID, reviewerID, model.ResourceStatusRejected, version, "resource.reject")
	if err != nil {
		return nil, err
	}
	s.repo.CreateReview(ctx, &model.ResourceReview{ //nolint:errcheck
		ID: uuid.New(), ResourceID: resourceID, ReviewerID: reviewerID, Action: "REJECTED", Notes: notes,
	})
	if s.notifSvc != nil {
		s.notifSvc.Send(ctx, res.AuthorID, model.EventReviewDecision, //nolint:errcheck
			"Your resource was rejected",
			fmt.Sprintf(`"%s" was not approved. Reviewer notes: %s`, res.Title, notes),
			&resourceID)
	}
	return res, nil
}

// ── Publish: APPROVED → PUBLISHED ────────────────────────────────────────────

func (s *CatalogService) Publish(ctx context.Context, resourceID, authorID uuid.UUID, version int) (*model.Resource, error) {
	res, err := s.transition(ctx, resourceID, authorID, model.ResourceStatusPublished, version, "resource.publish")
	if err != nil {
		return nil, err
	}
	if s.notifSvc != nil {
		// Notify the resource author that their content is now live.
		s.notifSvc.Send(ctx, res.AuthorID, model.EventPublishComplete, //nolint:errcheck
			"Your resource is now live",
			fmt.Sprintf(`"%s" has been published and is visible to all users.`, res.Title),
			&resourceID)
		// Fan-out to followers of the resource author.
		if s.engRepo != nil {
			if followerIDs, err := s.engRepo.ListFollowerIDs(ctx, res.AuthorID); err == nil {
				for _, followerID := range followerIDs {
					s.notifSvc.Send(ctx, followerID, model.EventFollowNewContent, //nolint:errcheck
						"New content from someone you follow",
						fmt.Sprintf(`"%s" just published a new resource.`, res.Title),
						&resourceID)
				}
			}
		}
	}
	return res, nil
}

// ── ReviseRejected: REJECTED → DRAFT ─────────────────────────────────────────

func (s *CatalogService) ReviseRejected(ctx context.Context, resourceID, authorID uuid.UUID, version int) (*model.Resource, error) {
	res, err := s.repo.GetResource(ctx, resourceID)
	if err != nil {
		return nil, err
	}
	if res.AuthorID != authorID {
		return nil, model.ErrForbidden
	}
	return s.transition(ctx, resourceID, authorID, model.ResourceStatusDraft, version, "resource.revise")
}

// ── Takedown: PUBLISHED → TAKEN_DOWN ─────────────────────────────────────────

func (s *CatalogService) Takedown(ctx context.Context, resourceID, moderatorID uuid.UUID, reason string, version int) (*model.Resource, error) {
	res, err := s.transition(ctx, resourceID, moderatorID, model.ResourceStatusTakenDown, version, "resource.takedown")
	if err != nil {
		return nil, err
	}
	s.auditSvc.Record(ctx, audit.Entry{ //nolint:errcheck
		ActorID: moderatorID, Action: "resource.takedown",
		EntityType: "resource", EntityID: resourceID,
		BeforeData: map[string]string{"status": "PUBLISHED"},
		AfterData:  map[string]string{"status": "TAKEN_DOWN"},
		Reason:     reason,
	})
	return res, nil
}

// ── Restore: TAKEN_DOWN → PUBLISHED ──────────────────────────────────────────

func (s *CatalogService) Restore(ctx context.Context, resourceID, adminID uuid.UUID, version int) (*model.Resource, error) {
	return s.transition(ctx, resourceID, adminID, model.ResourceStatusPublished, version, "resource.restore")
}

// ── UploadFile: validate type/size, store, hash ───────────────────────────────

func (s *CatalogService) UploadFile(ctx context.Context, resourceID, uploaderID uuid.UUID, uploaderRoles []string, reader io.Reader, originalName string, size int64) (*model.ResourceFile, error) {
	res, err := s.repo.GetResource(ctx, resourceID)
	if err != nil {
		return nil, err
	}
	if res.AuthorID != uploaderID && !containsRole(uploaderRoles, "ADMIN") {
		return nil, model.ErrForbidden
	}
	// CAT-06: max 50 MB
	if size > model.MaxFileSize {
		ve := model.NewValidationErrors()
		ve.Add("file", fmt.Sprintf("File exceeds maximum size of 50 MB (received %d bytes).", size))
		return nil, ve
	}

	// CAT-06: max 5 files per resource
	count, err := s.repo.CountFiles(ctx, resourceID)
	if err != nil {
		return nil, err
	}
	if count >= model.MaxFilesPerResource {
		ve := model.NewValidationErrors()
		ve.Add("file", fmt.Sprintf("Resource already has the maximum of %d files.", model.MaxFilesPerResource))
		return nil, ve
	}

	// Read content and compute SHA-256
	content, err := io.ReadAll(io.LimitReader(reader, model.MaxFileSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > model.MaxFileSize {
		ve := model.NewValidationErrors()
		ve.Add("file", "File exceeds maximum size of 50 MB.")
		return nil, ve
	}

	// Validate MIME type from content (not extension)
	mt := mimetype.Detect(content)
	if err := ValidateMIMEType(mt.String()); err != nil {
		ve := model.NewValidationErrors()
		ve.Add("file", err.Error())
		return nil, ve
	}

	ext := model.AllowedMIMETypes[mt.String()]
	fileID := uuid.New()
	storedName := fileID.String() + ext
	dirPath := filepath.Join(s.uploadDir, resourceID.String())
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return nil, fmt.Errorf("cannot create upload directory: %w", err)
	}
	storedPath := filepath.Join(dirPath, storedName)

	if err := os.WriteFile(storedPath, content, 0644); err != nil {
		return nil, fmt.Errorf("cannot write file: %w", err)
	}

	sum := sha256.Sum256(content)
	hashStr := hex.EncodeToString(sum[:])

	f := &model.ResourceFile{
		ID:           fileID,
		ResourceID:   resourceID,
		OriginalName: originalName,
		StoredPath:   storedPath,
		MIMEType:     mt.String(),
		SizeBytes:    int64(len(content)),
		SHA256:       hashStr,
	}
	if err := s.repo.AddFile(ctx, f); err != nil {
		os.Remove(storedPath) //nolint:errcheck
		return nil, err
	}

	return f, nil
}

// ── DeleteFile ────────────────────────────────────────────────────────────────

func (s *CatalogService) DeleteFile(ctx context.Context, resourceID, fileID, actorID uuid.UUID, actorRoles []string) error {
	f, err := s.repo.GetFile(ctx, fileID)
	if err != nil {
		return err
	}
	if f.ResourceID != resourceID {
		return model.ErrNotFound
	}
	res, err := s.repo.GetResource(ctx, f.ResourceID)
	if err != nil {
		return err
	}
	if res.AuthorID != actorID && !containsRole(actorRoles, "ADMIN") {
		return model.ErrForbidden
	}
	if err := s.repo.DeleteFile(ctx, fileID); err != nil {
		return err
	}
	os.Remove(f.StoredPath) //nolint:errcheck
	return nil
}

// ── List / Get ────────────────────────────────────────────────────────────────

func (s *CatalogService) DeleteResource(ctx context.Context, id, actorID uuid.UUID, actorRoles []string) error {
	res, err := s.repo.GetResource(ctx, id)
	if err != nil {
		return err
	}
	if res.AuthorID != actorID && !containsRole(actorRoles, "ADMIN") {
		return model.ErrForbidden
	}
	return s.repo.DeleteResource(ctx, id)
}

func (s *CatalogService) ListResources(ctx context.Context, f catalogrepo.ResourceFilter) ([]model.Resource, int, error) {
	return s.repo.ListResources(ctx, f)
}

func (s *CatalogService) GetResource(ctx context.Context, id uuid.UUID) (*model.Resource, error) {
	return s.repo.GetResource(ctx, id)
}

func (s *CatalogService) ListVersions(ctx context.Context, resourceID uuid.UUID) ([]model.ResourceVersion, error) {
	return s.repo.ListVersions(ctx, resourceID)
}

func (s *CatalogService) ListReviews(ctx context.Context, resourceID uuid.UUID) ([]model.ResourceReview, error) {
	return s.repo.ListReviews(ctx, resourceID)
}

// GetFile fetches a resource file and enforces visibility access policy:
//   - PUBLISHED: any authenticated caller may download.
//   - DRAFT / PENDING_REVIEW / APPROVED / REJECTED: only the resource author or ADMIN/REVIEWER.
//   - TAKEN_DOWN: only ADMIN.
func (s *CatalogService) GetFile(ctx context.Context, resourceID, fileID, callerID uuid.UUID, callerRoles []string) (*model.ResourceFile, error) {
	f, err := s.repo.GetFile(ctx, fileID)
	if err != nil {
		return nil, err
	}
	if f.ResourceID != resourceID {
		return nil, model.ErrNotFound
	}

	res, err := s.repo.GetResource(ctx, resourceID)
	if err != nil {
		return nil, err
	}

	isAdmin := containsRole(callerRoles, "ADMIN")
	isReviewer := containsRole(callerRoles, "REVIEWER")
	isAuthor := res.AuthorID == callerID

	switch res.Status {
	case model.ResourceStatusPublished:
		// any authenticated user is allowed
	case model.ResourceStatusTakenDown:
		if !isAdmin {
			return nil, model.ErrForbidden
		}
	default:
		// DRAFT, PENDING_REVIEW, APPROVED, REJECTED
		if !isAuthor && !isAdmin && !isReviewer {
			return nil, model.ErrForbidden
		}
	}

	return f, nil
}


// ── Helpers ───────────────────────────────────────────────────────────────────

func (s *CatalogService) transition(ctx context.Context, resourceID, actorID uuid.UUID, to model.ResourceStatus, version int, auditAction string) (*model.Resource, error) {
	res, err := s.repo.GetResource(ctx, resourceID)
	if err != nil {
		return nil, err
	}

	if err := ValidResourceStatusTransition(res.Status, to); err != nil {
		ve := model.NewValidationErrors()
		ve.Add("status", err.Error())
		return nil, ve
	}

	before := res.Status
	res.Status = to
	res.Version = version

	if err := s.repo.UpdateResource(ctx, res); err != nil {
		return nil, err
	}

	s.auditSvc.Record(ctx, audit.Entry{ //nolint:errcheck
		ActorID: actorID, Action: auditAction,
		EntityType: "resource", EntityID: resourceID,
		BeforeData: map[string]string{"status": before.String()},
		AfterData:  map[string]string{"status": to.String()},
		Source: "catalog", Reason: auditAction,
	})

	return s.repo.GetResource(ctx, resourceID)
}

func (s *CatalogService) resolveTagIDs(ctx context.Context, tagNames []string) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	for _, name := range tagNames {
		if name == "" {
			continue
		}
		t, err := s.repo.GetTagByName(ctx, name)
		if errors.Is(err, model.ErrNotFound) {
			// Create tag on the fly (Authors can create tags per TagService rule)
			t = &model.Tag{ID: uuid.New(), Name: name}
			if err := s.repo.CreateTag(ctx, t); err != nil {
				return nil, err
			}
			// Re-fetch to get the actual id (ON CONFLICT DO NOTHING may have not inserted)
			if t, err = s.repo.GetTagByName(ctx, name); err != nil {
				return nil, err
			}
		} else if err != nil {
			return nil, err
		}
		ids = append(ids, t.ID)
	}
	return ids, nil
}

func (s *CatalogService) createVersionSnapshot(ctx context.Context, res *model.Resource, tagNames []string, changedBy uuid.UUID) error {
	snap := model.ResourceSnapshot{
		Title:       res.Title,
		Description: res.Description,
		ContentBody: res.ContentBody,
		CategoryID:  res.CategoryID,
		Tags:        tagNames,
	}
	snapJSON, err := jsonMarshal(snap)
	if err != nil {
		return err
	}
	v := &model.ResourceVersion{
		ID:            uuid.New(),
		ResourceID:    res.ID,
		VersionNumber: res.CurrentVersionNumber,
		DataSnapshot:  snapJSON,
		ChangedBy:     changedBy,
	}
	return s.repo.CreateVersion(ctx, v)
}

// RateLimitError is returned when the rate limit is exceeded.
type RateLimitError struct {
	RetryAfter int // seconds
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("rate limit exceeded; retry after %d seconds", e.RetryAfter)
}

func containsRole(roles []string, target string) bool {
	for _, r := range roles {
		if r == target {
			return true
		}
	}
	return false
}
