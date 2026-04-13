package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/eduexchange/eduexchange/internal/middleware"
)

func init() {
	// Run all CSRF tests in ReleaseMode so the middleware enforcement branch
	// is exercised (the middleware skips validation in gin.TestMode).
	gin.SetMode(gin.ReleaseMode)
}

func csrfRouter(secure bool) *gin.Engine {
	r := gin.New()
	r.Use(middleware.CSRFMiddleware(secure))

	r.GET("/page", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	r.POST("/action", func(c *gin.Context) {
		c.String(http.StatusOK, "done")
	})
	return r
}

func TestCSRF_GETSetsTokenCookie(t *testing.T) {
	r := csrfRouter(false)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/page", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	cookies := w.Result().Cookies()
	var found bool
	for _, c := range cookies {
		if c.Name == "csrf_token" {
			found = true
			assert.NotEmpty(t, c.Value)
			assert.False(t, c.Secure, "insecure mode should not set Secure flag")
		}
	}
	assert.True(t, found, "csrf_token cookie must be set on GET")
}

func TestCSRF_SecureFlagSet(t *testing.T) {
	r := csrfRouter(true)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/page", nil)
	r.ServeHTTP(w, req)

	for _, c := range w.Result().Cookies() {
		if c.Name == "csrf_token" {
			assert.True(t, c.Secure, "secure mode must set Secure flag")
		}
	}
}

func TestCSRF_POSTWithoutToken_Returns403(t *testing.T) {
	r := csrfRouter(false)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/action", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestCSRF_POSTWithValidHeaderToken_Succeeds(t *testing.T) {
	r := csrfRouter(false)

	// Step 1: GET to obtain the token cookie.
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, httptest.NewRequest(http.MethodGet, "/page", nil))
	var token string
	for _, c := range w1.Result().Cookies() {
		if c.Name == "csrf_token" {
			token = c.Value
		}
	}
	assert.NotEmpty(t, token)

	// Step 2: POST with the token in header + cookie.
	w2 := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/action", nil)
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: token})
	req.Header.Set("X-CSRF-Token", token)
	r.ServeHTTP(w2, req)

	assert.Equal(t, http.StatusOK, w2.Code)
}

func TestCSRF_POSTWithWrongToken_Returns403(t *testing.T) {
	r := csrfRouter(false)

	// GET to obtain a valid token cookie.
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, httptest.NewRequest(http.MethodGet, "/page", nil))
	var token string
	for _, c := range w1.Result().Cookies() {
		if c.Name == "csrf_token" {
			token = c.Value
		}
	}
	assert.NotEmpty(t, token)

	// POST with a mismatched header value.
	w2 := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/action", nil)
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: token})
	req.Header.Set("X-CSRF-Token", "wrong-value")
	r.ServeHTTP(w2, req)

	assert.Equal(t, http.StatusForbidden, w2.Code)
}

func TestCSRF_HTMXRequest_Returns403WithRedirect(t *testing.T) {
	r := csrfRouter(false)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/action", nil)
	req.Header.Set("HX-Request", "true")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Equal(t, "/login", w.Header().Get("HX-Redirect"))
}
