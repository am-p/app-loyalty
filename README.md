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
# set GOOGLE_CLIENT_ID to the OAuth client ID the Expo app uses

# 2. Start PostgreSQL
docker compose up -d

# 3. Run the server (creates the `users` table automatically)
go run ./cmd/server
```

Run from the project root — `.env` is loaded from the working directory.

The API listens on **http://localhost:8080**.

## Configuration

`.env` (git-ignored, see `.env.example`):

| Variable           | Description                                                    |
|--------------------|----------------------------------------------------------------|
| `DATABASE_URL`     | PostgreSQL connection string                                   |
| `JWT_SECRET`       | Key used to sign JWTs. Must be secret and random.               |
| `GOOGLE_CLIENT_ID` | OAuth client ID of the Expo app. Not secret — it ships in the app. |

The server exits at startup if any of them is missing.

`GOOGLE_CLIENT_ID` must be the *same* client ID the frontend signs in with, since
it is checked against the `aud` claim of every Google token. Expo apps often
register several (iOS, Android, Web) — the right one is whichever the app
actually uses, frequently the Web client ID even on mobile.

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

### POST /auth/google

Signs in with a Google ID token. The Expo app runs Google Sign-In, Google hands it
an `id_token`, and the app posts that string here. The account is created if it
doesn't exist yet.

```bash
curl -X POST http://localhost:8080/auth/google \
  -H "Content-Type: application/json" \
  -d '{"id_token":"eyJhbGciOiJSUzI1NiIsImtpZCI6IjEyMzQifQ..."}'
```

`200 OK` — same body as login: `token` plus `usuario`. Note 200 and not 201, even
when the account was just created; the frontend is written against that.

| Status | Meaning                                                                          |
|--------|----------------------------------------------------------------------------------|
| 200    | Authenticated                                                                    |
| 400    | Invalid JSON or missing `id_token`                                               |
| 401    | Token not signed by Google, expired, issued for another app, or email unverified |
| 500    | Unexpected server/database error                                                 |

The `id_token` is Google's, signed by Google with RS256, and is verified against
Google's public keys — it is not, and cannot be, checked with `JWT_SECRET`. Its
`aud` claim must match `GOOGLE_CLIENT_ID`, so a token minted for any other
application is rejected. Google's token is used once, here; every request after
this one uses the token this endpoint returns.

Three cases are handled:

1. A user with that `google_id` exists → returned as is.
2. No such `google_id`, but the verified email matches an existing account → the
   account is linked (`google_id` is stored) and returned. This is how someone who
   registered with a password starts using Google Sign-In.
3. Neither → a new `CLIENTE_FINAL` account is created, with no password.

Case 2 treats a verified Google email as proof of ownership of an existing
account, which is only safe because the `email_verified` claim is required to be
true. An account whose email Google never verified is rejected with 401.

Checks worth running without a real Google token:

```bash
curl -i -X POST http://localhost:8080/auth/google \
  -H "Content-Type: application/json" -d '{"id_token":"abc"}'   # 401 — not a Google token

curl -i -X POST http://localhost:8080/auth/google \
  -H "Content-Type: application/json" -d '{}'                   # 400 — missing field
```

A real token needs the frontend (or Google's OAuth Playground) and the real
`GOOGLE_CLIENT_ID`. After a successful Google login, the returned token works
against `/me` exactly like one from a password login.

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

Google accounts have no `password_hash`, so a password login against one fails
with the same 401 as any other bad credential.

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
