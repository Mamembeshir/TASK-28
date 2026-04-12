package model

import (
	"time"

	"github.com/google/uuid"
)

// Notification represents a single notification for a user.
type Notification struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	EventType  EventType
	Title      string
	Body       string
	ResourceID *uuid.UUID
	IsRead     bool
	CreatedAt  time.Time
	ReadAt     *time.Time
}

// NotificationSubscription represents a user's subscription to a notification event type.
type NotificationSubscription struct {
	UserID    uuid.UUID
	EventType EventType
	Enabled   bool
	UpdatedAt time.Time
}

// NotificationRetryQueue represents an item in the notification retry queue.
type NotificationRetryQueue struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	EventType   EventType
	Title       string
	Body        string
	ResourceID  *uuid.UUID
	Attempts    int
	NextRetryAt time.Time
	Status      NotificationRetryStatus
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
