package recommendations

import (
	"context"

	"github.com/eduexchange/eduexchange/internal/model"
	"github.com/google/uuid"
)

// RecommendationStrategy is the pluggable interface per RANK-05.
type RecommendationStrategy interface {
	// Key returns the unique identifier matching recommendation_strategy_config.strategy_key.
	Key() string
	// Recommend returns up to limit resources for the given user.
	Recommend(ctx context.Context, userID uuid.UUID, limit int) ([]model.Resource, error)
}
