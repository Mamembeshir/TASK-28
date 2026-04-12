package searchrepo

import (
	"context"

	"github.com/eduexchange/eduexchange/internal/model"
	"github.com/google/uuid"
)

// SearchRepository is the persistence interface for the search domain.
type SearchRepository interface {
	// ── Full-text Search ─────────────────────────────────────────────────────
	SearchResources(ctx context.Context, f model.SearchFilter, expandedTerms []string) ([]model.Resource, int, error)
	// FindDidYouMean returns the best trigram-similar term for a query.
	FindDidYouMean(ctx context.Context, query string) (string, error)

	// ── Search Index ──────────────────────────────────────────────────────────
	UpsertSearchIndex(ctx context.Context, resourceID uuid.UUID, pinyinContent, tagContent string) error

	// ── Pinyin Mapping ────────────────────────────────────────────────────────
	GetPinyinMappings(ctx context.Context) (map[string]string, error)

	// ── Synonyms ─────────────────────────────────────────────────────────────
	ListSynonymGroups(ctx context.Context) ([]model.SynonymGroup, error)
	CreateSynonymGroup(ctx context.Context, sg *model.SynonymGroup) error
	UpdateSynonymGroup(ctx context.Context, sg *model.SynonymGroup) error
	DeleteSynonymGroup(ctx context.Context, id uuid.UUID) error
	ExpandQuery(ctx context.Context, query string) ([]string, error) // returns synonyms + canonical

	// ── Search Terms (type-ahead) ─────────────────────────────────────────────
	TypeAhead(ctx context.Context, prefix string, limit int) ([]model.SearchSuggestion, error)
	IncrementSearchTerm(ctx context.Context, term string) error

	// ── User Search History ───────────────────────────────────────────────────
	AddSearchHistory(ctx context.Context, h *model.UserSearchHistory) error
	GetSearchHistory(ctx context.Context, userID uuid.UUID, limit int) ([]model.UserSearchHistory, error)
	ClearSearchHistory(ctx context.Context, userID uuid.UUID) error
	// TrimSearchHistory removes oldest entries beyond limit.
	TrimSearchHistory(ctx context.Context, userID uuid.UUID, limit int) error
}
