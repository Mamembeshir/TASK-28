package frontend_test

import (
	"bytes"
	"context"
	"testing"

	components "github.com/eduexchange/eduexchange/internal/templ/components"
	layouts "github.com/eduexchange/eduexchange/internal/templ/layouts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Sidebar ───────────────────────────────────────────────────────────────────

func TestSidebar_Renders(t *testing.T) {
	sections := []components.NavSection{
		{
			Label: "Resources",
			Items: []components.NavItem{
				{Label: "Browse", URL: "/resources", Roles: []string{}}, // empty = all roles
				{Label: "My Resources", URL: "/resources/mine", Roles: []string{"AUTHOR"}},
			},
		},
		{
			Label: "Admin",
			Items: []components.NavItem{
				{Label: "Users", URL: "/admin/users", Roles: []string{"ADMIN"}},
			},
		},
	}

	var buf bytes.Buffer
	err := components.Sidebar(sections, "/resources", "AUTHOR").Render(context.Background(), &buf)
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "Browse")
	assert.Contains(t, html, "My Resources")
}

func TestSidebar_AdminVisible(t *testing.T) {
	sections := []components.NavSection{
		{
			Label: "Admin",
			Items: []components.NavItem{
				{Label: "Users", URL: "/admin/users", Roles: []string{"ADMIN"}},
			},
		},
	}

	var buf bytes.Buffer
	err := components.Sidebar(sections, "/admin/users", "ADMIN").Render(context.Background(), &buf)
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "Users")
}

// ── Pagination ────────────────────────────────────────────────────────────────

func TestPagination_FirstPage(t *testing.T) {
	var buf bytes.Buffer
	err := components.Pagination(1, 5, "/resources").Render(context.Background(), &buf)
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "page=2")
}

func TestPagination_LastPage(t *testing.T) {
	var buf bytes.Buffer
	err := components.Pagination(5, 5, "/resources").Render(context.Background(), &buf)
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "/resources")
}

func TestPagination_SinglePage(t *testing.T) {
	var buf bytes.Buffer
	err := components.Pagination(1, 1, "/resources").Render(context.Background(), &buf)
	require.NoError(t, err)

	// With 1 page the component renders nothing (no navigation needed) — just verify no panic
	_ = buf.String()
}

// ── Badge ─────────────────────────────────────────────────────────────────────

func TestBadge_Published(t *testing.T) {
	var buf bytes.Buffer
	err := components.Badge("PUBLISHED", "Published").Render(context.Background(), &buf)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "Published")
}

func TestBadge_Draft(t *testing.T) {
	var buf bytes.Buffer
	err := components.Badge("DRAFT", "Draft").Render(context.Background(), &buf)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "Draft")
}

// ── EmptyState ────────────────────────────────────────────────────────────────

func TestEmptyState_Renders(t *testing.T) {
	var buf bytes.Buffer
	err := components.EmptyState("📭", "Nothing here", "There are no items to show.", "Add one", "/new").Render(context.Background(), &buf)
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "Nothing here")
	assert.Contains(t, html, "Add one")
}

// ── Button ────────────────────────────────────────────────────────────────────

func TestButton_Primary(t *testing.T) {
	var buf bytes.Buffer
	err := components.Button(components.ButtonPrimary, "Save", components.ButtonAttrs{Type: "submit"}).Render(context.Background(), &buf)
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "Save")
	assert.Contains(t, html, "submit")
}

func TestButton_Danger(t *testing.T) {
	var buf bytes.Buffer
	err := components.Button(components.ButtonDanger, "Delete", components.ButtonAttrs{Type: "button"}).Render(context.Background(), &buf)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "Delete")
}

func TestButton_Secondary(t *testing.T) {
	var buf bytes.Buffer
	err := components.Button(components.ButtonSecondary, "Cancel", components.ButtonAttrs{Type: "button"}).Render(context.Background(), &buf)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "Cancel")
}

// ── Input ─────────────────────────────────────────────────────────────────────

func TestInput_NoError(t *testing.T) {
	var buf bytes.Buffer
	err := components.Input("Email", "email", "test@example.com", "").Render(context.Background(), &buf)
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "Email")
	assert.Contains(t, html, "test@example.com")
}

func TestInput_WithError(t *testing.T) {
	var buf bytes.Buffer
	err := components.Input("Email", "email", "", "Invalid email").Render(context.Background(), &buf)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "Invalid email")
}

func TestPasswordInput_Renders(t *testing.T) {
	var buf bytes.Buffer
	err := components.PasswordInput("Password", "password", "").Render(context.Background(), &buf)
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "Password")
	assert.Contains(t, html, "password")
}

// ── SelectField ───────────────────────────────────────────────────────────────

func TestSelectField_Renders(t *testing.T) {
	options := []components.SelectOption{
		{Value: "math", Label: "Mathematics"},
		{Value: "sci", Label: "Science"},
	}

	var buf bytes.Buffer
	err := components.SelectField("Category", "category_id", options, "math", "").Render(context.Background(), &buf)
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "Category")
	assert.Contains(t, html, "Mathematics")
	assert.Contains(t, html, "Science")
}

func TestSelectField_WithError(t *testing.T) {
	var buf bytes.Buffer
	err := components.SelectField("Category", "category_id", nil, "", "Required").Render(context.Background(), &buf)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "Required")
}

// ── TextArea ──────────────────────────────────────────────────────────────────

func TestTextArea_Renders(t *testing.T) {
	var buf bytes.Buffer
	err := components.TextArea("Description", "description", "Some text", "", 500).Render(context.Background(), &buf)
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "Description")
	assert.Contains(t, html, "Some text")
}

// ── Toast (UI) ────────────────────────────────────────────────────────────────

func TestToast_Success(t *testing.T) {
	var buf bytes.Buffer
	err := components.Toast(components.ToastSuccess, "Saved successfully!").Render(context.Background(), &buf)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "Saved successfully!")
}

func TestToast_Danger(t *testing.T) {
	var buf bytes.Buffer
	err := components.Toast(components.ToastDanger, "An error occurred").Render(context.Background(), &buf)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "An error occurred")
}

// ── PageHeader ────────────────────────────────────────────────────────────────

func TestPageHeader_WithBreadcrumbs(t *testing.T) {
	breadcrumbs := []components.Breadcrumb{
		{Label: "Home", URL: "/"},
		{Label: "Resources", URL: "/resources"},
	}

	var buf bytes.Buffer
	err := components.PageHeader("My Resources", breadcrumbs).Render(context.Background(), &buf)
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "My Resources")
	assert.Contains(t, html, "Home")
	assert.Contains(t, html, "Resources")
}

func TestPageHeader_NoBreadcrumbs(t *testing.T) {
	var buf bytes.Buffer
	err := components.PageHeader("Dashboard", nil).Render(context.Background(), &buf)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "Dashboard")
}

// ── HTMXIndicator ─────────────────────────────────────────────────────────────

func TestHTMXIndicator_Renders(t *testing.T) {
	var buf bytes.Buffer
	err := components.HTMXIndicator().Render(context.Background(), &buf)
	require.NoError(t, err)

	// Should contain some loading indicator markup
	assert.NotEmpty(t, buf.String())
}

// ── Card ──────────────────────────────────────────────────────────────────────

func TestCard_Renders(t *testing.T) {
	var buf bytes.Buffer
	err := components.Card("My Card Title").Render(context.Background(), &buf)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "My Card Title")
}

// ── Modal ─────────────────────────────────────────────────────────────────────

func TestModal_Renders(t *testing.T) {
	var buf bytes.Buffer
	err := components.Modal("Confirm Action").Render(context.Background(), &buf)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "Confirm Action")
}

// ── Table ─────────────────────────────────────────────────────────────────────

func TestTable_WithHeaders(t *testing.T) {
	var buf bytes.Buffer
	err := components.Table([]string{"Name", "Status", "Date"}).Render(context.Background(), &buf)
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "Name")
	assert.Contains(t, html, "Status")
	assert.Contains(t, html, "Date")
}

// ── Layouts ───────────────────────────────────────────────────────────────────

func TestBaseLayout_Renders(t *testing.T) {
	data := layouts.PageData{
		Title:       "Test Page",
		ActiveRoute: "/resources",
		UserRole:    "USER",
		Username:    "testuser",
	}

	var buf bytes.Buffer
	err := layouts.Base(data).Render(context.Background(), &buf)
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "Test Page")
	assert.Contains(t, html, "EduExchange")
}

func TestAuthLayout_Renders(t *testing.T) {
	var buf bytes.Buffer
	err := layouts.Auth("Login").Render(context.Background(), &buf)
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "Login")
	assert.Contains(t, html, "EduExchange")
}
