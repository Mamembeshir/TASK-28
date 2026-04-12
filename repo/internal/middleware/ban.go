package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BanCheckMiddleware checks if the authenticated user has an active temporary ban.
// On non-GET requests, if a ban is found, returns 403.
// Permanent bans are enforced by the auth middleware (status=BANNED).
func BanCheckMiddleware(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead {
			c.Next()
			return
		}
		user := GetAuthUser(c)
		if user == nil {
			c.Next()
			return
		}
		var banID uuid.UUID
		err := pool.QueryRow(c.Request.Context(), `
			SELECT id FROM user_bans
			WHERE user_id=$1 AND is_active=TRUE AND ban_type != 'PERMANENT'
			AND (expires_at IS NULL OR expires_at > NOW())`, user.ID).Scan(&banID)
		if err == nil {
			// Active temp ban found
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "Your account is temporarily banned. You may browse but cannot post.",
			})
			return
		}
		c.Next()
	}
}
