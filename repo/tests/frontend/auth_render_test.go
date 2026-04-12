package frontend_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eduexchange/eduexchange/internal/model"
	authrepo "github.com/eduexchange/eduexchange/internal/repository/auth"
	adminpages "github.com/eduexchange/eduexchange/internal/templ/pages/admin"
	authpages "github.com/eduexchange/eduexchange/internal/templ/pages/auth"
	"github.com/google/uuid"
)

func render(t *testing.T, c interface {
	Render(context.Context, io.Writer) error
}) string {
	t.Helper()
	var buf bytes.Buffer
	err := c.Render(context.Background(), &buf)
	require.NoError(t, err)
	return buf.String()
}

// ─── Login Page ───────────────────────────────────────────────────────────────

func TestLoginPage_RendersForm(t *testing.T) {
	html := render(t, authpages.LoginPage(authpages.LoginData{}))

	assert.Contains(t, html, `name="username"`)
	assert.Contains(t, html, `name="password"`)
	assert.Contains(t, html, `action="/login"`)
	assert.Contains(t, html, "Sign in to your account")
}

func TestLoginPage_ShowsError(t *testing.T) {
	html := render(t, authpages.LoginPage(authpages.LoginData{
		Error: "Invalid username or password.",
	}))

	assert.Contains(t, html, "Invalid username or password.")
}

func TestLoginPage_ShowsRegisteredBanner(t *testing.T) {
	html := render(t, authpages.LoginPage(authpages.LoginData{Registered: true}))
	assert.Contains(t, html, "Registration successful")
}

func TestLoginPage_PreFillsUsername(t *testing.T) {
	html := render(t, authpages.LoginPage(authpages.LoginData{Username: "testuser"}))
	assert.Contains(t, html, `value="testuser"`)
}

// ─── Register Page ────────────────────────────────────────────────────────────

func TestRegisterPage_RendersForm(t *testing.T) {
	html := render(t, authpages.RegisterPage(authpages.RegisterData{}))

	assert.Contains(t, html, `name="username"`)
	assert.Contains(t, html, `name="email"`)
	assert.Contains(t, html, `name="password"`)
	assert.Contains(t, html, `action="/register"`)
	assert.Contains(t, html, "Create your account")
}

func TestRegisterPage_ShowsFieldErrors(t *testing.T) {
	html := render(t, authpages.RegisterPage(authpages.RegisterData{
		FieldErrors: map[string]string{
			"username": "Username is already taken.",
			"email":    "Email address is already registered.",
		},
	}))

	assert.Contains(t, html, "Username is already taken.")
	assert.Contains(t, html, "Email address is already registered.")
	// Error inputs should use error class
	assert.Contains(t, html, "ee-input--error")
}

func TestRegisterPage_NoErrors_NoErrorClass(t *testing.T) {
	html := render(t, authpages.RegisterPage(authpages.RegisterData{}))
	assert.NotContains(t, html, "ee-input--error")
}

// ─── User List Page ───────────────────────────────────────────────────────────

func TestUserListPage_RendersTable(t *testing.T) {
	users := []model.UserWithRoles{
		{
			User:    model.User{ID: uuid.New(), Username: "alice", Email: "alice@example.com", Status: model.UserStatusActive, Version: 1},
			Profile: model.UserProfile{DisplayName: "Alice"},
			Roles:   []model.Role{model.RoleRegularUser},
		},
		{
			User:    model.User{ID: uuid.New(), Username: "bob", Email: "bob@example.com", Status: model.UserStatusSuspended, Version: 2},
			Profile: model.UserProfile{DisplayName: "Bob"},
			Roles:   []model.Role{model.RoleAuthor},
		},
	}

	html := render(t, adminpages.UserListPage(adminpages.UserListData{
		Users:    users,
		Total:    2,
		Page:     1,
		PageSize: 20,
		Filter:   authrepo.ListFilter{},
		Username: "admin",
	}))

	assert.Contains(t, html, "alice")
	assert.Contains(t, html, "bob")
	assert.Contains(t, html, "alice@example.com")
	// Status badges
	assert.Contains(t, html, "ACTIVE")
	assert.Contains(t, html, "SUSPENDED")
	// Role labels
	assert.Contains(t, html, "REGULAR_USER")
	assert.Contains(t, html, "AUTHOR")
}

func TestUserListPage_EmptyState(t *testing.T) {
	html := render(t, adminpages.UserListPage(adminpages.UserListData{
		Users:    nil,
		Total:    0,
		Page:     1,
		PageSize: 20,
		Username: "admin",
	}))

	assert.Contains(t, html, "No users found")
}

func TestUserListPage_FilterFormPresent(t *testing.T) {
	html := render(t, adminpages.UserListPage(adminpages.UserListData{
		Users: []model.UserWithRoles{},
		Page:  1, PageSize: 20, Username: "admin",
	}))

	assert.Contains(t, html, `name="search"`)
	assert.Contains(t, html, `name="status"`)
	assert.Contains(t, html, `name="role"`)
}

// ─── User Detail Page ─────────────────────────────────────────────────────────

func TestUserDetailPage_ShowsUserInfo(t *testing.T) {
	uid := uuid.New()
	html := render(t, adminpages.UserDetailPage(adminpages.UserDetailData{
		UserWithRoles: model.UserWithRoles{
			User: model.User{
				ID:       uid,
				Username: "charlie",
				Email:    "charlie@example.com",
				Status:   model.UserStatusActive,
				Version:  1,
			},
			Profile: model.UserProfile{DisplayName: "Charlie C."},
			Roles:   []model.Role{model.RoleRegularUser, model.RoleAuthor},
		},
		Username: "admin",
	}))

	assert.Contains(t, html, "charlie")
	assert.Contains(t, html, "charlie@example.com")
	assert.Contains(t, html, "Charlie C.")
	assert.Contains(t, html, "ACTIVE")
}

func TestUserDetailPage_ShowsAllowedStatusButtons(t *testing.T) {
	uid := uuid.New()
	html := render(t, adminpages.UserDetailPage(adminpages.UserDetailData{
		UserWithRoles: model.UserWithRoles{
			User:    model.User{ID: uid, Username: "dave", Status: model.UserStatusActive, Version: 1},
			Profile: model.UserProfile{},
			Roles:   []model.Role{model.RoleRegularUser},
		},
		Username: "admin",
	}))

	// ACTIVE can go to SUSPENDED, BANNED, DEACTIVATED
	assert.Contains(t, html, "SUSPENDED")
	assert.Contains(t, html, "BANNED")
	assert.Contains(t, html, "DEACTIVATED")
}

func TestUserDetailPage_TerminalStatus_NoActions(t *testing.T) {
	uid := uuid.New()
	html := render(t, adminpages.UserDetailPage(adminpages.UserDetailData{
		UserWithRoles: model.UserWithRoles{
			User:    model.User{ID: uid, Username: "eve", Status: model.UserStatusDeactivated, Version: 3},
			Profile: model.UserProfile{},
			Roles:   []model.Role{model.RoleRegularUser},
		},
		Username: "admin",
	}))

	assert.Contains(t, html, "terminal")
}

func TestUserDetailPage_ShowsRoleToggleButtons(t *testing.T) {
	uid := uuid.New()
	html := render(t, adminpages.UserDetailPage(adminpages.UserDetailData{
		UserWithRoles: model.UserWithRoles{
			User:    model.User{ID: uid, Username: "frank", Status: model.UserStatusActive, Version: 1},
			Profile: model.UserProfile{},
			Roles:   []model.Role{model.RoleRegularUser},
		},
		Username: "admin",
	}))

	// Should have Remove for REGULAR_USER and Assign for others
	assert.Contains(t, html, "Remove")
	assert.Contains(t, html, "Assign")
	// Role endpoints in form actions
	assert.Contains(t, html, "roles/assign")
	assert.Contains(t, html, "roles/remove")
}

// ─── Badge rendering ──────────────────────────────────────────────────────────

func TestBadgeStatusStyling(t *testing.T) {
	// Test that status values produce distinct badge classes
	tests := []struct {
		status string
		want   string
	}{
		{"ACTIVE", "ee-badge--success"},
		{"SUSPENDED", "ee-badge--danger"},
		{"BANNED", "ee-badge--danger"},
		{"DEACTIVATED", "ee-badge--muted"},
	}

	for _, tt := range tests {
		// The badge rendering is tested indirectly through user list
		html := render(t, adminpages.UserListPage(adminpages.UserListData{
			Users: []model.UserWithRoles{
				{
					User:  model.User{ID: uuid.New(), Username: "u", Status: parseStatus(t, tt.status)},
					Roles: []model.Role{model.RoleRegularUser},
				},
			},
			Total: 1, Page: 1, PageSize: 20, Username: "admin",
		}))
		assert.True(t, strings.Contains(html, tt.want),
			"expected %q in rendered HTML for status %q", tt.want, tt.status)
	}
}

func parseStatus(t *testing.T, s string) model.UserStatus {
	t.Helper()
	status, err := model.ParseUserStatus(s)
	require.NoError(t, err)
	return status
}
