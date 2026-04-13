package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	csrfCookieName = "csrf_token"
	csrfHeaderName = "X-CSRF-Token"
	csrfFormField  = "_csrf"
	// 24-hour TTL; refreshed on every request so active sessions never expire
	csrfCookieTTL = 86400
)

// CSRFMiddleware implements the double-submit cookie pattern.
//
//   - Every request: reads (or generates) a random token and re-sets it as a
//     non-HttpOnly cookie so that client-side code can include it in requests.
//   - POST / PUT / DELETE / PATCH: the submitted value (in the X-CSRF-Token
//     header or the _csrf form field) must match the cookie via constant-time
//     comparison.
//
// secureCookie controls the Secure flag on the CSRF cookie.  Set to true in
// production (HTTPS) so the cookie is never sent over plain HTTP.
func CSRFMiddleware(secureCookie bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Read the existing token from the cookie, or mint a fresh one.
		token, err := c.Cookie(csrfCookieName)
		if err != nil || token == "" {
			token = newCSRFToken()
		}

		// Re-set the cookie on every response (renews the TTL and ensures the
		// cookie is present after login).  Not HttpOnly so JS can read it.
		c.SetSameSite(http.SameSiteLaxMode)
		c.SetCookie(csrfCookieName, token, csrfCookieTTL, "/", "", secureCookie, false /*httpOnly*/)

		// For mutating methods validate the submitted token.
		switch c.Request.Method {
		case http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch:
			submitted := c.GetHeader(csrfHeaderName)
			if submitted == "" {
				submitted = c.PostForm(csrfFormField)
			}
			if subtle.ConstantTimeCompare([]byte(submitted), []byte(token)) != 1 {
				if c.GetHeader("HX-Request") == "true" {
					// HTMX: respond with a redirect header so the UI can handle it gracefully
					c.Header("HX-Redirect", "/login")
					c.AbortWithStatus(http.StatusForbidden)
				} else {
					c.AbortWithStatusJSON(http.StatusForbidden,
						gin.H{"error": "CSRF token validation failed; please reload the page and try again"})
				}
				return
			}
		}

		c.Next()
	}
}

// GetCSRFToken returns the CSRF token for the current request.
// Handlers or templates can use this to embed the token in responses.
func GetCSRFToken(c *gin.Context) string {
	token, _ := c.Cookie(csrfCookieName)
	return token
}

func newCSRFToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// rand.Read only errors on catastrophic OS failures.
		panic("csrf: cannot generate token: " + err.Error())
	}
	return hex.EncodeToString(b)
}
