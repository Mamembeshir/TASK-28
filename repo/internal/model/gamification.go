package model

import (
	"time"

	"github.com/google/uuid"
)

// UserPoints holds the gamification totals for a user.
type UserPoints struct {
	UserID      uuid.UUID `json:"user_id"`
	TotalPoints int       `json:"total_points"`
	Level       int       `json:"level"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Level returns the computed level for a given total.
// Level = floor(total / 200), min 0.
func ComputeLevel(totalPoints int) int {
	if totalPoints < 0 {
		return 0
	}
	return totalPoints / 200
}

// PointTransaction is an immutable record of a point change.
type PointTransaction struct {
	ID         uuid.UUID  `json:"id"`
	UserID     uuid.UUID  `json:"user_id"`
	Points     int        `json:"points"` // signed; negative = penalty
	Reason     string     `json:"reason"`
	SourceType string     `json:"source_type"` // e.g. ENTRY_APPROVED, UPVOTE_RECEIVED
	SourceID   *uuid.UUID `json:"source_id,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// PointRule defines how many points an event is worth.
type PointRule struct {
	ID          uuid.UUID `json:"id"`
	EventType   string    `json:"event_type"` // ENTRY_APPROVED, UPVOTE_RECEIVED, ...
	Points      int       `json:"points"`
	Description string    `json:"description"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Point rule event type constants.
const (
	EventEntryApproved   = "ENTRY_APPROVED"
	EventUpvoteReceived  = "UPVOTE_RECEIVED"
	EventDownvoteReceived = "DOWNVOTE_RECEIVED"
	EventFavoriteReceived = "FAVORITE_RECEIVED"
	EventTakedownPenalty  = "TAKEDOWN_PENALTY"
)

// Badge is an achievement awarded when a threshold is met.
type Badge struct {
	ID             uuid.UUID `json:"id"`
	Code           string    `json:"code"` // unique slug e.g. "POPULAR_50"
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	ThresholdType  string    `json:"threshold_type"`  // FAVORITES_RECEIVED, ENTRIES_APPROVED, UPVOTES_RECEIVED, TOTAL_POINTS
	ThresholdValue int       `json:"threshold_value"`
	CreatedAt      time.Time `json:"created_at"`
}

// UserBadge records when a badge was awarded to a user.
type UserBadge struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	BadgeID   uuid.UUID `json:"badge_id"`
	AwardedAt time.Time `json:"awarded_at"`

	// Populated on reads
	Badge *Badge `json:"badge,omitempty"`
}

// RankingEntry is one resource's entry in a ranking.
type RankingEntry struct {
	Rank       int       `json:"rank"`
	ResourceID uuid.UUID `json:"resource_id"`
	Title      string    `json:"title"`
	AuthorName string    `json:"author_name"`
	Score      int       `json:"score"` // upvote count for bestseller; 0 for new release
}

// RankingArchive is a weekly snapshot of rankings.
type RankingArchive struct {
	ID          uuid.UUID      `json:"id"`
	WeekNumber  int            `json:"week_number"` // ISO week
	Year        int            `json:"year"`
	RankingType string         `json:"ranking_type"` // BESTSELLER | NEW_RELEASE
	Entries     []RankingEntry `json:"entries"`
	CreatedAt   time.Time      `json:"created_at"`
}

// RecommendationStrategyConfig is the Admin-managed registry entry.
type RecommendationStrategyConfig struct {
	ID          uuid.UUID `json:"id"`
	StrategyKey string    `json:"strategy_key"`
	Label       string    `json:"label"`
	SortOrder   int       `json:"sort_order"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// LeaderboardEntry is one user's entry in the points leaderboard.
type LeaderboardEntry struct {
	Rank        int       `json:"rank"`
	UserID      uuid.UUID `json:"user_id"`
	Username    string    `json:"username"`
	TotalPoints int       `json:"total_points"`
	Level       int       `json:"level"`
}

// RecommendationSection is one strategy's output with its label.
type RecommendationSection struct {
	Label     string     `json:"label"`
	Strategy  string     `json:"strategy"`
	Resources []Resource `json:"resources"`
}
