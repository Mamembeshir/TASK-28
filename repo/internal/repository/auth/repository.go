package authrepo

import (
	"context"

	"github.com/eduexchange/eduexchange/internal/model"
	"github.com/google/uuid"
)

// ListFilter controls pagination and filtering for ListUsers.
type ListFilter struct {
	Page     int
	PageSize int
	Status   string // empty = all
	Role     string // empty = all
	Search   string // partial match on username/email
}

// UserRepository defines all persistence operations needed by auth and user management.
type UserRepository interface {
	// User
	Create(ctx context.Context, u *model.User, profile *model.UserProfile) error
	FindByID(ctx context.Context, id uuid.UUID) (*model.User, error)
	FindByUsername(ctx context.Context, username string) (*model.User, error)
	FindByEmail(ctx context.Context, email string) (*model.User, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status model.UserStatus, version int) error
	UpdatePassword(ctx context.Context, id uuid.UUID, hash string) error
	RecordFailedLogin(ctx context.Context, id uuid.UUID) error
	ClearLockout(ctx context.Context, id uuid.UUID) error
	ListUsers(ctx context.Context, f ListFilter) ([]model.UserWithRoles, int, error)

	// Roles
	GetRoles(ctx context.Context, userID uuid.UUID) ([]model.Role, error)
	AssignRole(ctx context.Context, userID uuid.UUID, role model.Role) error
	RemoveRole(ctx context.Context, userID uuid.UUID, role model.Role) error

	// Sessions
	CreateSession(ctx context.Context, s *model.Session) error
	FindSession(ctx context.Context, token string) (*model.Session, error)
	DeleteSession(ctx context.Context, token string) error

	// Profile
	GetProfile(ctx context.Context, userID uuid.UUID) (*model.UserProfile, error)
}
