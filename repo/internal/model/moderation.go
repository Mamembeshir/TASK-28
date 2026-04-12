package model

import (
	"time"

	"github.com/google/uuid"
)

// Report is a user-submitted report about a resource.
type Report struct {
	ID          uuid.UUID  `json:"id"`
	ReporterID  uuid.UUID  `json:"reporter_id"`
	ResourceID  uuid.UUID  `json:"resource_id"`
	ReasonType  string     `json:"reason_type"`
	Description string     `json:"description"`
	Status      string     `json:"status"`
	ReviewerID  *uuid.UUID `json:"reviewer_id,omitempty"`
	Notes       string     `json:"notes"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	// Populated on reads
	ResourceTitle string `json:"resource_title,omitempty"`
	ReporterName  string `json:"reporter_name,omitempty"`
	ReviewerName  string `json:"reviewer_name,omitempty"`
}

// ModerationAction records an action taken by a moderator.
type ModerationAction struct {
	ID           uuid.UUID              `json:"id"`
	ModeratorID  uuid.UUID              `json:"moderator_id"`
	ActionType   string                 `json:"action_type"`
	TargetType   string                 `json:"target_type"` // RESOURCE | USER
	TargetID     uuid.UUID              `json:"target_id"`
	ReportID     *uuid.UUID             `json:"report_id,omitempty"`
	Notes        string                 `json:"notes"`
	EvidenceJSON map[string]interface{} `json:"evidence_json"`
	CreatedAt    time.Time              `json:"created_at"`
	// Populated on reads
	ModeratorName string `json:"moderator_name,omitempty"`
}

// UserBan records a ban applied to a user.
type UserBan struct {
	ID        uuid.UUID  `json:"id"`
	UserID    uuid.UUID  `json:"user_id"`
	BanType   string     `json:"ban_type"`
	Reason    string     `json:"reason"`
	BannedBy  uuid.UUID  `json:"banned_by"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	IsActive  bool       `json:"is_active"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}
