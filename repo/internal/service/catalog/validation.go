package catalogservice

import (
	"fmt"
	"strings"

	"github.com/eduexchange/eduexchange/internal/model"
)

// ValidResourceStatusTransition checks PRD 9.2 state machine.
func ValidResourceStatusTransition(from, to model.ResourceStatus) error {
	allowed := map[model.ResourceStatus][]model.ResourceStatus{
		model.ResourceStatusDraft:         {model.ResourceStatusPendingReview},
		model.ResourceStatusPendingReview: {model.ResourceStatusApproved, model.ResourceStatusRejected},
		model.ResourceStatusApproved:      {model.ResourceStatusPublished},
		model.ResourceStatusRejected:      {model.ResourceStatusDraft},
		model.ResourceStatusPublished:     {model.ResourceStatusTakenDown, model.ResourceStatusPendingReview},
		model.ResourceStatusTakenDown:     {model.ResourceStatusPublished},
	}

	for _, ok := range allowed[from] {
		if ok == to {
			return nil
		}
	}
	return fmt.Errorf("invalid transition: %s → %s", from, to)
}

// ValidateMIMEType checks if the MIME type is in the allowed set (CAT-06).
func ValidateMIMEType(mimeType string) error {
	if _, ok := model.AllowedMIMETypes[mimeType]; ok {
		return nil
	}
	allowed := make([]string, 0, len(model.AllowedMIMETypes))
	for m := range model.AllowedMIMETypes {
		allowed = append(allowed, m)
	}
	return fmt.Errorf("file type %q is not allowed; permitted types: %s", mimeType, strings.Join(allowed, ", "))
}

// ValidateResourceInput validates title, description etc. for create/update.
func ValidateResourceInput(title, description string) *model.ValidationErrors {
	ve := model.NewValidationErrors()
	if strings.TrimSpace(title) == "" {
		ve.Add("title", "Title is required.")
	} else if len(title) > 300 {
		ve.Add("title", "Title must be 1–300 characters.")
	}
	if len(description) > 5000 {
		ve.Add("description", "Description must be at most 5000 characters.")
	}
	return ve
}

// ValidateImportRow validates a single import row.
// Returns a slice of ImportRowError (empty = valid).
func ValidateImportRow(row model.ImportRowResult, knownCategories map[string]bool) []model.ImportRowError {
	var errs []model.ImportRowError

	if strings.TrimSpace(row.Title) == "" {
		errs = append(errs, model.ImportRowError{Field: "title", Message: "Title is required."})
	} else if len(row.Title) > 300 {
		errs = append(errs, model.ImportRowError{Field: "title", Message: "Title must be 1–300 characters."})
	}

	if row.Category != "" && !knownCategories[strings.TrimSpace(row.Category)] {
		errs = append(errs, model.ImportRowError{Field: "category", Message: fmt.Sprintf("Category %q does not exist.", row.Category)})
	}

	// Tags: comma-separated, each max 100 chars
	if row.Tags != "" {
		for _, tag := range strings.Split(row.Tags, ",") {
			tag = strings.TrimSpace(tag)
			if len(tag) > 100 {
				errs = append(errs, model.ImportRowError{Field: "tags", Message: fmt.Sprintf("Tag %q exceeds 100 characters.", tag)})
			}
		}
	}

	return errs
}
