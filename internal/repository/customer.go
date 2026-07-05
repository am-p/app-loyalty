package repository

import (
	"clientesFrecuentes/internal/model"
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func CreateTableQuery(p *pgxpool.Pool) error {
	_, err := p.Exec(context.Background(), "CREATE TABLE IF NOT EXISTS customers (id SERIAL PRIMARY KEY,name VARCHAR(255) NOT NULL,email VARCHAR(255) UNIQUE NOT NULL,password VARCHAR(255) NOT NULL);")
	return err
}

func InsertQuery(p *pgxpool.Pool, c model.Customer) error {
	_, err := p.Exec(context.Background(), "INSERT INTO customers(name, email, password) values($1, $2, $3)", c.Name, c.Email, c.Password)
	return err
}
