package frontend_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/eduexchange/eduexchange/internal/middleware"
	"github.com/eduexchange/eduexchange/internal/model"
	engagementpages "github.com/eduexchange/eduexchange/internal/templ/pages/engagement"
	gamificationpages "github.com/eduexchange/eduexchange/internal/templ/pages/gamification"
)

var (
	gamTestUser = &middleware.AuthUser{
		ID:       uuid.New(),
		Username: "gamer",
		Roles:    []string{"REGULAR_USER"},
	}
	gamAdminUser = &middleware.AuthUser{
		ID:       uuid.New(),
		Username: "admin",
		Roles:    []string{"ADMIN"},
	}
)

// ─── VoteButtons ──────────────────────────────────────────────────────────────

func TestVoteButtons_RendersUpDownButtons(t *testing.T) {
	rid := uuid.New()
	d := engagementpages.VoteData{
		ResourceID: rid,
		Counts:     &model.VoteCounts{ResourceID: rid, Upvotes: 5, Downvotes: 2, Score: 3},
	}
	html := render(t, engagementpages.VoteButtons(d))

	assert.Contains(t, html, "hx-post")
	assert.Contains(t, html, "vote_type")
	assert.Contains(t, html, "UP")
	assert.Contains(t, html, "DOWN")
	assert.Contains(t, html, "5")
	assert.Contains(t, html, "2")
}

func TestVoteButtons_HighlightsActiveUpvote(t *testing.T) {
	rid := uuid.New()
	d := engagementpages.VoteData{
		ResourceID: rid,
		Counts:     &model.VoteCounts{ResourceID: rid, Upvotes: 3, Downvotes: 0, Score: 3},
		UserVote:   &model.Vote{UserID: uuid.New(), ResourceID: rid, VoteType: model.VoteTypeUp},
	}
	html := render(t, engagementpages.VoteButtons(d))

	assert.Contains(t, html, "bg-green-100")
}

func TestVoteButtons_HighlightsActiveDownvote(t *testing.T) {
	rid := uuid.New()
	d := engagementpages.VoteData{
		ResourceID: rid,
		Counts:     &model.VoteCounts{ResourceID: rid, Upvotes: 0, Downvotes: 1, Score: -1},
		UserVote:   &model.Vote{UserID: uuid.New(), ResourceID: rid, VoteType: model.VoteTypeDown},
	}
	html := render(t, engagementpages.VoteButtons(d))

	assert.Contains(t, html, "bg-red-100")
}

func TestVoteButtons_NoVoteNeutralStyle(t *testing.T) {
	rid := uuid.New()
	d := engagementpages.VoteData{
		ResourceID: rid,
		Counts:     &model.VoteCounts{ResourceID: rid},
	}
	html := render(t, engagementpages.VoteButtons(d))

	assert.NotContains(t, html, "bg-green-100")
	assert.NotContains(t, html, "bg-red-100")
}

// ─── FavoriteButton ───────────────────────────────────────────────────────────

func TestFavoriteButton_NotFavorited(t *testing.T) {
	rid := uuid.New()
	d := engagementpages.FavoriteData{ResourceID: rid, IsFavorited: false}
	html := render(t, engagementpages.FavoriteButton(d))

	assert.Contains(t, html, "hx-post")
	assert.Contains(t, html, "favorite")
	assert.Contains(t, html, "text-gray-400")
}

func TestFavoriteButton_Favorited(t *testing.T) {
	rid := uuid.New()
	d := engagementpages.FavoriteData{ResourceID: rid, IsFavorited: true}
	html := render(t, engagementpages.FavoriteButton(d))

	assert.Contains(t, html, "hx-delete")
	assert.Contains(t, html, "text-red-500")
}

// ─── FollowButton ─────────────────────────────────────────────────────────────

func TestFollowButton_NotFollowed(t *testing.T) {
	d := engagementpages.FollowData{
		TargetID:   uuid.New(),
		TargetType: model.FollowTargetAuthor,
		IsFollowed: false,
	}
	html := render(t, engagementpages.FollowButton(d))

	assert.Contains(t, html, "hx-post")
	assert.Contains(t, html, "Follow")
	assert.Contains(t, html, "bg-blue-600")
}

func TestFollowButton_Followed(t *testing.T) {
	d := engagementpages.FollowData{
		TargetID:   uuid.New(),
		TargetType: model.FollowTargetAuthor,
		IsFollowed: true,
	}
	html := render(t, engagementpages.FollowButton(d))

	assert.Contains(t, html, "hx-delete")
	assert.Contains(t, html, "Following")
}

// ─── UserStatsPage ────────────────────────────────────────────────────────────

func TestUserStatsPage_ShowsPointsAndLevel(t *testing.T) {
	uid := uuid.New()
	up := &model.UserPoints{UserID: uid, TotalPoints: 450, Level: 2}
	d := gamificationpages.UserStatsData{
		Points:   up,
		Badges:   nil,
		AuthUser: gamTestUser,
	}
	html := render(t, gamificationpages.UserStatsPage(d))

	assert.Contains(t, html, "450")
	assert.Contains(t, html, "My Stats")
	assert.Contains(t, html, "Level")
}

func TestUserStatsPage_ShowsBadges(t *testing.T) {
	uid := uuid.New()
	badgeID := uuid.New()
	badge := model.Badge{
		ID:   badgeID,
		Code: "POPULAR_50",
		Name: "Popular Author",
	}
	ub := model.UserBadge{
		ID:        uuid.New(),
		UserID:    uid,
		BadgeID:   badgeID,
		AwardedAt: time.Now(),
		Badge:     &badge,
	}
	d := gamificationpages.UserStatsData{
		Points:   &model.UserPoints{UserID: uid, TotalPoints: 600, Level: 3},
		Badges:   []model.UserBadge{ub},
		AuthUser: gamTestUser,
	}
	html := render(t, gamificationpages.UserStatsPage(d))

	assert.Contains(t, html, "Popular Author")
	assert.Contains(t, html, "Badges")
}

func TestUserStatsCard_NoBadgesShowsPrompt(t *testing.T) {
	d := gamificationpages.UserStatsData{
		Points:   &model.UserPoints{TotalPoints: 0, Level: 0},
		Badges:   nil,
		AuthUser: gamTestUser,
	}
	html := render(t, gamificationpages.UserStatsCard(d))

	assert.Contains(t, html, "No badges earned yet")
}

// ─── LeaderboardPage ─────────────────────────────────────────────────────────

func TestLeaderboardPage_ShowsEntries(t *testing.T) {
	entries := []model.LeaderboardEntry{
		{Rank: 1, UserID: uuid.New(), Username: "alice", TotalPoints: 1000, Level: 5},
		{Rank: 2, UserID: uuid.New(), Username: "bob", TotalPoints: 800, Level: 4},
		{Rank: 3, UserID: uuid.New(), Username: "charlie", TotalPoints: 600, Level: 3},
	}
	d := gamificationpages.LeaderboardData{Entries: entries, AuthUser: gamTestUser}
	html := render(t, gamificationpages.LeaderboardPage(d))

	assert.Contains(t, html, "Leaderboard")
	assert.Contains(t, html, "alice")
	assert.Contains(t, html, "bob")
	assert.Contains(t, html, "charlie")
	assert.Contains(t, html, "1000")
	assert.Contains(t, html, "800")
}

func TestLeaderboardTable_EmptyState(t *testing.T) {
	html := render(t, gamificationpages.LeaderboardTable(nil))

	assert.Contains(t, html, "No entries yet")
}

func TestLeaderboardTable_Top3GetMedals(t *testing.T) {
	entries := []model.LeaderboardEntry{
		{Rank: 1, Username: "gold", TotalPoints: 300},
		{Rank: 2, Username: "silver", TotalPoints: 200},
		{Rank: 3, Username: "bronze", TotalPoints: 100},
		{Rank: 4, Username: "fourth", TotalPoints: 50},
	}
	html := render(t, gamificationpages.LeaderboardTable(entries))

	assert.Contains(t, html, "bg-yellow-50")
	assert.Contains(t, html, "bg-gray-50")
	assert.Contains(t, html, "bg-orange-50")
	assert.Contains(t, html, "#4")
}

// ─── PointRulesPage ──────────────────────────────────────────────────────────

func TestPointRulesPage_ShowsRules(t *testing.T) {
	rules := []model.PointRule{
		{
			ID:          uuid.New(),
			EventType:   "ENTRY_APPROVED",
			Points:      50,
			Description: "Resource approved",
			IsActive:    true,
		},
		{
			ID:          uuid.New(),
			EventType:   "UPVOTE_RECEIVED",
			Points:      5,
			Description: "Got an upvote",
			IsActive:    true,
		},
	}
	d := gamificationpages.PointRulesData{Rules: rules, AuthUser: gamAdminUser}
	html := render(t, gamificationpages.PointRulesPage(d))

	assert.Contains(t, html, "Point Rules")
	assert.Contains(t, html, "ENTRY_APPROVED")
	assert.Contains(t, html, "UPVOTE_RECEIVED")
	assert.Contains(t, html, "50")
	assert.Contains(t, html, "hx-put")
}

func TestPointRulesTable_EmptyState(t *testing.T) {
	html := render(t, gamificationpages.PointRulesTable(nil))

	assert.Contains(t, html, "No point rules configured")
}
