package searchservice

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/eduexchange/eduexchange/internal/model"
	searchrepo "github.com/eduexchange/eduexchange/internal/repository/search"
	"github.com/google/uuid"
)

// SearchService implements the full search pipeline:
// pinyin conversion → synonym expansion → FTS → trigram fallback → history.
type SearchService struct {
	repo         searchrepo.SearchRepository
	pinyinCache  map[string]string // character → pinyin
	cacheLoaded  bool
}

func NewSearchService(repo searchrepo.SearchRepository) *SearchService {
	return &SearchService{repo: repo}
}

// Search executes the full search pipeline per SRCH-01 through SRCH-07.
func (s *SearchService) Search(ctx context.Context, f model.SearchFilter, userID *uuid.UUID) (*model.SearchResult, error) {
	query := strings.TrimSpace(f.Query)

	// 1. Convert query chars to pinyin where applicable (SRCH-02).
	pinyinQuery := s.toPinyin(ctx, query)

	// 2. Expand with synonyms (SRCH-04).
	expanded, err := s.repo.ExpandQuery(ctx, query)
	if err != nil {
		expanded = nil // non-fatal
	}
	// Also add pinyin-converted query as an expansion term.
	if pinyinQuery != "" && pinyinQuery != query {
		expanded = append(expanded, pinyinQuery)
	}

	// Set default page/size.
	if f.Page <= 0 {
		f.Page = 1
	}
	if f.PageSize <= 0 || f.PageSize > 100 {
		f.PageSize = 20
	}

	// 3. Run FTS + pinyin + tag content search (SRCH-01, SRCH-02, SRCH-05, SRCH-06).
	resources, total, err := s.repo.SearchResources(ctx, f, expanded)
	if err != nil {
		return nil, err
	}

	// 4. If no results, check for "did you mean" via trigram (SRCH-03).
	var didYouMean string
	if len(resources) == 0 && query != "" {
		didYouMean, _ = s.repo.FindDidYouMean(ctx, query)
	}

	// 5. Record history + increment search term usage (SRCH-07, SRCH-08).
	if query != "" && userID != nil {
		go func() {
			bgCtx := context.Background()
			h := &model.UserSearchHistory{
				ID:     uuid.New(),
				UserID: *userID,
				Query:  query,
			}
			_ = s.repo.AddSearchHistory(bgCtx, h)
			_ = s.repo.TrimSearchHistory(bgCtx, *userID, 20)
			_ = s.repo.IncrementSearchTerm(bgCtx, query)
		}()
	}

	return &model.SearchResult{
		Resources:  resources,
		Total:      total,
		Page:       f.Page,
		PageSize:   f.PageSize,
		Query:      query,
		DidYouMean: didYouMean,
	}, nil
}

// TypeAhead returns up to 5 suggestions for a prefix (SRCH-07, min 2 chars).
func (s *SearchService) TypeAhead(ctx context.Context, prefix string) ([]model.SearchSuggestion, error) {
	prefix = strings.TrimSpace(prefix)
	if utf8.RuneCountInString(prefix) < 2 {
		return nil, nil
	}
	return s.repo.TypeAhead(ctx, prefix, 5)
}

// GetHistory returns the last 20 search queries for a user (SRCH-08).
func (s *SearchService) GetHistory(ctx context.Context, userID uuid.UUID) ([]model.UserSearchHistory, error) {
	return s.repo.GetSearchHistory(ctx, userID, 20)
}

// ClearHistory deletes all search history for a user.
func (s *SearchService) ClearHistory(ctx context.Context, userID uuid.UUID) error {
	return s.repo.ClearSearchHistory(ctx, userID)
}

// ListSynonymGroups returns all synonym groups for Admin management.
func (s *SearchService) ListSynonymGroups(ctx context.Context) ([]model.SynonymGroup, error) {
	return s.repo.ListSynonymGroups(ctx)
}

// CreateSynonymGroup adds a new synonym group.
func (s *SearchService) CreateSynonymGroup(ctx context.Context, canonical string, synonyms []string) (*model.SynonymGroup, error) {
	sg := &model.SynonymGroup{
		ID:            uuid.New(),
		CanonicalTerm: strings.TrimSpace(strings.ToLower(canonical)),
		Synonyms:      synonyms,
	}
	if err := s.repo.CreateSynonymGroup(ctx, sg); err != nil {
		return nil, err
	}
	return sg, nil
}

// UpdateSynonymGroup updates an existing synonym group.
func (s *SearchService) UpdateSynonymGroup(ctx context.Context, sg *model.SynonymGroup) error {
	return s.repo.UpdateSynonymGroup(ctx, sg)
}

// DeleteSynonymGroup removes a synonym group.
func (s *SearchService) DeleteSynonymGroup(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteSynonymGroup(ctx, id)
}

// UpdateSearchIndex refreshes the pinyin and tag content for a resource.
// Called after resource create/update.
func (s *SearchService) UpdateSearchIndex(ctx context.Context, r *model.Resource) error {
	// Build pinyin content from title + description.
	pinyinContent := s.toPinyin(ctx, r.Title+" "+r.Description)
	// Build tag content.
	tagNames := make([]string, len(r.Tags))
	for i, t := range r.Tags {
		tagNames[i] = t.Name
	}
	tagContent := strings.Join(tagNames, " ")
	return s.repo.UpsertSearchIndex(ctx, r.ID, pinyinContent, tagContent)
}

// toPinyin converts Chinese characters in a string to their pinyin representations.
func (s *SearchService) toPinyin(ctx context.Context, text string) string {
	if text == "" {
		return ""
	}
	// Lazy-load pinyin mapping from DB.
	if !s.cacheLoaded {
		m, err := s.repo.GetPinyinMappings(ctx)
		if err == nil {
			s.pinyinCache = m
			s.cacheLoaded = true
		}
	}
	if len(s.pinyinCache) == 0 {
		return ""
	}

	var result []string
	for _, r := range text {
		char := string(r)
		if pinyin, ok := s.pinyinCache[char]; ok {
			result = append(result, pinyin)
		} else if r > 127 {
			// Non-ASCII, non-mapped — skip
			continue
		} else {
			result = append(result, char)
		}
	}
	return strings.Join(result, "")
}

// ConvertToPinyin exposes pinyin conversion for testing.
func (s *SearchService) ConvertToPinyin(ctx context.Context, text string) string {
	return s.toPinyin(ctx, text)
}
