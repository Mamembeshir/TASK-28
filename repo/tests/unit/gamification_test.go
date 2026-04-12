package unit_test

import (
	"testing"

	"github.com/eduexchange/eduexchange/internal/model"
	"github.com/stretchr/testify/assert"
)

// ── Level Calculation ─────────────────────────────────────────────────────────

func TestComputeLevel_Zero(t *testing.T) {
	assert.Equal(t, 0, model.ComputeLevel(0))
}

func TestComputeLevel_Negative(t *testing.T) {
	assert.Equal(t, 0, model.ComputeLevel(-50))
}

func TestComputeLevel_BelowFirstThreshold(t *testing.T) {
	assert.Equal(t, 0, model.ComputeLevel(199))
}

func TestComputeLevel_ExactFirstLevel(t *testing.T) {
	assert.Equal(t, 1, model.ComputeLevel(200))
}

func TestComputeLevel_Level5(t *testing.T) {
	assert.Equal(t, 5, model.ComputeLevel(1000))
}

func TestComputeLevel_Level5PlusOne(t *testing.T) {
	assert.Equal(t, 5, model.ComputeLevel(1001))
}

func TestComputeLevel_Level10(t *testing.T) {
	assert.Equal(t, 10, model.ComputeLevel(2000))
}

// ── Point Rule Constants ──────────────────────────────────────────────────────

func TestPointRuleConstants(t *testing.T) {
	assert.Equal(t, "ENTRY_APPROVED", model.EventEntryApproved)
	assert.Equal(t, "UPVOTE_RECEIVED", model.EventUpvoteReceived)
	assert.Equal(t, "DOWNVOTE_RECEIVED", model.EventDownvoteReceived)
	assert.Equal(t, "FAVORITE_RECEIVED", model.EventFavoriteReceived)
	assert.Equal(t, "TAKEDOWN_PENALTY", model.EventTakedownPenalty)
}

// ── VoteType ──────────────────────────────────────────────────────────────────

func TestVoteType_Values(t *testing.T) {
	assert.Equal(t, model.VoteType("UP"), model.VoteTypeUp)
	assert.Equal(t, model.VoteType("DOWN"), model.VoteTypeDown)
}

// ── FollowTargetType ──────────────────────────────────────────────────────────

func TestFollowTargetType_Values(t *testing.T) {
	assert.Equal(t, model.FollowTargetType("AUTHOR"), model.FollowTargetAuthor)
	assert.Equal(t, model.FollowTargetType("TOPIC"), model.FollowTargetTopic)
}

// ── Badge Threshold Types ─────────────────────────────────────────────────────

func TestBadge_ThresholdTypes(t *testing.T) {
	badge := model.Badge{
		Code:           "POPULAR_50",
		ThresholdType:  "FAVORITES_RECEIVED",
		ThresholdValue: 50,
	}
	assert.Equal(t, 50, badge.ThresholdValue)
	assert.Equal(t, "FAVORITES_RECEIVED", badge.ThresholdType)
}

// ── VoteCounts Score ──────────────────────────────────────────────────────────

func TestVoteCounts_Score(t *testing.T) {
	vc := model.VoteCounts{
		Upvotes:   10,
		Downvotes: 3,
	}
	// Score is computed during DB scan; test struct semantics here.
	vc.Score = vc.Upvotes - vc.Downvotes
	assert.Equal(t, 7, vc.Score)
}

func TestVoteCounts_NegativeScore(t *testing.T) {
	vc := model.VoteCounts{
		Upvotes:   2,
		Downvotes: 8,
	}
	vc.Score = vc.Upvotes - vc.Downvotes
	assert.Equal(t, -6, vc.Score)
}

// ── RecommendationSection ─────────────────────────────────────────────────────

func TestRecommendationSection_HasLabel(t *testing.T) {
	s := model.RecommendationSection{
		Label:    "Based on Your Interests",
		Strategy: "SimilarTagAffinity",
		Resources: []model.Resource{
			{Title: "Go Programming"},
		},
	}
	assert.Equal(t, "Based on Your Interests", s.Label)
	assert.Equal(t, "SimilarTagAffinity", s.Strategy)
	assert.Len(t, s.Resources, 1)
}
