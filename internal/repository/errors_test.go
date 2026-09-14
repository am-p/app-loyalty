package repository

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsRetryableRecognizesOnlySerializationAndDeadlock(t *testing.T) {
	for _, code := range []string{"40001", "40P01"} {
		if !IsRetryable(&pgconn.PgError{Code: code}) {
			t.Fatalf("code %s was not retryable", code)
		}
	}
	if IsRetryable(&pgconn.PgError{Code: "23505"}) || IsRetryable(errors.New("network")) {
		t.Fatal("non-transaction conflict was retryable")
	}
}
