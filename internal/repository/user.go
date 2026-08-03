package repository

import (
	"clientesFrecuentes/internal/model"
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func CreateTableQuery(p *pgxpool.Pool) error {
	_, err := p.Exec(context.Background(),
		"CREATE TABLE IF NOT EXISTS users (id SERIAL PRIMARY KEY,email VARCHAR(255) UNIQUE NOT NULL,password_hash VARCHAR(255),google_id VARCHAR(255),name VARCHAR(255) NOT NULL,role VARCHAR(20) NOT NULL,id_shop INT, id_client INT, created_at TIMESTAMPTZ NOT NULL DEFAULT now());",
	)
	return err
}

func InsertUser(p *pgxpool.Pool, u model.User) (int64, error) {
	var id int64
	err := p.QueryRow(context.Background(),
		"INSERT INTO users(name, email, password_hash, role, google_id) values($1, $2, $3, $4, $5) RETURNING id",
		u.Name, u.Email, u.PasswordHash, u.Role, u.GoogleID,
	).Scan(&id)
	return id, err
}

func GetUserByID(p *pgxpool.Pool, id int64) (model.User, error) {
	var u model.User
	err := p.QueryRow(context.Background(),
		"SELECT id, email, password_hash, google_id, name, role FROM users WHERE id = $1",
		id,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.GoogleID, &u.Name, &u.Role)
	return u, err
}

func GetUserByEmail(p *pgxpool.Pool, email string) (model.User, error) {
	var u model.User
	err := p.QueryRow(context.Background(),
		"SELECT id, email, password_hash, google_id, name, role FROM users WHERE email = $1",
		email,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.GoogleID, &u.Name, &u.Role)
	return u, err
}

func GetUserByGoogleID(p *pgxpool.Pool, googleID string) (model.User, error) {
	var u model.User
	err := p.QueryRow(context.Background(),
		"SELECT id, email, password_hash, google_id, name, role FROM users WHERE google_id = $1",
		googleID,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.GoogleID, &u.Name, &u.Role)
	return u, err
}

func LinkGoogleID(p *pgxpool.Pool, userID int64, googleID string) error {
	_, err := p.Exec(context.Background(),
		"UPDATE users SET google_id = $1 WHERE id = $2",
		googleID, userID,
	)
	return err
}
