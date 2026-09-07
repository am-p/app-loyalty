package repository

import (
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	Pool *pgxpool.Pool
	Now  func() time.Time
}

func New(pool *pgxpool.Pool) *Repository { return &Repository{Pool: pool, Now: time.Now} }
