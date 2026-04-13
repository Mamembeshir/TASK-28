package engagementrepo

import (
	"context"

	"github.com/eduexchange/eduexchange/internal/model"
	"github.com/google/uuid"
)

// EngagementRepository is the persistence interface for the engagement domain.
type EngagementRepository interface {
	// ── Votes ───────────────────────────────────────────────────────────────
	// UpsertVote creates or replaces a vote (cast/switch direction).
	UpsertVote(ctx context.Context, v *model.Vote) error
	// DeleteVote removes a vote (retract).
	DeleteVote(ctx context.Context, userID, resourceID uuid.UUID) error
	// GetVote returns the user's current vote on a resource, or ErrNotFound.
	GetVote(ctx context.Context, userID, resourceID uuid.UUID) (*model.Vote, error)
	// GetVoteCounts returns up/down totals for a resource.
	GetVoteCounts(ctx context.Context, resourceID uuid.UUID) (*model.VoteCounts, error)
	// GetMutualVoteCount returns how many mutual votes (both up+down in either direction)
	// two users have exchanged in the given hours window — used for like-ring detection.
	GetMutualVoteCount(ctx context.Context, userA, userB uuid.UUID, withinHours int) (int, error)

	// ── Favorites ───────────────────────────────────────────────────────────
	// UpsertFavorite adds a favorite; returns (true, nil) if newly added, (false, nil) if already existed.
	UpsertFavorite(ctx context.Context, f *model.Favorite) (bool, error)
	// DeleteFavorite removes a favorite.
	DeleteFavorite(ctx context.Context, userID, resourceID uuid.UUID) error
	// GetFavorite returns the favorite or ErrNotFound.
	GetFavorite(ctx context.Context, userID, resourceID uuid.UUID) (*model.Favorite, error)
	// ListFavorites returns all resources favorited by a user (paginated).
	ListFavorites(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]model.Resource, int, error)
	// CountFavoritesReceived returns how many favorites a user's resources have received in total.
	CountFavoritesReceived(ctx context.Context, authorID uuid.UUID) (int, error)

	// ── Follows ─────────────────────────────────────────────────────────────
	// UpsertFollow adds a follow; returns (true, nil) if newly followed.
	UpsertFollow(ctx context.Context, f *model.Follow) (bool, error)
	// DeleteFollow removes a follow.
	DeleteFollow(ctx context.Context, followerID uuid.UUID, targetType model.FollowTargetType, targetID uuid.UUID) error
	// GetFollow returns the follow record or ErrNotFound.
	GetFollow(ctx context.Context, followerID uuid.UUID, targetType model.FollowTargetType, targetID uuid.UUID) (*model.Follow, error)
	// ListFollowedAuthorIDs returns the author UUIDs that a user follows.
	ListFollowedAuthorIDs(ctx context.Context, followerID uuid.UUID) ([]uuid.UUID, error)
	// ListFollowerIDs returns the user UUIDs that follow a given author.
	ListFollowerIDs(ctx context.Context, authorID uuid.UUID) ([]uuid.UUID, error)

	// ── Anomaly Flags ────────────────────────────────────────────────────────
	CreateAnomalyFlag(ctx context.Context, flag *model.AnomalyFlag) error
	ListAnomalyFlags(ctx context.Context, status string) ([]model.AnomalyFlag, error)
	UpdateAnomalyFlag(ctx context.Context, flagID uuid.UUID, status string) error
	// HasOpenLikeRingFlag returns true when there is already an OPEN LIKE_RING
	// anomaly flag that names both userA and userB as involved parties.
	HasOpenLikeRingFlag(ctx context.Context, userA, userB uuid.UUID) (bool, error)
}
