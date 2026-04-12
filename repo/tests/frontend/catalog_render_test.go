package frontend_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/eduexchange/eduexchange/internal/middleware"
	"github.com/eduexchange/eduexchange/internal/model"
	catalogrepo "github.com/eduexchange/eduexchange/internal/repository/catalog"
	catalogservice "github.com/eduexchange/eduexchange/internal/service/catalog"
	catalogpages "github.com/eduexchange/eduexchange/internal/templ/pages/catalog"
)

var (
	testAuthorID   = uuid.New()
	testCategoryID = uuid.New()
	testResourceID = uuid.New()
	testFileID     = uuid.New()

	sampleResource = model.Resource{
		ID:                   testResourceID,
		Title:                "Introduction to Go",
		Description:          "A beginner's guide to Go.",
		ContentBody:          "Go is a statically typed, compiled programming language.",
		AuthorID:             testAuthorID,
		AuthorName:           "Alice",
		CategoryID:           &testCategoryID,
		CategoryName:         "Programming",
		Status:               model.ResourceStatusDraft,
		CurrentVersionNumber: 1,
		Version:              1,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
		Tags: []model.Tag{
			{ID: uuid.New(), Name: "golang"},
			{ID: uuid.New(), Name: "beginner"},
		},
	}

	sampleCategories = []model.Category{
		{ID: testCategoryID, Name: "Programming", Level: 1},
		{ID: uuid.New(), Name: "Science", Level: 1},
	}

	sampleTags = []model.Tag{
		{ID: uuid.New(), Name: "golang"},
		{ID: uuid.New(), Name: "beginner"},
		{ID: uuid.New(), Name: "advanced"},
	}

	adminUser = &middleware.AuthUser{
		ID:       testAuthorID,
		Username: "admin",
		Roles:    []string{"ADMIN"},
	}
	authorUser = &middleware.AuthUser{
		ID:       testAuthorID,
		Username: "alice",
		Roles:    []string{"AUTHOR"},
	}
	reviewerUser = &middleware.AuthUser{
		ID:       uuid.New(),
		Username: "reviewer",
		Roles:    []string{"REVIEWER"},
	}
)

// ─── ResourceListPage ─────────────────────────────────────────────────────────

func TestResourceListPage_RendersResources(t *testing.T) {
	resources := []model.Resource{sampleResource}
	html := render(t, catalogpages.ResourceListPage(catalogpages.ResourceListData{
		Resources:  resources,
		Total:      1,
		Page:       1,
		PageSize:   20,
		Filter:     catalogrepo.ResourceFilter{},
		Categories: sampleCategories,
		Tags:       sampleTags,
		AuthUser:   adminUser,
	}))

	assert.Contains(t, html, "Introduction to Go")
	assert.Contains(t, html, "DRAFT")
	assert.Contains(t, html, "Alice")
	assert.Contains(t, html, "+ New Resource")
}

func TestResourceListPage_EmptyState(t *testing.T) {
	html := render(t, catalogpages.ResourceListPage(catalogpages.ResourceListData{
		Resources: nil,
		Total:     0,
		Page:      1,
		PageSize:  20,
		AuthUser:  adminUser,
	}))

	assert.Contains(t, html, "No resources found")
}

func TestResourceListPage_FilterFormPresent(t *testing.T) {
	html := render(t, catalogpages.ResourceListPage(catalogpages.ResourceListData{
		Resources: []model.Resource{},
		Page:      1, PageSize: 20, AuthUser: adminUser,
	}))

	assert.Contains(t, html, `name="search"`)
	assert.Contains(t, html, `name="status"`)
}

func TestResourceListPage_Pagination_ShowsNavLinks(t *testing.T) {
	html := render(t, catalogpages.ResourceListPage(catalogpages.ResourceListData{
		Resources: make([]model.Resource, 20),
		Total:     45,
		Page:      2,
		PageSize:  20,
		AuthUser:  adminUser,
	}))

	assert.Contains(t, html, "Previous")
	assert.Contains(t, html, "Next")
}

func TestResourceListPage_NoNewButtonWhenLoggedOut(t *testing.T) {
	html := render(t, catalogpages.ResourceListPage(catalogpages.ResourceListData{
		Resources: []model.Resource{sampleResource},
		Total:     1, Page: 1, PageSize: 20,
		AuthUser: nil, // not logged in
	}))

	assert.NotContains(t, html, "+ New Resource")
}

// ─── ResourceDetailPage ───────────────────────────────────────────────────────

func TestResourceDetailPage_ShowsContent(t *testing.T) {
	r := sampleResource
	html := render(t, catalogpages.ResourceDetailPage(catalogpages.ResourceDetailData{
		Resource: &r,
		AuthUser: authorUser,
	}))

	assert.Contains(t, html, "Introduction to Go")
	// Apostrophe is HTML-escaped by templ
	assert.Contains(t, html, "beginner")
	assert.Contains(t, html, "golang")
	assert.Contains(t, html, "beginner")
	assert.Contains(t, html, "DRAFT")
}

func TestResourceDetailPage_OwnerDraft_ShowsSubmitButton(t *testing.T) {
	r := sampleResource
	r.AuthorID = authorUser.ID
	html := render(t, catalogpages.ResourceDetailPage(catalogpages.ResourceDetailData{
		Resource: &r,
		AuthUser: authorUser,
	}))

	assert.Contains(t, html, "Submit for Review")
}

func TestResourceDetailPage_ReviewerPendingReview_ShowsApproveReject(t *testing.T) {
	r := sampleResource
	r.Status = model.ResourceStatusPendingReview
	html := render(t, catalogpages.ResourceDetailPage(catalogpages.ResourceDetailData{
		Resource: &r,
		AuthUser: reviewerUser,
	}))

	assert.Contains(t, html, "Approve")
	assert.Contains(t, html, "Reject")
}

func TestResourceDetailPage_AdminApproved_ShowsPublishButton(t *testing.T) {
	r := sampleResource
	r.Status = model.ResourceStatusApproved
	html := render(t, catalogpages.ResourceDetailPage(catalogpages.ResourceDetailData{
		Resource: &r,
		AuthUser: adminUser,
	}))

	assert.Contains(t, html, "Publish")
}

func TestResourceDetailPage_AdminPublished_ShowsTakedownButton(t *testing.T) {
	r := sampleResource
	r.Status = model.ResourceStatusPublished
	html := render(t, catalogpages.ResourceDetailPage(catalogpages.ResourceDetailData{
		Resource: &r,
		AuthUser: adminUser,
	}))

	assert.Contains(t, html, "Take Down")
}

func TestResourceDetailPage_AdminTakenDown_ShowsRestoreButton(t *testing.T) {
	r := sampleResource
	r.Status = model.ResourceStatusTakenDown
	html := render(t, catalogpages.ResourceDetailPage(catalogpages.ResourceDetailData{
		Resource: &r,
		AuthUser: adminUser,
	}))

	assert.Contains(t, html, "Restore")
}

func TestResourceDetailPage_ShowsFileUploadFormForOwner(t *testing.T) {
	r := sampleResource
	r.AuthorID = authorUser.ID
	html := render(t, catalogpages.ResourceDetailPage(catalogpages.ResourceDetailData{
		Resource: &r,
		AuthUser: authorUser,
	}))

	assert.Contains(t, html, `type="file"`)
	assert.Contains(t, html, "Upload")
}

func TestResourceDetailPage_ShowsReviews(t *testing.T) {
	r := sampleResource
	reviews := []model.ResourceReview{
		{
			ID:           uuid.New(),
			ResourceID:   r.ID,
			ReviewerID:   reviewerUser.ID,
			ReviewerName: "reviewer",
			Action:       "REJECTED",
			Notes:        "Needs more detail.",
		},
	}
	html := render(t, catalogpages.ResourceDetailPage(catalogpages.ResourceDetailData{
		Resource: &r,
		Reviews:  reviews,
		AuthUser: adminUser,
	}))

	assert.Contains(t, html, "reviewer")
	assert.Contains(t, html, "REJECTED")
	assert.Contains(t, html, "Needs more detail.")
}

// ─── ResourceFormPage ─────────────────────────────────────────────────────────

func TestResourceFormPage_NewResource(t *testing.T) {
	html := render(t, catalogpages.ResourceFormPage(catalogpages.ResourceFormData{
		Categories: sampleCategories,
		Tags:       sampleTags,
		AuthUser:   authorUser,
	}))

	assert.Contains(t, html, "New Resource")
	assert.Contains(t, html, `name="title"`)
	assert.Contains(t, html, `name="description"`)
	assert.Contains(t, html, `name="content_body"`)
	assert.Contains(t, html, `name="category_id"`)
	assert.Contains(t, html, `name="tags"`)
	assert.Contains(t, html, "Create Draft")
}

func TestResourceFormPage_EditResource(t *testing.T) {
	r := sampleResource
	html := render(t, catalogpages.ResourceFormPage(catalogpages.ResourceFormData{
		Resource:   &r,
		Categories: sampleCategories,
		AuthUser:   authorUser,
	}))

	assert.Contains(t, html, "Edit Resource")
	assert.Contains(t, html, "Introduction to Go") // pre-filled
	assert.Contains(t, html, "Save Changes")
}

func TestResourceFormPage_ShowsValidationErrors(t *testing.T) {
	ve := model.NewValidationErrors()
	ve.Add("title", "Title is required.")
	ve.Add("description", "Description is too long.")

	html := render(t, catalogpages.ResourceFormPage(catalogpages.ResourceFormData{
		Errors:   ve,
		AuthUser: authorUser,
	}))

	assert.Contains(t, html, "Title is required.")
	assert.Contains(t, html, "Description is too long.")
}

func TestResourceFormPage_ShowsCategories(t *testing.T) {
	html := render(t, catalogpages.ResourceFormPage(catalogpages.ResourceFormData{
		Categories: sampleCategories,
		AuthUser:   authorUser,
	}))

	assert.Contains(t, html, "Programming")
	assert.Contains(t, html, "Science")
}

// ─── ReviewQueuePage ──────────────────────────────────────────────────────────

func TestReviewQueuePage_ShowsPendingResources(t *testing.T) {
	r := sampleResource
	r.Status = model.ResourceStatusPendingReview
	html := render(t, catalogpages.ReviewQueuePage(catalogpages.ReviewQueueData{
		Resources: []model.Resource{r},
		Total:     1,
		AuthUser:  reviewerUser,
	}))

	assert.Contains(t, html, "Introduction to Go")
	assert.Contains(t, html, "Approve")
	assert.Contains(t, html, "Review")
}

func TestReviewQueuePage_EmptyQueue(t *testing.T) {
	html := render(t, catalogpages.ReviewQueuePage(catalogpages.ReviewQueueData{
		Resources: nil,
		Total:     0,
		AuthUser:  reviewerUser,
	}))

	assert.Contains(t, html, "Queue is empty")
}

// ─── ImportWizardPage ─────────────────────────────────────────────────────────

func TestImportWizardPage_Step1_ShowsUploadForm(t *testing.T) {
	html := render(t, catalogpages.ImportWizardPage(catalogpages.ImportWizardData{
		Step:     1,
		AuthUser: adminUser,
	}))

	assert.Contains(t, html, "Upload CSV File")
	assert.Contains(t, html, `type="file"`)
	assert.Contains(t, html, "Upload &amp; Validate")
}

func TestImportWizardPage_Step2_ShowsPreview(t *testing.T) {
	jobID := uuid.New()
	job := &model.BulkImportJob{
		ID:               jobID,
		OriginalFilename: "resources.csv",
		Status:           model.BulkImportStatusPreviewReady,
		TotalRows:        3,
		ValidRows:        2,
		InvalidRows:      1,
		Results: []model.ImportRowResult{
			{Row: 2, Status: "VALID", Title: "Go Basics"},
			{Row: 3, Status: "ERROR", Title: "", Errors: []model.ImportRowError{{Field: "title", Message: "Title is required."}}},
			{Row: 4, Status: "VALID", Title: "Go Advanced"},
		},
	}
	html := render(t, catalogpages.ImportWizardPage(catalogpages.ImportWizardData{
		Step:     2,
		Job:      job,
		AuthUser: adminUser,
	}))

	assert.Contains(t, html, "resources.csv")
	assert.Contains(t, html, "Go Basics")
	assert.Contains(t, html, "Title is required.")
	assert.Contains(t, html, "Import 2 Valid Rows")
}

func TestImportWizardPage_Step3_ShowsDone(t *testing.T) {
	jobID := uuid.New()
	now := time.Now()
	job := &model.BulkImportJob{
		ID:               jobID,
		OriginalFilename: "resources.csv",
		Status:           model.BulkImportStatusConfirmed,
		TotalRows:        3,
		ValidRows:        3,
		CompletedAt:      &now,
	}
	html := render(t, catalogpages.ImportWizardPage(catalogpages.ImportWizardData{
		Step:     3,
		Job:      job,
		AuthUser: adminUser,
	}))

	assert.Contains(t, html, "Import Complete")
	assert.Contains(t, html, "resources.csv")
	assert.Contains(t, html, "View Resources")
}

// ─── ExportPage ───────────────────────────────────────────────────────────────

func TestExportPage_Renders(t *testing.T) {
	html := render(t, catalogpages.ExportPage(catalogpages.ExportData{
		AuthUser: adminUser,
	}))

	assert.Contains(t, html, "Export Resource Metadata")
	assert.Contains(t, html, "Generate")
	assert.Contains(t, html, `action="/export/generate"`)
}

// ─── CategoryListPage ─────────────────────────────────────────────────────────

func TestCategoryListPage_ShowsCategories(t *testing.T) {
	html := render(t, catalogpages.CategoryListPage(catalogpages.CategoryListData{
		Categories: sampleCategories,
		AuthUser:   adminUser,
	}))

	assert.Contains(t, html, "Programming")
	assert.Contains(t, html, "Science")
	assert.Contains(t, html, "New Category")
}

func TestCategoryListPage_EmptyState(t *testing.T) {
	html := render(t, catalogpages.CategoryListPage(catalogpages.CategoryListData{
		Categories: nil,
		AuthUser:   adminUser,
	}))

	assert.Contains(t, html, "No categories")
}

// ─── FileRow ──────────────────────────────────────────────────────────────────

func TestFileRow_Renders(t *testing.T) {
	f := &model.ResourceFile{
		ID:           testFileID,
		ResourceID:   testResourceID,
		OriginalName: "document.pdf",
		MIMEType:     "application/pdf",
		SizeBytes:    102400,
	}
	html := render(t, catalogpages.FileRow(f))

	assert.Contains(t, html, "document.pdf")
	assert.Contains(t, html, "application/pdf")
	assert.Contains(t, html, "Remove")
}

// ─── ResourceInput (form data helper) ────────────────────────────────────────

func TestResourceFormData_InputOverridesResource(t *testing.T) {
	r := sampleResource
	data := catalogpages.ResourceFormData{
		Resource: &r,
		Input: catalogservice.ResourceInput{
			Title:       "Overridden Title",
			Description: "Overridden Desc",
		},
		AuthUser: authorUser,
	}
	html := render(t, catalogpages.ResourceFormPage(data))
	assert.Contains(t, html, "Overridden Title")
}
