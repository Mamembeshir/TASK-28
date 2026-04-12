package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ─── Category ─────────────────────────────────────────────────────────────────

type Category struct {
	ID        uuid.UUID  `json:"id"`
	Name      string     `json:"name"`
	ParentID  *uuid.UUID `json:"parent_id,omitempty"`
	Level     int        `json:"level"` // 1–3
	SortOrder int        `json:"sort_order"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	// Populated on reads
	Children []*Category `json:"children,omitempty"`
}

// ─── Tag ──────────────────────────────────────────────────────────────────────

type Tag struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// ─── Resource ─────────────────────────────────────────────────────────────────

type Resource struct {
	ID                   uuid.UUID      `json:"id"`
	Title                string         `json:"title"`
	Description          string         `json:"description"`
	ContentBody          string         `json:"content_body"`
	AuthorID             uuid.UUID      `json:"author_id"`
	CategoryID           *uuid.UUID     `json:"category_id,omitempty"`
	Status               ResourceStatus `json:"status"`
	CurrentVersionNumber int            `json:"current_version_number"`
	Version              int            `json:"version"` // optimistic lock
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`

	// Populated on reads
	Tags         []Tag          `json:"tags,omitempty"`
	Files        []ResourceFile `json:"files,omitempty"`
	AuthorName   string         `json:"author_name,omitempty"`
	CategoryName string         `json:"category_name,omitempty"`
}

// ─── ResourceVersion (immutable) ─────────────────────────────────────────────

type ResourceVersion struct {
	ID            uuid.UUID       `json:"id"`
	ResourceID    uuid.UUID       `json:"resource_id"`
	VersionNumber int             `json:"version_number"`
	DataSnapshot  json.RawMessage `json:"data_snapshot"` // full resource JSON at point-in-time
	ChangedBy     uuid.UUID       `json:"changed_by"`
	CreatedAt     time.Time       `json:"created_at"`
}

// ResourceSnapshot is the struct stored inside DataSnapshot JSON.
type ResourceSnapshot struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	ContentBody string    `json:"content_body"`
	CategoryID  *uuid.UUID `json:"category_id,omitempty"`
	Tags        []string  `json:"tags"`
}

// ─── ResourceFile ─────────────────────────────────────────────────────────────

type ResourceFile struct {
	ID           uuid.UUID `json:"id"`
	ResourceID   uuid.UUID `json:"resource_id"`
	OriginalName string    `json:"original_name"`
	StoredPath   string    `json:"stored_path"`
	MIMEType     string    `json:"mime_type"`
	SizeBytes    int64     `json:"size_bytes"`
	SHA256       string    `json:"sha256"`
	CreatedAt    time.Time `json:"created_at"`
}

// ─── ResourceReview ───────────────────────────────────────────────────────────

type ResourceReview struct {
	ID         uuid.UUID `json:"id"`
	ResourceID uuid.UUID `json:"resource_id"`
	ReviewerID uuid.UUID `json:"reviewer_id"`
	Action     string    `json:"action"` // "APPROVED" | "REJECTED"
	Notes      string    `json:"notes"`
	CreatedAt  time.Time `json:"created_at"`

	// Populated on reads
	ReviewerName string `json:"reviewer_name,omitempty"`
}

// ─── BulkImportJob ────────────────────────────────────────────────────────────

type BulkImportJob struct {
	ID               uuid.UUID        `json:"id"`
	UploadedBy       uuid.UUID        `json:"uploaded_by"`
	FilePath         string           `json:"file_path"`
	OriginalFilename string           `json:"original_filename"`
	Status           BulkImportStatus `json:"status"`
	TotalRows        int              `json:"total_rows"`
	ValidRows        int              `json:"valid_rows"`
	InvalidRows      int              `json:"invalid_rows"`
	Results          []ImportRowResult `json:"results,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
	CompletedAt      *time.Time       `json:"completed_at,omitempty"`
}

// ImportRowResult represents the validation result for a single import row.
type ImportRowResult struct {
	Row      int               `json:"row"`
	Status   string            `json:"status"` // "VALID" | "ERROR"
	Title    string            `json:"title"`
	Category string            `json:"category"`
	Tags     string            `json:"tags"`
	Errors   []ImportRowError  `json:"errors,omitempty"`
}

type ImportRowError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ─── Allowed MIME types (CAT-06) ─────────────────────────────────────────────

var AllowedMIMETypes = map[string]string{
	"application/pdf": ".pdf",
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   ".docx",
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": ".pptx",
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         ".xlsx",
	"image/jpeg": ".jpg",
	"image/png":  ".png",
}

const (
	MaxFileSize      = 50 * 1024 * 1024 // 50 MB
	MaxFilesPerResource = 5
	MaxImportRows    = 500
	MaxImportFileSize = 25 * 1024 * 1024 // 25 MB
)
