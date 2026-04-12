package model

import (
	"time"

	"github.com/google/uuid"
)

// SearchFilter holds all query parameters for a search request.
type SearchFilter struct {
	Query      string     `json:"query"`
	AuthorID   *uuid.UUID `json:"author_id,omitempty"`
	CategoryID *uuid.UUID `json:"category_id,omitempty"`
	Tags       []string   `json:"tags,omitempty"`
	DateFrom   *time.Time `json:"date_from,omitempty"`
	DateTo     *time.Time `json:"date_to,omitempty"`
	Status     string     `json:"status,omitempty"` // empty = PUBLISHED only for non-admin
	Sort       string     `json:"sort,omitempty"`   // relevance | newest | most_upvoted | most_favorited
	Page       int        `json:"page"`
	PageSize   int        `json:"page_size"`
}

// SearchResult wraps resources with metadata.
type SearchResult struct {
	Resources  []Resource `json:"resources"`
	Total      int        `json:"total"`
	Page       int        `json:"page"`
	PageSize   int        `json:"page_size"`
	Query      string     `json:"query"`
	DidYouMean string     `json:"did_you_mean,omitempty"` // trigram suggestion
}

// SearchSuggestion is a single type-ahead entry.
type SearchSuggestion struct {
	Term       string `json:"term"`
	UsageCount int    `json:"usage_count"`
}

// SearchTerm is the stored type-ahead entry in the DB.
type SearchTerm struct {
	ID         uuid.UUID `json:"id"`
	Term       string    `json:"term"`
	Category   string    `json:"category"`
	UsageCount int       `json:"usage_count"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// UserSearchHistory is one recorded search query.
type UserSearchHistory struct {
	ID         uuid.UUID   `json:"id"`
	UserID     uuid.UUID   `json:"user_id"`
	Query      string      `json:"query"`
	FiltersRaw interface{} `json:"filters,omitempty"`
	SearchedAt time.Time   `json:"searched_at"`
}

// SynonymGroup maps a canonical term to its synonyms.
type SynonymGroup struct {
	ID            uuid.UUID `json:"id"`
	CanonicalTerm string    `json:"canonical_term"`
	Synonyms      []string  `json:"synonyms"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// SearchIndexEntry is the search_index row.
type SearchIndexEntry struct {
	ResourceID     uuid.UUID `json:"resource_id"`
	PinyinContent  string    `json:"pinyin_content"`
	TagContent     string    `json:"tag_content"`
	UpdatedAt      time.Time `json:"updated_at"`
}
