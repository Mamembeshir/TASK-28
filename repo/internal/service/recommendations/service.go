package recommendations

import (
	"context"
	"log"

	"github.com/eduexchange/eduexchange/internal/audit"
	"github.com/eduexchange/eduexchange/internal/model"
	gamificationrepo "github.com/eduexchange/eduexchange/internal/repository/gamification"
	"github.com/google/uuid"
)

// RecommendationService iterates active strategies from the DB config
// and returns combined results per RANK-06.
type RecommendationService struct {
	repo       gamificationrepo.GamificationRepository
	strategies map[string]RecommendationStrategy
	auditSvc   *audit.Service
}

func NewRecommendationService(
	repo gamificationrepo.GamificationRepository,
	strategies []RecommendationStrategy,
	auditSvc *audit.Service,
) *RecommendationService {
	m := make(map[string]RecommendationStrategy, len(strategies))
	for _, s := range strategies {
		m[s.Key()] = s
	}
	return &RecommendationService{repo: repo, strategies: m, auditSvc: auditSvc}
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
// actorID is the admin performing the change, recorded in the audit log.
func (s *RecommendationService) UpdateStrategyConfig(ctx context.Context, actorID uuid.UUID, cfg *model.RecommendationStrategyConfig) error {
	// Capture before-state for audit trail.
	var before map[string]interface{}
	if cfgs, err := s.repo.ListStrategyConfigs(ctx); err == nil {
		for _, c := range cfgs {
			if c.ID == cfg.ID {
				before = map[string]interface{}{
					"strategy_key": c.StrategyKey,
					"label":        c.Label,
					"sort_order":   c.SortOrder,
					"is_active":    c.IsActive,
				}
				break
			}
		}
	}

	if err := s.repo.UpdateStrategyConfig(ctx, cfg); err != nil {
		return err
	}

	if s.auditSvc != nil {
		_ = s.auditSvc.Record(ctx, audit.Entry{
			ActorID:    actorID,
			Action:     "recommendation_strategy.update",
			EntityType: "recommendation_strategy",
			EntityID:   cfg.ID,
			BeforeData: before,
			AfterData: map[string]interface{}{
				"label":      cfg.Label,
				"sort_order": cfg.SortOrder,
				"is_active":  cfg.IsActive,
			},
			Source: "recommendations",
			Reason: "admin strategy config update",
		})
	}
	return nil
}
