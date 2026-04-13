package authhandler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/eduexchange/eduexchange/internal/middleware"
	"github.com/eduexchange/eduexchange/internal/model"
	authservice "github.com/eduexchange/eduexchange/internal/service/auth"
	authpages "github.com/eduexchange/eduexchange/internal/templ/pages/auth"
)

// Handler serves the auth routes: GET /login, GET /register, POST /login, POST /register, POST /logout.
type Handler struct {
	authSvc       *authservice.AuthService
	secureCookies bool
}

func New(authSvc *authservice.AuthService) *Handler {
	return &Handler{authSvc: authSvc}
}

// NewSecure creates a Handler with the Secure flag enabled on session cookies.
func NewSecure(authSvc *authservice.AuthService) *Handler {
	return &Handler{authSvc: authSvc, secureCookies: true}
}

// GetLogin renders the login page.
func (h *Handler) GetLogin(c *gin.Context) {
	// Already logged in? Redirect to dashboard.
	if _, err := c.Cookie("session_token"); err == nil {
		c.Redirect(http.StatusFound, "/")
		return
	}
	c.Status(http.StatusOK)
	_ = authpages.LoginPage(authpages.LoginData{}).Render(c.Request.Context(), c.Writer)
}

// GetRegister renders the registration page.
func (h *Handler) GetRegister(c *gin.Context) {
	if _, err := c.Cookie("session_token"); err == nil {
		c.Redirect(http.StatusFound, "/")
		return
	}
	c.Status(http.StatusOK)
	_ = authpages.RegisterPage(authpages.RegisterData{}).Render(c.Request.Context(), c.Writer)
}

// PostLogin handles form submission for login.
func (h *Handler) PostLogin(c *gin.Context) {
	username := c.PostForm("username")
	password := c.PostForm("password")

	result, err := h.authSvc.Login(c.Request.Context(), username, password)
	if err != nil {
		var errMsg string
		if errors.Is(err, model.ErrForbidden) {
			errMsg = "Your account is locked or inactive. Please try again later."
		} else {
			errMsg = "Invalid username or password."
		}
		c.Status(http.StatusUnprocessableEntity)
		_ = authpages.LoginPage(authpages.LoginData{
			Error:    errMsg,
			Username: username,
		}).Render(c.Request.Context(), c.Writer)
		return
	}

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "session_token",
		Value:    result.Token,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400, // 24h
	})

	c.Redirect(http.StatusFound, "/")
}

// PostRegister handles form submission for registration.
func (h *Handler) PostRegister(c *gin.Context) {
	username := c.PostForm("username")
	email := c.PostForm("email")
	password := c.PostForm("password")

	_, err := h.authSvc.Register(c.Request.Context(), username, email, password)
	if err != nil {
		data := authpages.RegisterData{
			Username: username,
			Email:    email,
		}

		var ve *model.ValidationErrors
		if errors.As(err, &ve) {
			data.FieldErrors = make(map[string]string)
			for _, e := range ve.Errors {
				data.FieldErrors[e.Field] = e.Message
			}
		} else {
			data.Error = "Registration failed. Please try again."
		}

		c.Status(http.StatusUnprocessableEntity)
		_ = authpages.RegisterPage(data).Render(c.Request.Context(), c.Writer)
		return
	}

	c.Redirect(http.StatusFound, "/login?registered=1")
}

// PostLogout deletes the session and clears the cookie.
func (h *Handler) PostLogout(c *gin.Context) {
	token, err := c.Cookie("session_token")
	if err == nil && token != "" {
		_ = h.authSvc.Logout(c.Request.Context(), token)
	}

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "session_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})

	// Check if current user is the logged-in user (middleware sets this)
	_ = middleware.GetAuthUser(c) // no-op, just documents the flow

	if c.GetHeader("HX-Request") == "true" {
		c.Header("HX-Redirect", "/login")
		c.Status(http.StatusOK)
		return
	}
	c.Redirect(http.StatusFound, "/login")
}
