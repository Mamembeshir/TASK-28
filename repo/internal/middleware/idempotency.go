package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func IdempotencyMiddleware(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodPost && c.Request.Method != http.MethodPut {
			c.Next()
			return
		}

		key := c.GetHeader("X-Idempotency-Key")
		if key == "" {
			c.Next()
			return
		}

		var responseStatus int
		var responseBody string
		err := pool.QueryRow(c.Request.Context(),
			`SELECT response_status, response_body FROM idempotency_records
			 WHERE key = $1 AND created_at > NOW() - INTERVAL '24 hours'`,
			key,
		).Scan(&responseStatus, &responseBody)

		if err == nil {
			c.Data(responseStatus, "application/json", []byte(responseBody))
			c.Abort()
			return
		}

		c.Set("idempotency_key", key)
		c.Next()
	}
}
