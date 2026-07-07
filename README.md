# Puntazo — Backend

Backend for the "Puntazo" customer loyalty system, built with Go, [Gin](https://github.com/gin-gonic/gin) and PostgreSQL ([pgx](https://github.com/jackc/pgx)).

## Domain actors

- **Shop**: a business using the platform (e.g. a wine store). Registers an account, defines loyalty benefits ("4th wine free"), records sales. These are the current endpoints.
- **Client**: an end customer of a shop. Belongs to a shop, accumulates purchases, receives benefits. Not implemented yet — always use "client" for this actor, never "customer/shop".

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

# 2. Run the server (creates the `shops` table automatically)
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

### POST /shops — register a shop

Request:

```bash
curl -X POST http://localhost:8080/shops \
  -H "Content-Type: application/json" \
  -d '{"name":"La Vinoteca","email":"vinoteca@example.com","password":"12345678"}'
```

Validation: all fields required, `email` must be a valid email, `password` minimum 8 characters.

Responses:

| Status | Body | Meaning |
|--------|------|---------|
| 201 | `{"message":"shop registered"}` | Created |
| 400 | `{"error":"..."}` | Invalid JSON or failed validation |
| 409 | `{"error":"email already registered"}` | Duplicate email |
| 500 | `{"error":"could not save shop"}` | Unexpected server/database error |

## Project structure

```
cmd/server/          main.go — entry point, wiring, routes
internal/config/     database pool configuration
internal/handler/    HTTP handlers (Gin)
internal/model/      domain structs
internal/repository/ SQL queries (pgx)
internal/service/    business logic (coming soon)
```
