package catalogservice

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/eduexchange/eduexchange/internal/audit"
	catalogrepo "github.com/eduexchange/eduexchange/internal/repository/catalog"
	"github.com/google/uuid"
)

// BulkExportService handles CSV export of resource metadata (CAT-08).
// Admin only — enforced at handler level.
type BulkExportService struct {
	repo      catalogrepo.CatalogRepository
	auditSvc  *audit.Service
	exportDir string // e.g. "data/exports"
}

func NewBulkExportService(repo catalogrepo.CatalogRepository, auditSvc *audit.Service, exportDir string) *BulkExportService {
	return &BulkExportService{repo: repo, auditSvc: auditSvc, exportDir: exportDir}
}

// ExportResult contains the path and filename of the generated CSV.
type ExportResult struct {
	FilePath string
	Filename string
}

// Export generates a CSV of all resource metadata and saves it to exportDir.
// Returns the file path for download. Audit-logged.
func (s *BulkExportService) Export(ctx context.Context, actorID uuid.UUID) (*ExportResult, error) {
	// Fetch all resources (no pagination — admin export).
	filter := catalogrepo.ResourceFilter{
		Page:     1,
		PageSize: 10_000, // effectively all
	}
	resources, _, err := s.repo.ListResources(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("listing resources: %w", err)
	}

	// Build CSV in memory.
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	headers := []string{
		"id", "title", "description", "author_id", "author_name",
		"category_id", "category_name", "status",
		"current_version_number", "tags", "created_at", "updated_at",
	}
	if err := w.Write(headers); err != nil {
		return nil, fmt.Errorf("writing CSV header: %w", err)
	}

	for _, r := range resources {
		// Collect tag names.
		tagNames := make([]string, 0, len(r.Tags))
		for _, t := range r.Tags {
			tagNames = append(tagNames, t.Name)
		}
		tagsStr := joinStrings(tagNames, "|")

		catID := ""
		if r.CategoryID != nil {
			catID = r.CategoryID.String()
		}

		row := []string{
			r.ID.String(),
			r.Title,
			r.Description,
			r.AuthorID.String(),
			r.AuthorName,
			catID,
			r.CategoryName,
			r.Status.String(),
			fmt.Sprintf("%d", r.CurrentVersionNumber),
			tagsStr,
			r.CreatedAt.UTC().Format(time.RFC3339),
			r.UpdatedAt.UTC().Format(time.RFC3339),
		}
		if err := w.Write(row); err != nil {
			return nil, fmt.Errorf("writing CSV row: %w", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("flushing CSV: %w", err)
	}

	// Save to disk.
	if err := os.MkdirAll(s.exportDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating export dir: %w", err)
	}
	filename := fmt.Sprintf("resources_export_%s.csv", time.Now().UTC().Format("20060102_150405"))
	filePath := filepath.Join(s.exportDir, filename)
	if err := os.WriteFile(filePath, buf.Bytes(), 0o644); err != nil {
		return nil, fmt.Errorf("saving export file: %w", err)
	}

	s.auditSvc.Record(ctx, audit.Entry{ //nolint:errcheck
		ActorID: actorID, Action: "bulk_export.generate",
		EntityType: "bulk_export",
		AfterData: map[string]string{
			"filename":       filename,
			"resource_count": fmt.Sprintf("%d", len(resources)),
		},
	})

	return &ExportResult{FilePath: filePath, Filename: filename}, nil
}

func joinStrings(ss []string, sep string) string {
	if len(ss) == 0 {
		return ""
	}
	result := ss[0]
	for _, s := range ss[1:] {
		result += sep + s
	}
	return result
}
