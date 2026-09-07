package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type IdempotentResult struct {
	Status   int
	Body     json.RawMessage
	Replayed bool
}

func claimIdempotency(ctx context.Context, tx pgx.Tx, key, actorScope, operation string, fingerprint []byte) (*IdempotentResult, error) {
	// Bound only the reservation wait. A concurrent speculative insert for the
	// same key must become IDEMPOTENCY_IN_PROGRESS instead of waiting for the
	// first request until the HTTP deadline.
	if _, err := tx.Exec(ctx, `SET LOCAL lock_timeout='150ms'`); err != nil {
		return nil, err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO solicitudes_idempotentes(idempotency_key,actor_scope,operacion,fingerprint,estado) VALUES($1,$2,$3,$4,'PENDING') ON CONFLICT DO NOTHING`, key, actorScope, operation, fingerprint)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "55P03" {
			return nil, ErrIdempotencyInProgress
		}
		return nil, err
	}
	if _, err = tx.Exec(ctx, `SET LOCAL lock_timeout='0'`); err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 1 {
		return nil, nil
	}
	var storedActor, storedOperation, state string
	var storedFingerprint []byte
	var status *int
	var body []byte
	err = tx.QueryRow(ctx, `SELECT actor_scope,operacion,fingerprint,estado,response_status,response_body FROM solicitudes_idempotentes WHERE idempotency_key=$1 FOR UPDATE`, key).Scan(&storedActor, &storedOperation, &storedFingerprint, &state, &status, &body)
	if err != nil {
		return nil, err
	}
	if storedActor != actorScope || storedOperation != operation || !bytes.Equal(storedFingerprint, fingerprint) {
		return nil, ErrIdempotencyConflict
	}
	if state != "COMPLETED" || status == nil {
		return nil, ErrIdempotencyInProgress
	}
	return &IdempotentResult{Status: *status, Body: json.RawMessage(body), Replayed: true}, nil
}
