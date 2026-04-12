package cataloghandler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/eduexchange/eduexchange/internal/middleware"
	"github.com/eduexchange/eduexchange/internal/model"
	catalogrepo "github.com/eduexchange/eduexchange/internal/repository/catalog"
	catalogservice "github.com/eduexchange/eduexchange/internal/service/catalog"
	catalogpages "github.com/eduexchange/eduexchange/internal/templ/pages/catalog"
)

// Handler serves all catalog routes.
type Handler struct {
	catalogSvc *catalogservice.CatalogService
	categorySvc *catalogservice.CategoryService
	tagSvc     *catalogservice.TagService
	importSvc  *catalogservice.BulkImportService
	exportSvc  *catalogservice.BulkExportService
}

func New(
	catalogSvc *catalogservice.CatalogService,
	categorySvc *catalogservice.CategoryService,
	tagSvc *catalogservice.TagService,
	importSvc *catalogservice.BulkImportService,
	exportSvc *catalogservice.BulkExportService,
) *Handler {
	return &Handler{
		catalogSvc:  catalogSvc,
		categorySvc: categorySvc,
		tagSvc:      tagSvc,
		importSvc:   importSvc,
		exportSvc:   exportSvc,
	}
}

// ─── Resources ────────────────────────────────────────────────────────────────

func (h *Handler) GetResourceList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	filter := catalogrepo.ResourceFilter{
		Page:     page,
		PageSize: pageSize,
		Status:   c.Query("status"),
		Tag:      c.Query("tag"),
		Search:   c.Query("search"),
		Sort:     c.DefaultQuery("sort", "created_at"),
		Order:    c.DefaultQuery("order", "desc"),
	}
	if catStr := c.Query("category_id"); catStr != "" {
		if id, err := uuid.Parse(catStr); err == nil {
			filter.CategoryID = &id
		}
	}

	authUser := middleware.GetAuthUser(c)

	// Non-admin/non-reviewer: only show own + published resources.
	if authUser != nil && !hasRole(authUser, "ADMIN", "REVIEWER") {
		filter.Status = "PUBLISHED"
	}

	resources, total, err := h.catalogSvc.ListResources(c.Request.Context(), filter)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	categories, _ := h.categorySvc.ListCategories(c.Request.Context())
	tags, _ := h.tagSvc.ListTags(c.Request.Context())

	c.Status(http.StatusOK)
	_ = catalogpages.ResourceListPage(catalogpages.ResourceListData{
		Resources:  resources,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		Filter:     filter,
		Categories: categories,
		Tags:       tags,
		AuthUser:   authUser,
	}).Render(c.Request.Context(), c.Writer)
}

func (h *Handler) GetResourceDetail(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}

	res, err := h.catalogSvc.GetResource(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			c.Status(http.StatusNotFound)
			return
		}
		c.Status(http.StatusInternalServerError)
		return
	}

	authUser := middleware.GetAuthUser(c)

	// Non-published resources are only visible to the author or admin/reviewer.
	if res.Status != model.ResourceStatusPublished {
		if authUser == nil || (res.AuthorID != authUser.ID && !hasRole(authUser, "ADMIN", "REVIEWER")) {
			c.Status(http.StatusNotFound)
			return
		}
	}

	versions, _ := h.catalogSvc.ListVersions(c.Request.Context(), id)
	reviews, _ := h.catalogSvc.ListReviews(c.Request.Context(), id)

	c.Status(http.StatusOK)
	_ = catalogpages.ResourceDetailPage(catalogpages.ResourceDetailData{
		Resource: res,
		Versions: versions,
		Reviews:  reviews,
		AuthUser: authUser,
	}).Render(c.Request.Context(), c.Writer)
}

func (h *Handler) GetNewResource(c *gin.Context) {
	categories, _ := h.categorySvc.ListCategories(c.Request.Context())
	tags, _ := h.tagSvc.ListTags(c.Request.Context())
	authUser := middleware.GetAuthUser(c)

	c.Status(http.StatusOK)
	_ = catalogpages.ResourceFormPage(catalogpages.ResourceFormData{
		Categories: categories,
		Tags:       tags,
		AuthUser:   authUser,
	}).Render(c.Request.Context(), c.Writer)
}

func (h *Handler) PostCreateResource(c *gin.Context) {
	authUser := middleware.GetAuthUser(c)
	if authUser == nil {
		c.Status(http.StatusUnauthorized)
		return
	}

	input := parseResourceInput(c)

	res, err := h.catalogSvc.CreateDraft(c.Request.Context(), authUser.ID, input)
	if err != nil {
		h.renderFormError(c, err, catalogpages.ResourceFormData{Input: input})
		return
	}

	htmxRedirect(c, fmt.Sprintf("/resources/%s", res.ID))
}

func (h *Handler) GetEditResource(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}

	res, err := h.catalogSvc.GetResource(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			c.Status(http.StatusNotFound)
			return
		}
		c.Status(http.StatusInternalServerError)
		return
	}

	authUser := middleware.GetAuthUser(c)
	if authUser == nil || (res.AuthorID != authUser.ID && !hasRole(authUser, "ADMIN")) {
		c.Status(http.StatusForbidden)
		return
	}

	categories, _ := h.categorySvc.ListCategories(c.Request.Context())
	tags, _ := h.tagSvc.ListTags(c.Request.Context())

	c.Status(http.StatusOK)
	_ = catalogpages.ResourceFormPage(catalogpages.ResourceFormData{
		Resource:   res,
		Categories: categories,
		Tags:       tags,
		AuthUser:   authUser,
	}).Render(c.Request.Context(), c.Writer)
}

func (h *Handler) PutUpdateResource(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}

	authUser := middleware.GetAuthUser(c)
	if authUser == nil {
		c.Status(http.StatusUnauthorized)
		return
	}

	version, _ := strconv.Atoi(c.PostForm("version"))
	input := parseResourceInput(c)

	res, err := h.catalogSvc.GetResource(c.Request.Context(), id)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}

	var updated *model.Resource
	if res.Status == model.ResourceStatusPublished {
		updated, err = h.catalogSvc.EditPublished(c.Request.Context(), id, authUser.ID, input, version)
	} else {
		updated, err = h.catalogSvc.UpdateDraft(c.Request.Context(), id, authUser.ID, input, version)
	}
	if err != nil {
		h.renderFormError(c, err, catalogpages.ResourceFormData{Resource: res, Input: input})
		return
	}

	htmxRedirect(c, fmt.Sprintf("/resources/%s", updated.ID))
}

func (h *Handler) DeleteResource(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	authUser := middleware.GetAuthUser(c)
	if authUser == nil {
		c.Status(http.StatusUnauthorized)
		return
	}
	if err := h.catalogSvc.DeleteResource(c.Request.Context(), id, authUser.ID, authUser.Roles); err != nil {
		renderError(c, err)
		return
	}
	htmxRedirect(c, "/resources")
}

// ─── Workflow actions ─────────────────────────────────────────────────────────

func (h *Handler) PostSubmitForReview(c *gin.Context) {
	id, _ := parseUUID(c, "id")
	authUser := middleware.GetAuthUser(c)
	version, _ := strconv.Atoi(c.PostForm("version"))
	res, err := h.catalogSvc.SubmitForReview(c.Request.Context(), id, authUser.ID, version)
	if err != nil {
		renderError(c, err)
		return
	}
	htmxRedirect(c, fmt.Sprintf("/resources/%s", res.ID))
}

func (h *Handler) PostApprove(c *gin.Context) {
	id, _ := parseUUID(c, "id")
	authUser := middleware.GetAuthUser(c)
	version, _ := strconv.Atoi(c.PostForm("version"))
	res, err := h.catalogSvc.Approve(c.Request.Context(), id, authUser.ID, version)
	if err != nil {
		renderError(c, err)
		return
	}
	htmxRedirect(c, fmt.Sprintf("/resources/%s", res.ID))
}

func (h *Handler) PostReject(c *gin.Context) {
	id, _ := parseUUID(c, "id")
	authUser := middleware.GetAuthUser(c)
	version, _ := strconv.Atoi(c.PostForm("version"))
	notes := c.PostForm("notes")
	res, err := h.catalogSvc.Reject(c.Request.Context(), id, authUser.ID, notes, version)
	if err != nil {
		renderError(c, err)
		return
	}
	htmxRedirect(c, fmt.Sprintf("/resources/%s", res.ID))
}

func (h *Handler) PostPublish(c *gin.Context) {
	id, _ := parseUUID(c, "id")
	authUser := middleware.GetAuthUser(c)
	version, _ := strconv.Atoi(c.PostForm("version"))
	res, err := h.catalogSvc.Publish(c.Request.Context(), id, authUser.ID, version)
	if err != nil {
		renderError(c, err)
		return
	}
	htmxRedirect(c, fmt.Sprintf("/resources/%s", res.ID))
}

func (h *Handler) PostRevise(c *gin.Context) {
	id, _ := parseUUID(c, "id")
	authUser := middleware.GetAuthUser(c)
	version, _ := strconv.Atoi(c.PostForm("version"))
	res, err := h.catalogSvc.ReviseRejected(c.Request.Context(), id, authUser.ID, version)
	if err != nil {
		renderError(c, err)
		return
	}
	htmxRedirect(c, fmt.Sprintf("/resources/%s", res.ID))
}

func (h *Handler) PostTakedown(c *gin.Context) {
	id, _ := parseUUID(c, "id")
	authUser := middleware.GetAuthUser(c)
	version, _ := strconv.Atoi(c.PostForm("version"))
	reason := c.PostForm("reason")
	res, err := h.catalogSvc.Takedown(c.Request.Context(), id, authUser.ID, reason, version)
	if err != nil {
		renderError(c, err)
		return
	}
	htmxRedirect(c, fmt.Sprintf("/resources/%s", res.ID))
}

func (h *Handler) PostRestore(c *gin.Context) {
	id, _ := parseUUID(c, "id")
	authUser := middleware.GetAuthUser(c)
	version, _ := strconv.Atoi(c.PostForm("version"))
	res, err := h.catalogSvc.Restore(c.Request.Context(), id, authUser.ID, version)
	if err != nil {
		renderError(c, err)
		return
	}
	htmxRedirect(c, fmt.Sprintf("/resources/%s", res.ID))
}

// ─── File upload/download ─────────────────────────────────────────────────────

func (h *Handler) PostUploadFile(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}

	authUser := middleware.GetAuthUser(c)
	if authUser == nil {
		c.Status(http.StatusUnauthorized)
		return
	}

	fh, err := c.FormFile("file")
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}

	f, err := fh.Open()
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	defer f.Close()

	rf, err := h.catalogSvc.UploadFile(c.Request.Context(), id, authUser.ID, authUser.Roles, f, fh.Filename, fh.Size)
	if err != nil {
		renderError(c, err)
		return
	}

	if c.GetHeader("HX-Request") == "true" {
		c.Status(http.StatusOK)
		_ = catalogpages.FileRow(rf).Render(c.Request.Context(), c.Writer)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"file": rf})
}

func (h *Handler) GetDownloadFile(c *gin.Context) {
	resourceID, err := parseUUID(c, "id")
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	fileID, err := parseUUID(c, "fileID")
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}

	rf, err := h.catalogSvc.GetFile(c.Request.Context(), resourceID, fileID)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			c.Status(http.StatusNotFound)
			return
		}
		c.Status(http.StatusInternalServerError)
		return
	}

	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, rf.OriginalName))
	c.Header("Content-Type", rf.MIMEType)
	c.File(rf.StoredPath)
}

func (h *Handler) DeleteFile(c *gin.Context) {
	resourceID, err := parseUUID(c, "id")
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	fileID, err := parseUUID(c, "fileID")
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	authUser := middleware.GetAuthUser(c)
	if authUser == nil {
		c.Status(http.StatusUnauthorized)
		return
	}
	if err := h.catalogSvc.DeleteFile(c.Request.Context(), resourceID, fileID, authUser.ID, authUser.Roles); err != nil {
		renderError(c, err)
		return
	}
	c.Status(http.StatusOK)
}

// ─── Review queue ─────────────────────────────────────────────────────────────

func (h *Handler) GetReviewQueue(c *gin.Context) {
	filter := catalogrepo.ResourceFilter{
		Status:   "PENDING_REVIEW",
		Page:     1,
		PageSize: 50,
	}
	resources, total, err := h.catalogSvc.ListResources(c.Request.Context(), filter)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	authUser := middleware.GetAuthUser(c)
	c.Status(http.StatusOK)
	_ = catalogpages.ReviewQueuePage(catalogpages.ReviewQueueData{
		Resources: resources,
		Total:     total,
		AuthUser:  authUser,
	}).Render(c.Request.Context(), c.Writer)
}

// ─── Categories (Admin) ───────────────────────────────────────────────────────

func (h *Handler) GetCategories(c *gin.Context) {
	cats, err := h.categorySvc.ListCategories(c.Request.Context())
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	authUser := middleware.GetAuthUser(c)
	c.Status(http.StatusOK)
	_ = catalogpages.CategoryListPage(catalogpages.CategoryListData{
		Categories: cats,
		AuthUser:   authUser,
	}).Render(c.Request.Context(), c.Writer)
}

func (h *Handler) PostCreateCategory(c *gin.Context) {
	authUser := middleware.GetAuthUser(c)
	if authUser == nil {
		c.Status(http.StatusUnauthorized)
		return
	}

	name := strings.TrimSpace(c.PostForm("name"))
	sortOrder, _ := strconv.Atoi(c.PostForm("sort_order"))
	var parentID *uuid.UUID
	if p := c.PostForm("parent_id"); p != "" {
		if pid, err := uuid.Parse(p); err == nil {
			parentID = &pid
		}
	}

	_, err := h.categorySvc.CreateCategory(c.Request.Context(), authUser.ID, name, parentID, sortOrder)
	if err != nil {
		renderError(c, err)
		return
	}
	htmxRedirect(c, "/categories")
}

func (h *Handler) PutUpdateCategory(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	authUser := middleware.GetAuthUser(c)
	if authUser == nil {
		c.Status(http.StatusUnauthorized)
		return
	}
	name := strings.TrimSpace(c.PostForm("name"))
	sortOrder, _ := strconv.Atoi(c.PostForm("sort_order"))
	if _, err := h.categorySvc.UpdateCategory(c.Request.Context(), authUser.ID, id, name, sortOrder); err != nil {
		renderError(c, err)
		return
	}
	htmxRedirect(c, "/categories")
}

func (h *Handler) DeleteCategory(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	authUser := middleware.GetAuthUser(c)
	if authUser == nil {
		c.Status(http.StatusUnauthorized)
		return
	}
	if err := h.categorySvc.DeleteCategory(c.Request.Context(), authUser.ID, id); err != nil {
		renderError(c, err)
		return
	}
	c.Status(http.StatusOK)
}

// ─── Tags ─────────────────────────────────────────────────────────────────────

func (h *Handler) GetTags(c *gin.Context) {
	tags, err := h.tagSvc.ListTags(c.Request.Context())
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusOK, gin.H{"tags": tags})
}

func (h *Handler) PostCreateTag(c *gin.Context) {
	authUser := middleware.GetAuthUser(c)
	if authUser == nil {
		c.Status(http.StatusUnauthorized)
		return
	}
	name := strings.TrimSpace(c.PostForm("name"))
	t, err := h.tagSvc.CreateTag(c.Request.Context(), authUser.ID, name)
	if err != nil {
		renderError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"tag": t})
}

func (h *Handler) DeleteTag(c *gin.Context) {
	id, err := parseUUID(c, "id")
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	authUser := middleware.GetAuthUser(c)
	if authUser == nil {
		c.Status(http.StatusUnauthorized)
		return
	}
	if err := h.tagSvc.DeleteTag(c.Request.Context(), authUser.ID, id); err != nil {
		renderError(c, err)
		return
	}
	c.Status(http.StatusOK)
}

// ─── Bulk Import (Admin) ──────────────────────────────────────────────────────

func (h *Handler) GetImportWizard(c *gin.Context) {
	authUser := middleware.GetAuthUser(c)
	c.Status(http.StatusOK)
	_ = catalogpages.ImportWizardPage(catalogpages.ImportWizardData{
		Step:     1,
		AuthUser: authUser,
	}).Render(c.Request.Context(), c.Writer)
}

func (h *Handler) PostImportUpload(c *gin.Context) {
	authUser := middleware.GetAuthUser(c)
	if authUser == nil {
		c.Status(http.StatusUnauthorized)
		return
	}

	fh, err := c.FormFile("file")
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}

	f, err := fh.Open()
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	defer f.Close()

	job, err := h.importSvc.Upload(c.Request.Context(), authUser.ID, f, fh.Filename, fh.Size)
	if err != nil {
		renderError(c, err)
		return
	}

	htmxRedirect(c, fmt.Sprintf("/import/%s/preview", job.ID))
}

func (h *Handler) GetImportPreview(c *gin.Context) {
	jobID, err := parseUUID(c, "jobID")
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	job, err := h.importSvc.GetJob(c.Request.Context(), jobID)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			c.Status(http.StatusNotFound)
			return
		}
		c.Status(http.StatusInternalServerError)
		return
	}
	authUser := middleware.GetAuthUser(c)
	c.Status(http.StatusOK)
	_ = catalogpages.ImportWizardPage(catalogpages.ImportWizardData{
		Step:     2,
		Job:      job,
		AuthUser: authUser,
	}).Render(c.Request.Context(), c.Writer)
}

func (h *Handler) PostImportConfirm(c *gin.Context) {
	jobID, err := parseUUID(c, "jobID")
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	authUser := middleware.GetAuthUser(c)
	if authUser == nil {
		c.Status(http.StatusUnauthorized)
		return
	}
	job, err := h.importSvc.Confirm(c.Request.Context(), authUser.ID, jobID)
	if err != nil {
		renderError(c, err)
		return
	}
	htmxRedirect(c, fmt.Sprintf("/import/%s/done", job.ID))
}

func (h *Handler) GetImportDone(c *gin.Context) {
	jobID, err := parseUUID(c, "jobID")
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	job, err := h.importSvc.GetJob(c.Request.Context(), jobID)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	authUser := middleware.GetAuthUser(c)
	c.Status(http.StatusOK)
	_ = catalogpages.ImportWizardPage(catalogpages.ImportWizardData{
		Step:     3,
		Job:      job,
		AuthUser: authUser,
	}).Render(c.Request.Context(), c.Writer)
}

// ─── Bulk Export (Admin) ──────────────────────────────────────────────────────

func (h *Handler) GetExportPage(c *gin.Context) {
	authUser := middleware.GetAuthUser(c)
	c.Status(http.StatusOK)
	_ = catalogpages.ExportPage(catalogpages.ExportData{
		AuthUser: authUser,
	}).Render(c.Request.Context(), c.Writer)
}

func (h *Handler) PostGenerateExport(c *gin.Context) {
	authUser := middleware.GetAuthUser(c)
	if authUser == nil {
		c.Status(http.StatusUnauthorized)
		return
	}
	result, err := h.exportSvc.Export(c.Request.Context(), authUser.ID)
	if err != nil {
		renderError(c, err)
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, result.Filename))
	c.Header("Content-Type", "text/csv")
	c.File(result.FilePath)
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func parseUUID(c *gin.Context, param string) (uuid.UUID, error) {
	return uuid.Parse(c.Param(param))
}

func parseResourceInput(c *gin.Context) catalogservice.ResourceInput {
	var catID *uuid.UUID
	if s := c.PostForm("category_id"); s != "" {
		if id, err := uuid.Parse(s); err == nil {
			catID = &id
		}
	}

	var tagNames []string
	for _, t := range strings.Split(c.PostForm("tags"), ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			tagNames = append(tagNames, t)
		}
	}

	return catalogservice.ResourceInput{
		Title:       strings.TrimSpace(c.PostForm("title")),
		Description: c.PostForm("description"),
		ContentBody: c.PostForm("content_body"),
		CategoryID:  catID,
		TagNames:    tagNames,
	}
}

func htmxRedirect(c *gin.Context, url string) {
	if c.GetHeader("HX-Request") == "true" {
		c.Header("HX-Redirect", url)
		c.Status(http.StatusOK)
		return
	}
	c.Redirect(http.StatusSeeOther, url)
}

func renderError(c *gin.Context, err error) {
	var ve *model.ValidationErrors
	if errors.As(err, &ve) {
		if c.GetHeader("HX-Request") == "true" || c.GetHeader("Accept") == "application/json" {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"errors": ve.Errors})
			return
		}
		c.Status(http.StatusUnprocessableEntity)
		return
	}
	if errors.Is(err, model.ErrForbidden) {
		c.Status(http.StatusForbidden)
		return
	}
	if errors.Is(err, model.ErrNotFound) {
		c.Status(http.StatusNotFound)
		return
	}
	if errors.Is(err, model.ErrConflict) {
		c.Status(http.StatusConflict)
		return
	}
	var rle *catalogservice.RateLimitError
	if errors.As(err, &rle) {
		c.Header("Retry-After", strconv.Itoa(rle.RetryAfter))
		c.Status(http.StatusTooManyRequests)
		return
	}
	c.Status(http.StatusInternalServerError)
}

func (h *Handler) renderFormError(c *gin.Context, err error, data catalogpages.ResourceFormData) {
	var ve *model.ValidationErrors
	if errors.As(err, &ve) {
		data.Errors = ve
		categories, _ := h.categorySvc.ListCategories(c.Request.Context())
		tags, _ := h.tagSvc.ListTags(c.Request.Context())
		data.Categories = categories
		data.Tags = tags
		status := http.StatusUnprocessableEntity
		if c.GetHeader("HX-Request") == "true" {
			status = http.StatusOK
		}
		c.Status(status)
		_ = catalogpages.ResourceFormPage(data).Render(c.Request.Context(), c.Writer)
		return
	}
	renderError(c, err)
}

func hasRole(u *middleware.AuthUser, roles ...string) bool {
	for _, required := range roles {
		for _, r := range u.Roles {
			if r == required {
				return true
			}
		}
	}
	return false
}
