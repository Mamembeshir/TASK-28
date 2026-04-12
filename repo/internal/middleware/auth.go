package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type contextKey string

const (
	UserContextKey    contextKey = "user"
	SessionContextKey contextKey = "session"
)

type AuthUser struct {
	ID       uuid.UUID
	Username string
	Email    string
	Status   string
	Roles    []string
}

func AuthMiddleware(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		cookie, err := c.Cookie("session_token")
		if err != nil || cookie == "" {
			handleUnauthorized(c)
			return
		}

		var user AuthUser
		var expiresAt time.Time
		err = pool.QueryRow(c.Request.Context(),
			`SELECT u.id, u.username, u.email, u.status, s.expires_at
			 FROM sessions s
			 JOIN users u ON u.id = s.user_id
			 WHERE s.token = $1 AND s.expires_at > NOW()`,
			cookie,
		).Scan(&user.ID, &user.Username, &user.Email, &user.Status, &expiresAt)

		if err != nil {
			handleUnauthorized(c)
			return
		}

		if user.Status != "ACTIVE" {
			handleUnauthorized(c)
			return
		}

		rows, err := pool.Query(c.Request.Context(),
			`SELECT role FROM user_roles WHERE user_id = $1`, user.ID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var role string
				if err := rows.Scan(&role); err == nil {
					user.Roles = append(user.Roles, role)
				}
			}
		}

		ctx := context.WithValue(c.Request.Context(), UserContextKey, &user)
		c.Request = c.Request.WithContext(ctx)
		c.Set("auth_user", &user)
		c.Next()
	}
}

func GetAuthUser(c *gin.Context) *AuthUser {
	if u, exists := c.Get("auth_user"); exists {
		if user, ok := u.(*AuthUser); ok {
			return user
		}
	}
	return nil
}

func handleUnauthorized(c *gin.Context) {
	if c.GetHeader("HX-Request") == "true" {
		c.Header("HX-Redirect", "/login")
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	if c.GetHeader("Accept") == "application/json" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error": gin.H{
				"code":    "UNAUTHORIZED",
				"message": "Authentication required",
			},
		})
		return
	}
	c.Redirect(http.StatusFound, "/login")
	c.Abort()
}
