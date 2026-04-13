package audit

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/eduexchange/eduexchange/internal/sanitize"
)

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

type Entry struct {
	ActorID        uuid.UUID
	Action         string
	EntityType     string
	EntityID       uuid.UUID
	BeforeData     interface{}
	AfterData      interface{}
	IPAddress      string
	Source         string
	Reason        string
	CorrelationID string
	IdempotencyKey string
}

func (s *Service) Record(ctx context.Context, entry Entry) error {
	beforeJSON, err := sanitize.JSONNullable(entry.BeforeData)
	if err != nil {
		return err
	}

	afterJSON, err := sanitize.JSONNullable(entry.AfterData)
	if err != nil {
		return err
	}

	_, err = s.pool.Exec(ctx,
		`INSERT INTO audit_logs (id, actor_id, action, entity_type, entity_id,
			before_data, after_data, ip_address, source, reason,
			correlation_id, idempotency_key, timestamp)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW())`,
		uuid.New(), entry.ActorID, entry.Action, entry.EntityType, entry.EntityID,
		beforeJSON, afterJSON, entry.IPAddress, entry.Source, entry.Reason,
		entry.CorrelationID, entry.IdempotencyKey,
	)
	return err
}

