package messagingservice

import (
	"context"
	"encoding/json"
	"time"

	"github.com/eduexchange/eduexchange/internal/model"
	messagingrepo "github.com/eduexchange/eduexchange/internal/repository/messaging"
	"github.com/eduexchange/eduexchange/internal/sse"
	"github.com/google/uuid"
)

// retryIntervals defines the backoff intervals indexed by attempt count (after failure).
var retryIntervals = []time.Duration{
	1 * time.Minute,
	2 * time.Minute,
	4 * time.Minute,
	8 * time.Minute,
	15 * time.Minute,
}

// NotificationService handles notification creation, SSE pushing, and subscription management.
type NotificationService struct {
	repo   messagingrepo.MessagingRepository
	sseHub *sse.Hub
}

// NewNotificationService creates a new NotificationService.
func NewNotificationService(repo messagingrepo.MessagingRepository, hub *sse.Hub) *NotificationService {
	return &NotificationService{repo: repo, sseHub: hub}
}

// Send creates a notification if the user is subscribed, writes to DB, and pushes via SSE.
// If DB write fails, enqueues for retry.
func (s *NotificationService) Send(ctx context.Context, userID uuid.UUID, eventType model.EventType, title, body string, resourceID *uuid.UUID) error {
	// Check subscription — if explicitly disabled, skip
	sub, err := s.repo.GetSubscription(ctx, userID, eventType)
	if err == nil && !sub.Enabled {
		return nil
	}
	// If ErrNotFound, default is enabled — continue

	n := &model.Notification{
		ID:         uuid.New(),
		UserID:     userID,
		EventType:  eventType,
		Title:      title,
		Body:       body,
		ResourceID: resourceID,
	}

	if dbErr := s.repo.CreateNotification(ctx, n); dbErr != nil {
		// Enqueue for retry
		retryItem := &model.NotificationRetryQueue{
			ID:          uuid.New(),
			UserID:      userID,
			EventType:   eventType,
			Title:       title,
			Body:        body,
			ResourceID:  resourceID,
			Attempts:    0,
			NextRetryAt: time.Now().Add(retryIntervals[0]),
			Status:      model.NotificationRetryPending,
		}
		_ = s.repo.CreateRetryQueueItem(ctx, retryItem)
		return dbErr
	}

	// Push via SSE
	type ssePayload struct {
		ID        string `json:"id"`
		EventType string `json:"event_type"`
		Title     string `json:"title"`
		Body      string `json:"body"`
	}
	payload, _ := json.Marshal(ssePayload{
		ID:        n.ID.String(),
		EventType: string(eventType),
		Title:     title,
		Body:      body,
	})
	s.sseHub.SendToUser(userID, sse.Event{
		Type: "notification",
		Data: string(payload),
	})

	return nil
}

// GetUnread returns unread notifications and count for a user.
func (s *NotificationService) GetUnread(ctx context.Context, userID uuid.UUID) ([]model.Notification, int, error) {
	isRead := false
	filter := messagingrepo.NotificationFilter{
		IsRead:   &isRead,
		Page:     1,
		PageSize: 50,
	}
	return s.repo.ListNotifications(ctx, userID, filter)
}

// GetAll returns paginated notifications with filters.
func (s *NotificationService) GetAll(ctx context.Context, userID uuid.UUID, filter messagingrepo.NotificationFilter) ([]model.Notification, int, error) {
	return s.repo.ListNotifications(ctx, userID, filter)
}

// MarkRead marks one notification as read.
func (s *NotificationService) MarkRead(ctx context.Context, notificationID, userID uuid.UUID) error {
	return s.repo.MarkRead(ctx, notificationID, userID)
}

// BulkMarkRead marks all notifications for a user as read.
func (s *NotificationService) BulkMarkRead(ctx context.Context, userID uuid.UUID) error {
	return s.repo.BulkMarkRead(ctx, userID)
}

// ManageSubscription enables or disables a notification event type for a user.
func (s *NotificationService) ManageSubscription(ctx context.Context, userID uuid.UUID, eventType model.EventType, enabled bool) error {
	sub := &model.NotificationSubscription{
		UserID:    userID,
		EventType: eventType,
		Enabled:   enabled,
	}
	return s.repo.UpsertSubscription(ctx, sub)
}

// ListSubscriptions returns all subscriptions for a user, with defaults (enabled=true) for missing event types.
func (s *NotificationService) ListSubscriptions(ctx context.Context, userID uuid.UUID) ([]model.NotificationSubscription, error) {
	existing, err := s.repo.ListSubscriptions(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Build a lookup map
	lookup := make(map[model.EventType]model.NotificationSubscription, len(existing))
	for _, sub := range existing {
		lookup[sub.EventType] = sub
	}

	allEventTypes := []model.EventType{
		model.EventEntryDeadline,
		model.EventReviewDecision,
		model.EventPublishComplete,
		model.EventSupplierShipment,
		model.EventSupplierQC,
		model.EventAnomalyAlert,
		model.EventBanNotice,
		model.EventReportUpdate,
		model.EventBadgeEarned,
		model.EventLevelUp,
		model.EventFollowNewContent,
	}

	result := make([]model.NotificationSubscription, 0, len(allEventTypes))
	for _, et := range allEventTypes {
		if sub, ok := lookup[et]; ok {
			result = append(result, sub)
		} else {
			// Default: enabled
			result = append(result, model.NotificationSubscription{
				UserID:    userID,
				EventType: et,
				Enabled:   true,
				UpdatedAt: time.Time{},
			})
		}
	}
	return result, nil
}

// ── RetryService ──────────────────────────────────────────────────────────────

// RetryService processes the notification retry queue.
type RetryService struct {
	repo messagingrepo.MessagingRepository
}

// NewRetryService creates a new RetryService.
func NewRetryService(repo messagingrepo.MessagingRepository) *RetryService {
	return &RetryService{repo: repo}
}

// ProcessRetryQueue finds PENDING items due for retry, attempts DB write, and updates status.
func (s *RetryService) ProcessRetryQueue(ctx context.Context) error {
	items, err := s.repo.GetPendingRetries(ctx, time.Now())
	if err != nil {
		return err
	}

	for i := range items {
		item := &items[i]

		n := &model.Notification{
			ID:         uuid.New(),
			UserID:     item.UserID,
			EventType:  item.EventType,
			Title:      item.Title,
			Body:       item.Body,
			ResourceID: item.ResourceID,
		}

		if createErr := s.repo.CreateNotification(ctx, n); createErr == nil {
			// Success: remove from retry queue
			_ = s.repo.DeleteRetryItem(ctx, item.ID)
		} else {
			// Failure: increment attempts
			item.Attempts++
			if item.Attempts >= 5 {
				item.Status = model.NotificationRetryFailed
			} else {
				item.NextRetryAt = time.Now().Add(retryIntervals[item.Attempts])
			}
			_ = s.repo.UpdateRetryItem(ctx, item)
		}
	}
	return nil
}
