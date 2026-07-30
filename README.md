# Puntazo — Backend

Backend for the "Puntazo" customer loyalty system, built with Go, [Gin](https://github.com/gin-gonic/gin) and PostgreSQL ([pgx](https://github.com/jackc/pgx)).

## Domain actors

Both actors share a single `users` table and a single authentication flow.

- **Client** (`rol: CLIENTE_FINAL`): an end customer. Every account starts here.
- **Shop** (`rol: TIENDA`): a business using the platform (e.g. a wine store). A user becomes a shop by subscribing to a paid plan via `POST /suscripciones` — not implemented yet.

## Prerequisites

- Go 1.25+
- Docker (with the compose plugin)

No local PostgreSQL needed — the database runs in Docker.

## Getting started

```bash
git clone <repo-url>
cd clientesFrecuentes

# 1. Configuration
cp .env.example .env
# set JWT_SECRET, e.g. with: openssl rand -base64 32

# 2. Start PostgreSQL
docker compose up -d

# 3. Run the server (creates the `users` table automatically)
go run ./cmd/server
```

Run from the project root — `.env` is loaded from the working directory.

The API listens on **http://localhost:8080**.

## Configuration

`.env` (git-ignored, see `.env.example`):

| Variable       | Description                                       |
|----------------|---------------------------------------------------|
| `DATABASE_URL` | PostgreSQL connection string                      |
| `JWT_SECRET`   | Key used to sign JWTs. Must be secret and random.  |

The server exits at startup if either one is missing.

## Database

PostgreSQL 16, defined in `docker-compose.yml`. Connection details are not
documented here — they live in `docker-compose.yml` and in your local `.env`.

### `users` table

| Column          | Type          | Notes                                  |
|-----------------|---------------|----------------------------------------|
| `id`            | SERIAL PK     |                                        |
| `email`         | VARCHAR(255)  | unique, not null                       |
| `password_hash` | VARCHAR(255)  | nullable — null for Google accounts     |
| `google_id`     | VARCHAR(255)  | nullable — OAuth only                   |
| `name`          | VARCHAR(255)  | not null                               |
| `role`          | VARCHAR(20)   | not null — `CLIENTE_FINAL` \| `TIENDA` |
| `id_shop`       | INT           | nullable — only for `TIENDA`            |
| `id_client`     | INT           | nullable — only for `CLIENTE_FINAL`     |
| `created_at`    | TIMESTAMPTZ   | defaults to `now()`                    |

## API

Error responses are always `{"message": "<texto para el usuario, en español>"}`.

### POST /auth/register

Registers a user. Every account is created with `rol: CLIENTE_FINAL`; the role is
assigned server-side and cannot be set by the client.

```bash
curl -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"nombre":"Ariel","email":"ariel@example.com","password":"12345678"}'
```

Validation: all fields required, `email` must be a valid email, `password` minimum 8 characters.

`201 Created`:

```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "usuario": {
    "id_usuario": 1,
    "email": "ariel@example.com",
    "nombre": "Ariel",
    "rol": "CLIENTE_FINAL",
    "suscripcion": null
  }
}
```

| Status | Meaning                              |
|--------|--------------------------------------|
| 201    | Created                              |
| 400    | Invalid JSON or failed validation    |
| 409    | Email already registered             |
| 500    | Unexpected server/database error     |

### POST /auth/login

```bash
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"ariel@example.com","password":"12345678"}'
```

`200 OK` — same body as register: `token` plus `usuario`.

| Status | Meaning                                                       |
|--------|---------------------------------------------------------------|
| 200    | Authenticated                                                 |
| 400    | Invalid JSON or missing fields                                |
| 401    | Wrong password, unknown email, or account without a password  |
| 500    | Unexpected server/database error                              |

All three 401 cases return the same message, so the response never reveals
whether an email is registered.

### GET /me

Returns the user the token belongs to, so the app can restore a session on
startup. Requires authentication.

```bash
TOKEN='paste-the-token-from-login-here'

curl http://localhost:8080/me \
  -H "Authorization: Bearer $TOKEN"
```

`200 OK` — the `usuario` object alone, with no token:

```json
{
  "id_usuario": 1,
  "email": "ariel@example.com",
  "nombre": "Ariel",
  "rol": "CLIENTE_FINAL",
  "suscripcion": null
}
```

| Status | Meaning                                                  |
|--------|----------------------------------------------------------|
| 200    | OK                                                       |
| 401    | Missing, malformed, forged or expired token              |
| 404    | Token is valid but the account no longer exists          |
| 500    | Unexpected server/database error                         |

Checks worth running against `/me`:

```bash
curl -i http://localhost:8080/me                                 # 401 — no header
curl -i http://localhost:8080/me -H "Authorization: $TOKEN"      # 401 — no "Bearer " prefix
curl -i http://localhost:8080/me -H "Authorization: Bearer abc"  # 401 — not a JWT
```

Then take a valid token, change one character in its middle section and send it:
still 401. That is the signature check working — the payload is readable by
anyone, but it cannot be edited without `JWT_SECRET`.

### Authentication

The `token` is a JWT signed with `JWT_SECRET` (HS256), valid for 24 hours, with claims
`sub` (user id), `rol`, `iat` and `exp`. Protected endpoints expect it as:

```
Authorization: Bearer <token>
```

The token is signed, not encrypted: its payload is readable by anyone holding it.
Never put anything secret in the claims.

Not implemented yet: `POST /auth/google`.

## Project structure

```
cmd/server/          main.go — entry point, wiring, routes
internal/auth/       JWT generation and verification
internal/config/     database pool configuration
internal/handler/    HTTP handlers (Gin)
internal/middleware/ Bearer token check for protected routes
internal/model/      domain structs and request/response DTOs
internal/repository/ SQL queries (pgx)
internal/service/    business logic (hashing, credential checking)
```
