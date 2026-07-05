# Puntazo — Backend

Backend for the "Puntazo" customer loyalty system, built with Go, [Gin](https://github.com/gin-gonic/gin) and PostgreSQL ([pgx](https://github.com/jackc/pgx)).

## Prerequisites

- Go 1.25+
- Docker (with the compose plugin)

No local PostgreSQL needed — the database runs in Docker.

## Getting started

```bash
git clone <repo-url>
cd clientesFrecuentes

# 1. Start PostgreSQL
docker compose up -d

# 2. Run the server (creates the `customers` table automatically)
go run cmd/server/main.go
```

The API listens on **http://localhost:8080**.

## Database

Defined in `docker-compose.yml` (PostgreSQL 16):

| Setting  | Value         |
|----------|---------------|
| Host     | localhost     |
| Port     | 5432          |
| User     | loyalty       |
| Password | loyalty       |
| Database | loyalty_dev   |

To inspect it:

```bash
docker compose exec db psql -U loyalty -d loyalty_dev
```

## API

### POST /customers — register a customer

Request:

```bash
curl -X POST http://localhost:8080/customers \
  -H "Content-Type: application/json" \
  -d '{"name":"Ariel","email":"ariel@example.com","password":"1234"}'
```

Responses:

| Status | Body | Meaning |
|--------|------|---------|
| 201 | `{"message":"customer registered"}` | Created |
| 400 | `{"error":"..."}` | Invalid JSON or missing name/email/password |
| 409 | `{"error":"email already registered"}` | Duplicate email |
| 500 | `{"error":"could not save customer"}` | Unexpected server/database error |

## Project structure

```
cmd/server/          main.go — entry point, wiring, routes
internal/config/     database pool configuration
internal/handler/    HTTP handlers (Gin)
internal/model/      domain structs
internal/repository/ SQL queries (pgx)
internal/service/    business logic (coming soon)
```
