package frontend_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/eduexchange/eduexchange/internal/middleware"
	"github.com/eduexchange/eduexchange/internal/model"
	searchpages "github.com/eduexchange/eduexchange/internal/templ/pages/search"
)

var (
	searchAdminUser = &middleware.AuthUser{
		ID:       uuid.New(),
		Username: "admin",
		Roles:    []string{"ADMIN"},
	}
	searchRegularUser = &middleware.AuthUser{
		ID:       uuid.New(),
		Username: "alice",
		Roles:    []string{"REGULAR_USER"},
	}

	sampleSearchResult = &model.SearchResult{
		Resources: []model.Resource{
			{
				ID:          uuid.New(),
				Title:       "Introduction to Go Programming",
				Description: "A beginner guide.",
				AuthorName:  "Alice",
				Status:      model.ResourceStatusPublished,
			},
		},
		Total:    1,
		Page:     1,
		PageSize: 20,
		Query:    "golang",
	}
)

// ─── SearchPage ───────────────────────────────────────────────────────────────

func TestSearchPage_RendersSearchBar(t *testing.T) {
	html := render(t, searchpages.SearchPage(searchpages.SearchData{
		Result:   sampleSearchResult,
		AuthUser: searchRegularUser,
	}))

	assert.Contains(t, html, `name="q"`)
	assert.Contains(t, html, "Search Resources")
}

func TestSearchPage_RendersResults(t *testing.T) {
	html := render(t, searchpages.SearchPage(searchpages.SearchData{
		Result:   sampleSearchResult,
		AuthUser: searchRegularUser,
	}))

	assert.Contains(t, html, "Introduction to Go Programming")
	assert.Contains(t, html, "Alice")
}

func TestSearchPage_EmptyStateShowsNoResults(t *testing.T) {
	html := render(t, searchpages.SearchPage(searchpages.SearchData{
		Result: &model.SearchResult{
			Resources: nil,
			Total:     0,
			Query:     "zzz",
		},
		AuthUser: searchRegularUser,
	}))

	assert.Contains(t, html, "No resources found")
}

func TestSearchPage_DidYouMean(t *testing.T) {
	html := render(t, searchpages.SearchPage(searchpages.SearchData{
		Result: &model.SearchResult{
			Resources:  nil,
			Total:      0,
			Query:      "mathamatics",
			DidYouMean: "mathematics",
		},
		AuthUser: searchRegularUser,
	}))

	assert.Contains(t, html, "Did you mean")
	assert.Contains(t, html, "mathematics")
}

func TestSearchPage_HasTypeAheadTrigger(t *testing.T) {
	html := render(t, searchpages.SearchPage(searchpages.SearchData{
		Result:   sampleSearchResult,
		AuthUser: searchRegularUser,
	}))

	assert.Contains(t, html, "hx-get")
	assert.Contains(t, html, "keyup changed delay:300ms")
}

func TestSearchPage_HistoryShownWhenPresent(t *testing.T) {
	history := []model.UserSearchHistory{
		{ID: uuid.New(), UserID: searchRegularUser.ID, Query: "golang"},
		{ID: uuid.New(), UserID: searchRegularUser.ID, Query: "python"},
	}
	html := render(t, searchpages.SearchPage(searchpages.SearchData{
		Result:   sampleSearchResult,
		History:  history,
		AuthUser: searchRegularUser,
	}))

	assert.Contains(t, html, "Recent Searches")
	assert.Contains(t, html, "golang")
	assert.Contains(t, html, "python")
	assert.Contains(t, html, "Clear All")
}

func TestSearchPage_NoHistoryWhenLoggedOut(t *testing.T) {
	html := render(t, searchpages.SearchPage(searchpages.SearchData{
		Result:   sampleSearchResult,
		AuthUser: nil, // logged out
	}))

	assert.NotContains(t, html, "Recent Searches")
}

// ─── SuggestionsDropdown ──────────────────────────────────────────────────────

func TestSuggestionsDropdown_RendersSuggestions(t *testing.T) {
	suggestions := []model.SearchSuggestion{
		{Term: "mathematics", UsageCount: 10},
		{Term: "math", UsageCount: 5},
	}
	html := render(t, searchpages.SuggestionsDropdown(suggestions))

	assert.Contains(t, html, "mathematics")
	assert.Contains(t, html, "math")
}

func TestSuggestionsDropdown_EmptyRendersNothing(t *testing.T) {
	html := render(t, searchpages.SuggestionsDropdown(nil))
	assert.Empty(t, html)
}

// ─── RankingsPage ─────────────────────────────────────────────────────────────

func TestRankingsPage_ShowsBestsellers(t *testing.T) {
	entries := []model.RankingEntry{
		{Rank: 1, ResourceID: uuid.New(), Title: "Go Programming Guide", AuthorName: "Bob", Score: 42},
		{Rank: 2, ResourceID: uuid.New(), Title: "Python Basics", AuthorName: "Carol", Score: 30},
	}
	html := render(t, searchpages.RankingsPage(searchpages.RankingsData{
		Bestsellers: entries,
		AuthUser:    searchAdminUser,
	}))

	assert.Contains(t, html, "Bestsellers This Week")
	assert.Contains(t, html, "Go Programming Guide")
	assert.Contains(t, html, "Python Basics")
	assert.Contains(t, html, "42")
}

func TestRankingsPage_EmptyBestsellersState(t *testing.T) {
	html := render(t, searchpages.RankingsPage(searchpages.RankingsData{
		Bestsellers: nil,
		NewReleases: nil,
		AuthUser:    searchAdminUser,
	}))

	assert.Contains(t, html, "No bestsellers yet this week")
	assert.Contains(t, html, "No new releases yet this week")
}

func TestRankingsPage_ShowsNewReleases(t *testing.T) {
	releases := []model.RankingEntry{
		{Rank: 1, ResourceID: uuid.New(), Title: "Brand New Resource", AuthorName: "Dave"},
	}
	html := render(t, searchpages.RankingsPage(searchpages.RankingsData{
		NewReleases: releases,
		AuthUser:    searchAdminUser,
	}))

	assert.Contains(t, html, "New Releases This Week")
	assert.Contains(t, html, "Brand New Resource")
}

// ─── HomePage ─────────────────────────────────────────────────────────────────

func TestHomePage_ShowsRecommendationSections(t *testing.T) {
	sections := []model.RecommendationSection{
		{
			Label:    "Based on Your Interests",
			Strategy: "SimilarTagAffinity",
			Resources: []model.Resource{
				{ID: uuid.New(), Title: "Tag Affinity Resource", AuthorName: "Eve"},
			},
		},
		{
			Label:    "From Authors You Follow",
			Strategy: "FollowedAuthorNewContent",
			Resources: []model.Resource{
				{ID: uuid.New(), Title: "Followed Author Resource", AuthorName: "Frank"},
			},
		},
	}
	html := render(t, searchpages.HomePage(searchpages.HomeData{
		Recommendations: sections,
		AuthUser:        searchRegularUser,
	}))

	assert.Contains(t, html, "Recommended For You")
	assert.Contains(t, html, "Based on Your Interests")
	assert.Contains(t, html, "From Authors You Follow")
	assert.Contains(t, html, "Tag Affinity Resource")
	assert.Contains(t, html, "Followed Author Resource")
}

func TestHomePage_NoRecommendationsWhenLoggedOut(t *testing.T) {
	html := render(t, searchpages.HomePage(searchpages.HomeData{
		Bestsellers: []model.RankingEntry{},
		AuthUser:    nil,
	}))

	assert.NotContains(t, html, "Recommended For You")
}

func TestHomePage_ShowsRankingSections(t *testing.T) {
	html := render(t, searchpages.HomePage(searchpages.HomeData{
		Bestsellers: []model.RankingEntry{
			{Rank: 1, ResourceID: uuid.New(), Title: "Top Resource", AuthorName: "Alice", Score: 99},
		},
		NewReleases: []model.RankingEntry{
			{Rank: 1, ResourceID: uuid.New(), Title: "New Resource", AuthorName: "Bob"},
		},
		AuthUser: searchRegularUser,
	}))

	assert.Contains(t, html, "Bestsellers This Week")
	assert.Contains(t, html, "New Releases This Week")
	assert.Contains(t, html, "Top Resource")
	assert.Contains(t, html, "New Resource")
}

// ─── StrategyConfigPage ───────────────────────────────────────────────────────

func TestStrategyConfigPage_ShowsStrategies(t *testing.T) {
	cfgs := []model.RecommendationStrategyConfig{
		{
			ID:          uuid.New(),
			StrategyKey: "MostEngagedCategories",
			Label:       "Popular in Your Categories",
			SortOrder:   1,
			IsActive:    true,
		},
		{
			ID:          uuid.New(),
			StrategyKey: "SimilarTagAffinity",
			Label:       "Based on Your Interests",
			SortOrder:   3,
			IsActive:    false,
		},
	}
	html := render(t, searchpages.StrategyConfigPage(cfgs, searchAdminUser))

	assert.Contains(t, html, "Recommendation Strategies")
	assert.Contains(t, html, "MostEngagedCategories")
	assert.Contains(t, html, "Popular in Your Categories")
	assert.Contains(t, html, "SimilarTagAffinity")
	assert.Contains(t, html, "Active")
	assert.Contains(t, html, "Inactive")
	assert.Contains(t, html, "Enable")
	assert.Contains(t, html, "Disable")
}
