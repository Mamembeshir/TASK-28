package model

import (
	"time"

	"github.com/google/uuid"
)

// Vote is a user's up/down vote on a resource.
type Vote struct {
	ID         uuid.UUID `json:"id"`
	UserID     uuid.UUID `json:"user_id"`
	ResourceID uuid.UUID `json:"resource_id"`
	VoteType   VoteType  `json:"vote_type"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// VoteCounts holds the upvote/downvote totals for a resource.
type VoteCounts struct {
	ResourceID uuid.UUID `json:"resource_id"`
	Upvotes    int       `json:"upvotes"`
	Downvotes  int       `json:"downvotes"`
	Score      int       `json:"score"` // upvotes - downvotes
}

// Favorite records a user's bookmarked resource.
type Favorite struct {
	ID         uuid.UUID `json:"id"`
	UserID     uuid.UUID `json:"user_id"`
	ResourceID uuid.UUID `json:"resource_id"`
	CreatedAt  time.Time `json:"created_at"`
}

// Follow records a user following an author or topic.
type Follow struct {
	ID         uuid.UUID        `json:"id"`
	FollowerID uuid.UUID        `json:"follower_id"`
	TargetType FollowTargetType `json:"target_type"`
	TargetID   uuid.UUID        `json:"target_id"`
	CreatedAt  time.Time        `json:"created_at"`
}

// AnomalyFlag records a detected anomalous pattern.
type AnomalyFlag struct {
	ID           uuid.UUID              `json:"id"`
	FlagType     string                 `json:"flag_type"` // LIKE_RING, RATE_SPIKE, OTHER
	UserIDs      []uuid.UUID            `json:"user_ids"`
	EvidenceJSON map[string]interface{} `json:"evidence_json"`
	Status       string                 `json:"status"` // OPEN, REVIEWED, DISMISSED
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}
