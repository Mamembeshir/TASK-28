package messagingrepo

import (
	"context"
	"time"

	"github.com/eduexchange/eduexchange/internal/model"
	"github.com/google/uuid"
)

// NotificationFilter holds filter parameters for listing notifications.
type NotificationFilter struct {
	EventType *model.EventType
	IsRead    *bool
	Page      int
	PageSize  int
}

// MessagingRepository defines the persistence interface for the messaging subsystem.
type MessagingRepository interface {
	// Notifications
	CreateNotification(ctx context.Context, n *model.Notification) error
	GetNotification(ctx context.Context, id uuid.UUID) (*model.Notification, error)
	ListNotifications(ctx context.Context, userID uuid.UUID, filter NotificationFilter) ([]model.Notification, int, error)
	GetUnreadCount(ctx context.Context, userID uuid.UUID) (int, error)
	MarkRead(ctx context.Context, notificationID, userID uuid.UUID) error
	BulkMarkRead(ctx context.Context, userID uuid.UUID) error

	// Subscriptions
	GetSubscription(ctx context.Context, userID uuid.UUID, eventType model.EventType) (*model.NotificationSubscription, error)
	ListSubscriptions(ctx context.Context, userID uuid.UUID) ([]model.NotificationSubscription, error)
	UpsertSubscription(ctx context.Context, sub *model.NotificationSubscription) error

	// Retry queue
	CreateRetryQueueItem(ctx context.Context, item *model.NotificationRetryQueue) error
	GetPendingRetries(ctx context.Context, now time.Time) ([]model.NotificationRetryQueue, error)
	UpdateRetryItem(ctx context.Context, item *model.NotificationRetryQueue) error
	DeleteRetryItem(ctx context.Context, id uuid.UUID) error
}
