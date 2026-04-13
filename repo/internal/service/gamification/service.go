package gamificationservice

import (
	"context"
	"fmt"

	"github.com/eduexchange/eduexchange/internal/audit"
	"github.com/eduexchange/eduexchange/internal/model"
	gamificationrepo "github.com/eduexchange/eduexchange/internal/repository/gamification"
	"github.com/google/uuid"
)

// NotificationSender is the interface for sending notifications from the gamification service.
type NotificationSender interface {
	Send(ctx context.Context, userID uuid.UUID, eventType model.EventType, title, body string, resourceID *uuid.UUID) error
}

// PointsService handles point transactions, level calculation, and badge checking.
type PointsService struct {
	repo     gamificationrepo.GamificationRepository
	notifSvc NotificationSender
	auditSvc *audit.Service
}

func NewPointsService(repo gamificationrepo.GamificationRepository, auditSvc *audit.Service) *PointsService {
	return &PointsService{repo: repo, auditSvc: auditSvc}
}

// SetNotificationSender wires in the notification service after construction.
func (s *PointsService) SetNotificationSender(n NotificationSender) {
	s.notifSvc = n
}

// AwardPoints implements the GamificationHooks interface and the core point engine.
// It looks up the active PointRule for eventType, records the transaction,
// updates user_points, recomputes level, and checks badge thresholds.
func (s *PointsService) AwardPoints(ctx context.Context, userID uuid.UUID, eventType string, sourceID *uuid.UUID) error {
	rule, err := s.repo.GetPointRuleByEvent(ctx, eventType)
	if err == model.ErrNotFound {
		// No rule configured for this event — silently skip.
		return nil
	}
	if err != nil {
		return fmt.Errorf("getting point rule: %w", err)
	}
	if !rule.IsActive {
		return nil
	}

	// Record the transaction.
	tx := &model.PointTransaction{
		ID:         uuid.New(),
		UserID:     userID,
		Points:     rule.Points,
		Reason:     rule.Description,
		SourceType: eventType,
		SourceID:   sourceID,
	}
	if err := s.repo.CreatePointTransaction(ctx, tx); err != nil {
		return fmt.Errorf("creating point transaction: %w", err)
	}

	// Update totals.
	current, err := s.repo.GetUserPoints(ctx, userID)
	if err != nil {
		return fmt.Errorf("getting user points: %w", err)
	}

	oldLevel := current.Level
	newTotal := current.TotalPoints + rule.Points
	up := &model.UserPoints{
		UserID:      userID,
		TotalPoints: newTotal,
		Level:       model.ComputeLevel(newTotal),
	}
	if err := s.repo.UpsertUserPoints(ctx, up); err != nil {
		return fmt.Errorf("upserting user points: %w", err)
	}

	// Notify level up.
	if s.notifSvc != nil && up.Level > oldLevel {
		go func() {
			_ = s.notifSvc.Send(context.Background(), userID, model.EventLevelUp,
				fmt.Sprintf("Level Up! You reached level %d", up.Level),
				fmt.Sprintf("Congratulations! You have reached level %d with %d total points.", up.Level, up.TotalPoints),
				nil)
		}()
	}

	// Check badge thresholds after every point change.
	go func() {
		_ = s.checkAndAwardBadges(context.Background(), userID, up)
	}()

	return nil
}

// GetUserPoints returns current points and level for a user.
func (s *PointsService) GetUserPoints(ctx context.Context, userID uuid.UUID) (*model.UserPoints, error) {
	return s.repo.GetUserPoints(ctx, userID)
}

// ListPointTransactions returns the point history for a user.
func (s *PointsService) ListPointTransactions(ctx context.Context, userID uuid.UUID, limit int) ([]model.PointTransaction, error) {
	return s.repo.ListPointTransactions(ctx, userID, limit)
}

// ListPointRules returns all configured point rules.
func (s *PointsService) ListPointRules(ctx context.Context) ([]model.PointRule, error) {
	return s.repo.ListPointRules(ctx)
}

// UpsertPointRule creates or updates a point rule (Admin only).
func (s *PointsService) UpsertPointRule(ctx context.Context, rule *model.PointRule) error {
	if rule.ID == uuid.Nil {
		rule.ID = uuid.New()
	}
	return s.repo.UpsertPointRule(ctx, rule)
}

// UpdatePointRule updates an existing point rule by ID (Admin only).
// actorID is the admin performing the change, recorded in the audit log.
func (s *PointsService) UpdatePointRule(ctx context.Context, actorID uuid.UUID, rule *model.PointRule) error {
	// Capture before-state for audit trail.
	var before map[string]interface{}
	if rules, err := s.repo.ListPointRules(ctx); err == nil {
		for _, r := range rules {
			if r.ID == rule.ID {
				before = map[string]interface{}{
					"event_type":  r.EventType,
					"points":      r.Points,
					"description": r.Description,
					"is_active":   r.IsActive,
				}
				break
			}
		}
	}

	if err := s.repo.UpdatePointRuleByID(ctx, rule); err != nil {
		return err
	}

	if s.auditSvc != nil {
		_ = s.auditSvc.Record(ctx, audit.Entry{
			ActorID:    actorID,
			Action:     "point_rule.update",
			EntityType: "point_rule",
			EntityID:   rule.ID,
			BeforeData: before,
			AfterData: map[string]interface{}{
				"points":      rule.Points,
				"description": rule.Description,
				"is_active":   rule.IsActive,
			},
			Source: "gamification",
			Reason: "admin point rule update",
		})
	}
	return nil
}

// ── Badge Checking ────────────────────────────────────────────────────────────

func (s *PointsService) checkAndAwardBadges(ctx context.Context, userID uuid.UUID, up *model.UserPoints) error {
	badges, err := s.repo.ListBadges(ctx)
	if err != nil {
		return err
	}

	for _, badge := range badges {
		// Check if already awarded.
		_, err := s.repo.GetUserBadge(ctx, userID, badge.ID)
		if err == nil {
			continue // already has it
		}

		met, err := s.thresholdMet(ctx, userID, badge, up)
		if err != nil || !met {
			continue
		}

		ub := &model.UserBadge{
			ID:      uuid.New(),
			UserID:  userID,
			BadgeID: badge.ID,
		}
		if err := s.repo.CreateUserBadge(ctx, ub); err == nil && s.notifSvc != nil {
			_ = s.notifSvc.Send(ctx, userID, model.EventBadgeEarned,
				fmt.Sprintf("Badge Earned: %s", badge.Name),
				badge.Description,
				nil)
		}
	}
	return nil
}

func (s *PointsService) thresholdMet(ctx context.Context, userID uuid.UUID, badge model.Badge, up *model.UserPoints) (bool, error) {
	switch badge.ThresholdType {
	case "TOTAL_POINTS":
		return up.TotalPoints >= badge.ThresholdValue, nil

	case "FAVORITES_RECEIVED":
		count, err := s.repo.CountUpvotesReceived(ctx, userID)
		// Re-using upvotes count fn isn't right; we need favorites received.
		// The repo stores favorites count separately. For now use a direct check.
		_ = count
		// We'll use a dedicated query; fall through for now.
		favCount := 0
		// The gamification repo has CountUpvotesReceived but not CountFavoritesReceived.
		// We query via the engagement side through a combined approach:
		// Use CountEntriesApproved as a proxy for entries; use upvotes for upvotes.
		// For favorites, we check point transactions summing FAVORITE_RECEIVED events.
		txs, err := s.repo.ListPointTransactions(ctx, userID, 10000)
		if err != nil {
			return false, err
		}
		for _, tx := range txs {
			if tx.SourceType == model.EventFavoriteReceived {
				favCount++
			}
		}
		return favCount >= badge.ThresholdValue, nil

	case "ENTRIES_APPROVED":
		count, err := s.repo.CountEntriesApproved(ctx, userID)
		if err != nil {
			return false, err
		}
		return count >= badge.ThresholdValue, nil

	case "UPVOTES_RECEIVED":
		count, err := s.repo.CountUpvotesReceived(ctx, userID)
		if err != nil {
			return false, err
		}
		return count >= badge.ThresholdValue, nil
	}
	return false, nil
}

// ListUserBadges returns all badges earned by a user.
func (s *PointsService) ListUserBadges(ctx context.Context, userID uuid.UUID) ([]model.UserBadge, error) {
	return s.repo.ListUserBadges(ctx, userID)
}

// GetLeaderboard returns top-N users by total points.
func (s *PointsService) GetLeaderboard(ctx context.Context, limit int) ([]model.LeaderboardEntry, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return s.repo.GetLeaderboard(ctx, limit)
}

