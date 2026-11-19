// Package outbox records domain events in the same transaction as the change (transactional outbox). A relay,
// added with the event bus, publishes the rows; other services (Authentication's org copy) subscribe there.
package outbox

import (
	"context"
	"encoding/json"

	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/httpx/reqid"
)

type Event struct {
	AggregateType string // e.g. "org_level"
	AggregateID   string // the record's id (UUID)
	AggregateCode string // its business code, for reading the log
	Type          string // e.g. "pm.org_level.created"
	Payload       any
	ActorID       string // user id (UUID)
}

type Writer struct{ DB *database.DB }

func (w Writer) Write(ctx context.Context, e Event) error {
	body, err := json.Marshal(e.Payload)
	if err != nil {
		return err
	}
	_, err = w.DB.Q(ctx).Exec(ctx,
		`INSERT INTO pm.outbox (aggregate_type, aggregate_id, aggregate_code, event_type, payload, actor_id, request_id)
		 VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, NULLIF($7, ''))`,
		e.AggregateType, e.AggregateID, e.AggregateCode, e.Type, body, e.ActorID, reqid.From(ctx))
	return err
}
