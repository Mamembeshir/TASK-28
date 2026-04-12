package catalogservice

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eduexchange/eduexchange/internal/audit"
	"github.com/eduexchange/eduexchange/internal/model"
	catalogrepo "github.com/eduexchange/eduexchange/internal/repository/catalog"
	"github.com/gabriel-vasile/mimetype"
	"github.com/google/uuid"
)

// BulkImportService handles CSV bulk import (CAT-07).
// Two-phase flow: Upload → Preview → Confirm.
type BulkImportService struct {
	repo      catalogrepo.CatalogRepository
	auditSvc  *audit.Service
	uploadDir string // e.g. "data/imports"
}

func NewBulkImportService(repo catalogrepo.CatalogRepository, auditSvc *audit.Service, uploadDir string) *BulkImportService {
	return &BulkImportService{repo: repo, auditSvc: auditSvc, uploadDir: uploadDir}
}

// Upload saves the file, parses and validates rows, stores job with PREVIEW_READY status.
// Returns the job so the caller can render the preview.
func (s *BulkImportService) Upload(ctx context.Context, actorID uuid.UUID, reader io.Reader, originalFilename string, fileSize int64) (*model.BulkImportJob, error) {
	if fileSize > model.MaxImportFileSize {
		ve := model.NewValidationErrors()
		ve.Add("file", fmt.Sprintf("File exceeds maximum size of %d MB.", model.MaxImportFileSize/1024/1024))
		return nil, ve
	}

	// Read entire file into memory for MIME detection + parsing (≤25 MB).
	data, err := io.ReadAll(io.LimitReader(reader, model.MaxImportFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("reading import file: %w", err)
	}
	if int64(len(data)) > model.MaxImportFileSize {
		ve := model.NewValidationErrors()
		ve.Add("file", fmt.Sprintf("File exceeds maximum size of %d MB.", model.MaxImportFileSize/1024/1024))
		return nil, ve
	}

	// Validate MIME type by content.
	mt := mimetype.Detect(data)
	switch mt.String() {
	case "text/plain; charset=utf-8", "text/csv; charset=utf-8",
		"text/plain", "text/csv", "application/csv":
		// ok
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"application/vnd.ms-excel":
		ve := model.NewValidationErrors()
		ve.Add("file", "XLSX files are not yet supported for bulk import. Please convert to CSV.")
		return nil, ve
	default:
		ve := model.NewValidationErrors()
		ve.Add("file", "Only CSV files are accepted for bulk import.")
		return nil, ve
	}

	// Parse CSV.
	rows, err := parseCSV(data)
	if err != nil {
		ve := model.NewValidationErrors()
		ve.Add("file", fmt.Sprintf("Invalid CSV: %s", err))
		return nil, ve
	}

	if len(rows) == 0 {
		ve := model.NewValidationErrors()
		ve.Add("file", "CSV file contains no data rows.")
		return nil, ve
	}
	if len(rows) > model.MaxImportRows {
		ve := model.NewValidationErrors()
		ve.Add("file", fmt.Sprintf("CSV file exceeds maximum of %d rows.", model.MaxImportRows))
		return nil, ve
	}

	// Build known-categories map.
	cats, err := s.repo.ListCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading categories: %w", err)
	}
	knownCategories := make(map[string]bool, len(cats))
	for _, c := range cats {
		knownCategories[c.Name] = true
	}

	// Validate each row.
	results := make([]model.ImportRowResult, 0, len(rows))
	validRows, invalidRows := 0, 0
	for i, row := range rows {
		result := model.ImportRowResult{
			Row:      i + 2, // +2: 1-indexed, skip header
			Title:    row["title"],
			Category: row["category"],
			Tags:     row["tags"],
		}
		errs := ValidateImportRow(result, knownCategories)
		if len(errs) == 0 {
			result.Status = "VALID"
			validRows++
		} else {
			result.Status = "ERROR"
			result.Errors = errs
			invalidRows++
		}
		results = append(results, result)
	}

	// Persist file.
	if err := os.MkdirAll(s.uploadDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating import dir: %w", err)
	}
	fileID := uuid.New()
	ext := filepath.Ext(originalFilename)
	if ext == "" {
		ext = ".csv"
	}
	storedPath := filepath.Join(s.uploadDir, fileID.String()+ext)
	if err := os.WriteFile(storedPath, data, 0o644); err != nil {
		return nil, fmt.Errorf("saving import file: %w", err)
	}

	job := &model.BulkImportJob{
		ID:               uuid.New(),
		UploadedBy:       actorID,
		FilePath:         storedPath,
		OriginalFilename: originalFilename,
		Status:           model.BulkImportStatusPreviewReady,
		TotalRows:        len(rows),
		ValidRows:        validRows,
		InvalidRows:      invalidRows,
		Results:          results,
	}

	if err := s.repo.CreateImportJob(ctx, job); err != nil {
		_ = os.Remove(storedPath)
		return nil, err
	}

	s.auditSvc.Record(ctx, audit.Entry{ //nolint:errcheck
		ActorID: actorID, Action: "bulk_import.upload",
		EntityType: "bulk_import_job", EntityID: job.ID,
		AfterData: map[string]string{
			"filename":  originalFilename,
			"total_rows": fmt.Sprintf("%d", len(rows)),
		},
	})

	return job, nil
}

// GetJob returns the import job (for preview rendering).
func (s *BulkImportService) GetJob(ctx context.Context, id uuid.UUID) (*model.BulkImportJob, error) {
	return s.repo.GetImportJob(ctx, id)
}

// Confirm imports all VALID rows as DRAFT resources, updates job to CONFIRMED.
// Only valid rows are imported; invalid rows are skipped.
func (s *BulkImportService) Confirm(ctx context.Context, actorID, jobID uuid.UUID) (*model.BulkImportJob, error) {
	job, err := s.repo.GetImportJob(ctx, jobID)
	if err != nil {
		return nil, err
	}

	if job.Status != model.BulkImportStatusPreviewReady {
		ve := model.NewValidationErrors()
		ve.Add("job", fmt.Sprintf("Job is in status %s; only PREVIEW_READY jobs can be confirmed.", job.Status))
		return nil, ve
	}

	if job.UploadedBy != actorID {
		ve := model.NewValidationErrors()
		ve.Add("job", "You do not own this import job.")
		return nil, ve
	}

	// Build category name → ID map.
	cats, err := s.repo.ListCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading categories: %w", err)
	}
	catByName := make(map[string]uuid.UUID, len(cats))
	for _, c := range cats {
		catByName[c.Name] = c.ID
	}

	now := time.Now()
	imported := 0
	for _, row := range job.Results {
		if row.Status != "VALID" {
			continue
		}

		var catID *uuid.UUID
		if row.Category != "" {
			if id, ok := catByName[strings.TrimSpace(row.Category)]; ok {
				catID = &id
			}
		}

		// Parse tags.
		var tagNames []string
		if row.Tags != "" {
			for _, t := range strings.Split(row.Tags, ",") {
				t = strings.TrimSpace(t)
				if t != "" {
					tagNames = append(tagNames, t)
				}
			}
		}

		// Resolve/create tag IDs.
		tagIDs := make([]uuid.UUID, 0, len(tagNames))
		for _, name := range tagNames {
			existing, err := s.repo.GetTagByName(ctx, name)
			if err == nil {
				tagIDs = append(tagIDs, existing.ID)
				continue
			}
			newTag := &model.Tag{ID: uuid.New(), Name: name}
			if err := s.repo.CreateTag(ctx, newTag); err == nil {
				tagIDs = append(tagIDs, newTag.ID)
			}
		}

		r := &model.Resource{
			ID:                   uuid.New(),
			Title:                strings.TrimSpace(row.Title),
			AuthorID:             actorID,
			CategoryID:           catID,
			Status:               model.ResourceStatusDraft,
			CurrentVersionNumber: 1,
			Version:              1,
		}
		if err := s.repo.CreateResource(ctx, r); err != nil {
			continue // best-effort; skip failed rows
		}

		if len(tagIDs) > 0 {
			_ = s.repo.SetTags(ctx, r.ID, tagIDs)
		}

		imported++
	}

	job.Status = model.BulkImportStatusConfirmed
	job.CompletedAt = &now
	if err := s.repo.UpdateImportJob(ctx, job); err != nil {
		return nil, err
	}

	s.auditSvc.Record(ctx, audit.Entry{ //nolint:errcheck
		ActorID: actorID, Action: "bulk_import.confirm",
		EntityType: "bulk_import_job", EntityID: jobID,
		AfterData: map[string]string{
			"imported": fmt.Sprintf("%d", imported),
		},
	})

	return job, nil
}

// ─── internal helpers ─────────────────────────────────────────────────────────

// parseCSV parses CSV bytes; first row is header. Returns slice of row maps.
// Required header columns: title. Optional: category, tags.
func parseCSV(data []byte) ([]map[string]string, error) {
	r := csv.NewReader(strings.NewReader(string(data)))
	r.TrimLeadingSpace = true
	r.FieldsPerRecord = -1 // allow rows with varying field count

	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("reading header: %w", err)
	}

	// Normalize header column names.
	colIndex := make(map[string]int, len(header))
	for i, h := range header {
		colIndex[strings.ToLower(strings.TrimSpace(h))] = i
	}

	if _, ok := colIndex["title"]; !ok {
		return nil, fmt.Errorf("missing required column: title")
	}

	var rows []map[string]string
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parsing row %d: %w", len(rows)+2, err)
		}

		row := make(map[string]string, len(colIndex))
		for col, idx := range colIndex {
			if idx < len(record) {
				row[col] = strings.TrimSpace(record[idx])
			}
		}
		rows = append(rows, row)
	}

	return rows, nil
}
