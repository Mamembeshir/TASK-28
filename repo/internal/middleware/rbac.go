package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := GetAuthUser(c)
		if user == nil {
			handleUnauthorized(c)
			return
		}

		for _, required := range roles {
			for _, userRole := range user.Roles {
				if userRole == required {
					c.Next()
					return
				}
			}
		}

		accept := c.GetHeader("Accept")
		if c.GetHeader("HX-Request") == "true" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"status": "error",
				"error":  gin.H{"code": "FORBIDDEN", "message": "Insufficient permissions"},
			})
			return
		}
		if strings.Contains(accept, "text/html") {
			c.Status(http.StatusForbidden)
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.Abort()
			_, _ = c.Writer.WriteString(forbiddenPage)
			return
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"status": "error",
			"error":  gin.H{"code": "FORBIDDEN", "message": "Insufficient permissions"},
		})
	}
}

const forbiddenPage = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8"/>
  <meta name="viewport" content="width=device-width, initial-scale=1.0"/>
  <title>Access Denied — EduExchange</title>
  <link rel="stylesheet" href="/static/css/app.css"/>
  <style>
    body { margin:0; font-family: system-ui, sans-serif; background:#f9fafb; display:flex; align-items:center; justify-content:center; min-height:100vh; }
    .card { background:white; border:1px solid #e5e7eb; border-radius:12px; padding:48px 40px; text-align:center; max-width:400px; box-shadow:0 1px 3px rgba(0,0,0,0.08); }
    .icon { width:56px; height:56px; border-radius:12px; background:#fef2f2; display:flex; align-items:center; justify-content:center; margin:0 auto 20px; }
    h1 { font-size:1.25rem; font-weight:700; color:#111827; margin:0 0 8px; }
    p { font-size:0.875rem; color:#6b7280; margin:0 0 24px; line-height:1.5; }
    a { display:inline-block; padding:10px 20px; background:#3b82f6; color:white; border-radius:8px; text-decoration:none; font-size:0.875rem; font-weight:600; }
    a:hover { background:#2563eb; }
  </style>
</head>
<body>
  <div class="card">
    <div class="icon">
      <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="#ef4444" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <rect x="3" y="11" width="18" height="11" rx="2" ry="2"></rect>
        <path d="M7 11V7a5 5 0 0110 0v4"></path>
      </svg>
    </div>
    <h1>Access Denied</h1>
    <p>You don't have permission to view this page. Contact your administrator if you think this is a mistake.</p>
    <a href="/">Go to Dashboard</a>
  </div>
</body>
</html>`
