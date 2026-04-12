package unit_test

import (
	"strings"
	"testing"

	"github.com/eduexchange/eduexchange/internal/model"
	catalogservice "github.com/eduexchange/eduexchange/internal/service/catalog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── State machine (PRD 9.2) ──────────────────────────────────────────────────

func TestValidResourceStatusTransition_AllowedPaths(t *testing.T) {
	cases := []struct {
		from model.ResourceStatus
		to   model.ResourceStatus
	}{
		{model.ResourceStatusDraft, model.ResourceStatusPendingReview},
		{model.ResourceStatusPendingReview, model.ResourceStatusApproved},
		{model.ResourceStatusPendingReview, model.ResourceStatusRejected},
		{model.ResourceStatusApproved, model.ResourceStatusPublished},
		{model.ResourceStatusRejected, model.ResourceStatusDraft},
		{model.ResourceStatusPublished, model.ResourceStatusTakenDown},
		{model.ResourceStatusPublished, model.ResourceStatusPendingReview},
		{model.ResourceStatusTakenDown, model.ResourceStatusPublished},
	}
	for _, c := range cases {
		err := catalogservice.ValidResourceStatusTransition(c.from, c.to)
		require.NoError(t, err, "expected %s → %s to be allowed", c.from, c.to)
	}
}

func TestValidResourceStatusTransition_ForbiddenPaths(t *testing.T) {
	cases := []struct {
		from model.ResourceStatus
		to   model.ResourceStatus
	}{
		{model.ResourceStatusDraft, model.ResourceStatusPublished},
		{model.ResourceStatusDraft, model.ResourceStatusApproved},
		{model.ResourceStatusDraft, model.ResourceStatusRejected},
		{model.ResourceStatusPendingReview, model.ResourceStatusPublished},
		{model.ResourceStatusPendingReview, model.ResourceStatusDraft},
		{model.ResourceStatusApproved, model.ResourceStatusDraft},
		{model.ResourceStatusApproved, model.ResourceStatusRejected},
		{model.ResourceStatusRejected, model.ResourceStatusPublished},
		{model.ResourceStatusPublished, model.ResourceStatusDraft},
		{model.ResourceStatusPublished, model.ResourceStatusApproved},
		{model.ResourceStatusTakenDown, model.ResourceStatusDraft},
		{model.ResourceStatusTakenDown, model.ResourceStatusRejected},
	}
	for _, c := range cases {
		err := catalogservice.ValidResourceStatusTransition(c.from, c.to)
		assert.Error(t, err, "expected %s → %s to be forbidden", c.from, c.to)
	}
}

func TestValidResourceStatusTransition_SameStatus_Forbidden(t *testing.T) {
	for _, s := range []model.ResourceStatus{
		model.ResourceStatusDraft,
		model.ResourceStatusPendingReview,
		model.ResourceStatusApproved,
		model.ResourceStatusPublished,
		model.ResourceStatusRejected,
		model.ResourceStatusTakenDown,
	} {
		err := catalogservice.ValidResourceStatusTransition(s, s)
		assert.Error(t, err, "expected %s → %s (no-op) to be forbidden", s, s)
	}
}

// ─── MIME type validation (CAT-06) ───────────────────────────────────────────

func TestValidateMIMEType_AllowedTypes(t *testing.T) {
	allowed := []string{
		"application/pdf",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/vnd.openxmlformats-officedocument.presentationml.presentation",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"image/jpeg",
		"image/png",
	}
	for _, mime := range allowed {
		err := catalogservice.ValidateMIMEType(mime)
		assert.NoError(t, err, "expected %q to be allowed", mime)
	}
}

func TestValidateMIMEType_ForbiddenTypes(t *testing.T) {
	forbidden := []string{
		"application/zip",
		"application/octet-stream",
		"text/plain",
		"text/html",
		"image/gif",
		"video/mp4",
		"application/x-executable",
		"",
	}
	for _, mime := range forbidden {
		err := catalogservice.ValidateMIMEType(mime)
		assert.Error(t, err, "expected %q to be forbidden", mime)
	}
}

// ─── ValidateResourceInput ────────────────────────────────────────────────────

func TestValidateResourceInput_EmptyTitle(t *testing.T) {
	ve := catalogservice.ValidateResourceInput("", "some description")
	assert.True(t, ve.HasErrors())
	assert.Equal(t, "title", ve.Errors[0].Field)
}

func TestValidateResourceInput_WhitespaceTitle(t *testing.T) {
	ve := catalogservice.ValidateResourceInput("   ", "")
	assert.True(t, ve.HasErrors())
}

func TestValidateResourceInput_TitleTooLong(t *testing.T) {
	long := strings.Repeat("a", 301)
	ve := catalogservice.ValidateResourceInput(long, "")
	assert.True(t, ve.HasErrors())
	assert.Equal(t, "title", ve.Errors[0].Field)
}

func TestValidateResourceInput_DescriptionTooLong(t *testing.T) {
	long := strings.Repeat("x", 5001)
	ve := catalogservice.ValidateResourceInput("Valid Title", long)
	assert.True(t, ve.HasErrors())
	assert.Equal(t, "description", ve.Errors[0].Field)
}

func TestValidateResourceInput_Valid(t *testing.T) {
	ve := catalogservice.ValidateResourceInput("A Great Resource", "Short description.")
	assert.False(t, ve.HasErrors())
}

func TestValidateResourceInput_ValidBoundaryTitle(t *testing.T) {
	title := strings.Repeat("a", 300)
	ve := catalogservice.ValidateResourceInput(title, "")
	assert.False(t, ve.HasErrors())
}

// ─── ValidateImportRow ───────────────────────────────────────────────────────

func TestValidateImportRow_EmptyTitle(t *testing.T) {
	row := model.ImportRowResult{Row: 2, Title: "", Category: "", Tags: ""}
	errs := catalogservice.ValidateImportRow(row, map[string]bool{})
	require.Len(t, errs, 1)
	assert.Equal(t, "title", errs[0].Field)
}

func TestValidateImportRow_TitleTooLong(t *testing.T) {
	row := model.ImportRowResult{Row: 2, Title: strings.Repeat("a", 301)}
	errs := catalogservice.ValidateImportRow(row, map[string]bool{})
	require.Len(t, errs, 1)
	assert.Equal(t, "title", errs[0].Field)
}

func TestValidateImportRow_UnknownCategory(t *testing.T) {
	row := model.ImportRowResult{Row: 2, Title: "Good Title", Category: "Ghost Category"}
	errs := catalogservice.ValidateImportRow(row, map[string]bool{"Real Category": true})
	require.Len(t, errs, 1)
	assert.Equal(t, "category", errs[0].Field)
}

func TestValidateImportRow_KnownCategory_OK(t *testing.T) {
	row := model.ImportRowResult{Row: 2, Title: "Good Title", Category: "Science"}
	errs := catalogservice.ValidateImportRow(row, map[string]bool{"Science": true})
	assert.Empty(t, errs)
}

func TestValidateImportRow_TagTooLong(t *testing.T) {
	longTag := strings.Repeat("t", 101)
	row := model.ImportRowResult{Row: 2, Title: "Good Title", Tags: "math, " + longTag}
	errs := catalogservice.ValidateImportRow(row, map[string]bool{})
	require.Len(t, errs, 1)
	assert.Equal(t, "tags", errs[0].Field)
}

func TestValidateImportRow_EmptyCategory_OK(t *testing.T) {
	row := model.ImportRowResult{Row: 2, Title: "Valid", Category: "", Tags: ""}
	errs := catalogservice.ValidateImportRow(row, map[string]bool{})
	assert.Empty(t, errs)
}

func TestValidateImportRow_MultipleErrors(t *testing.T) {
	row := model.ImportRowResult{Row: 2, Title: "", Category: "Missing"}
	errs := catalogservice.ValidateImportRow(row, map[string]bool{})
	// Title required + category not found
	assert.Len(t, errs, 2)
}

func TestValidateImportRow_ValidFull(t *testing.T) {
	row := model.ImportRowResult{Row: 2, Title: "Full Resource", Category: "Math", Tags: "algebra, geometry"}
	errs := catalogservice.ValidateImportRow(row, map[string]bool{"Math": true})
	assert.Empty(t, errs)
}
