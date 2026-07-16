package repository

import (
	"clientesFrecuentes/internal/model"
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func CreateTableQuery(p *pgxpool.Pool) error {
	_, err := p.Exec(context.Background(), "CREATE TABLE IF NOT EXISTS users (id SERIAL PRIMARY KEY,email VARCHAR(255) UNIQUE NOT NULL,password_hash VARCHAR(255),google_id VARCHAR(255),name VARCHAR(255) NOT NULL,role VARCHAR(20) NOT NULL,id_shop INT, id_client INT, created_at TIMESTAMPTZ NOT NULL DEFAULT now());")
	return err
}

func InsertUser(p *pgxpool.Pool, u model.User) (int64, error) {
	var id int64
	err := p.QueryRow(context.Background(), "INSERT INTO users(name, email, password_hash, role) values($1, $2, $3, $4) RETURNING id", u.Name, u.Email, u.PasswordHash, u.Role).Scan(&id)
	return id, err
}
