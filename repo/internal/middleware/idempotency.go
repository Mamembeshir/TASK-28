package middleware

import (
	"bytes"
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

		// Check for a previously stored response.
		var responseStatus int
		var responseBody string
		err := pool.QueryRow(c.Request.Context(),
			`SELECT response_status, response_body FROM idempotency_records
			 WHERE key = $1 AND created_at > NOW() - INTERVAL '24 hours'`,
			key,
		).Scan(&responseStatus, &responseBody)

		if err == nil {
			// Replay the cached response deterministically.
			c.Data(responseStatus, "application/json", []byte(responseBody))
			c.Abort()
			return
		}

		// Intercept the response so we can persist it after the handler runs.
		rw := &idempotencyWriter{
			ResponseWriter: c.Writer,
			body:           &bytes.Buffer{},
		}
		c.Writer = rw

		c.Set("idempotency_key", key)
		c.Next()

		// Persist the response for future replays.  Best-effort: a failure here
		// means the next identical request will simply re-execute the handler.
		status := rw.Status()
		body := rw.body.String()
		_, _ = pool.Exec(c.Request.Context(),
			`INSERT INTO idempotency_records (key, endpoint, response_status, response_body)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (key) DO NOTHING`,
			key, c.FullPath(), status, body,
		)
	}
}

// idempotencyWriter wraps gin.ResponseWriter to capture the response body.
type idempotencyWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *idempotencyWriter) Write(data []byte) (int, error) {
	w.body.Write(data)
	return w.ResponseWriter.Write(data)
}
