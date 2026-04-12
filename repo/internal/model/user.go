package model

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID               uuid.UUID
	Username         string
	Email            string
	PasswordHash     string
	Status           UserStatus
	FailedLoginCount int
	LockedUntil      *time.Time
	Version          int
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type UserProfile struct {
	UserID      uuid.UUID
	DisplayName string
	Bio         string
	AvatarPath  string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type UserRoleEntry struct {
	UserID    uuid.UUID
	Role      Role
	CreatedAt time.Time
}

type Session struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Token     string
	ExpiresAt time.Time
	CreatedAt time.Time
}

// UserWithRoles is a convenience type for views that need roles alongside user data.
type UserWithRoles struct {
	User    User
	Profile UserProfile
	Roles   []Role
}

// IsLocked returns true if the user account is currently locked due to failed logins.
func (u *User) IsLocked() bool {
	return u.LockedUntil != nil && u.LockedUntil.After(time.Now().UTC())
}

// HasRole returns true if the user holds the given role.
func (uwr *UserWithRoles) HasRole(r Role) bool {
	for _, role := range uwr.Roles {
		if role == r {
			return true
		}
	}
	return false
}
