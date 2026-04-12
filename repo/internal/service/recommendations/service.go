package recommendations

import (
	"context"
	"log"

	"github.com/eduexchange/eduexchange/internal/model"
	gamificationrepo "github.com/eduexchange/eduexchange/internal/repository/gamification"
	"github.com/google/uuid"
)

// RecommendationService iterates active strategies from the DB config
// and returns combined results per RANK-06.
type RecommendationService struct {
	repo       gamificationrepo.GamificationRepository
	strategies map[string]RecommendationStrategy
}

func NewRecommendationService(
	repo gamificationrepo.GamificationRepository,
	strategies []RecommendationStrategy,
) *RecommendationService {
	m := make(map[string]RecommendationStrategy, len(strategies))
	for _, s := range strategies {
		m[s.Key()] = s
	}
	return &RecommendationService{repo: repo, strategies: m}
}

const perStrategyLimit = 10

// GetRecommendations returns one section per active strategy, ordered by sort_order.
func (s *RecommendationService) GetRecommendations(ctx context.Context, userID uuid.UUID) ([]model.RecommendationSection, error) {
	configs, err := s.repo.ListStrategyConfigs(ctx)
	if err != nil {
		return nil, err
	}

	var sections []model.RecommendationSection
	for _, cfg := range configs {
		if !cfg.IsActive {
			continue
		}
		strategy, ok := s.strategies[cfg.StrategyKey]
		if !ok {
			log.Printf("RecommendationService: unknown strategy key %q", cfg.StrategyKey)
			continue
		}
		resources, err := strategy.Recommend(ctx, userID, perStrategyLimit)
		if err != nil {
			log.Printf("RecommendationService: strategy %q error: %v", cfg.StrategyKey, err)
			continue
		}
		sections = append(sections, model.RecommendationSection{
			Label:     cfg.Label,
			Strategy:  cfg.StrategyKey,
			Resources: resources,
		})
	}
	return sections, nil
}

// ListStrategyConfigs returns all strategy config entries for Admin management.
func (s *RecommendationService) ListStrategyConfigs(ctx context.Context) ([]model.RecommendationStrategyConfig, error) {
	return s.repo.ListStrategyConfigs(ctx)
}

// UpdateStrategyConfig updates the label, active state, and sort order.
func (s *RecommendationService) UpdateStrategyConfig(ctx context.Context, cfg *model.RecommendationStrategyConfig) error {
	return s.repo.UpdateStrategyConfig(ctx, cfg)
}
