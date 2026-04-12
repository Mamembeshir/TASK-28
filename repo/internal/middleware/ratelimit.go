package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func RateLimitMiddleware(pool *pgxpool.Pool, actionType string, maxPerHour int) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := GetAuthUser(c)
		if user == nil {
			c.Next()
			return
		}

		windowStart := time.Now().UTC().Truncate(time.Hour)

		var count int
		err := pool.QueryRow(c.Request.Context(),
			`SELECT count FROM rate_limit_counters
			 WHERE user_id = $1 AND action_type = $2 AND window_start = $3`,
			user.ID, actionType, windowStart,
		).Scan(&count)

		if err != nil {
			count = 0
		}

		if count >= maxPerHour {
			retryAfter := windowStart.Add(time.Hour).Sub(time.Now().UTC())
			c.Header("Retry-After", string(rune(int(retryAfter.Seconds()))))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"status": "error",
				"error": gin.H{
					"code":    "RATE_LIMITED",
					"message": "Rate limit exceeded. Try again later.",
				},
			})
			return
		}

		// Increment counter (UPSERT) after check passes
		_, _ = pool.Exec(c.Request.Context(), `
			INSERT INTO rate_limit_counters (user_id, action_type, window_start, count)
			VALUES ($1, $2, $3, 1)
			ON CONFLICT (user_id, action_type, window_start) DO UPDATE
			SET count = rate_limit_counters.count + 1`,
			user.ID, actionType, windowStart)

		c.Next()
	}
}
