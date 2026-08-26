# Puntazo Preview · backend Sellos

API aislada de la demo `DEMO-01-SELLOS`, implementada con Go, Gin, pgx y PostgreSQL. El contrato vinculante está en `../contracts/openapi.yaml`; este proceso no crea tablas al iniciar.

## Desarrollo

Requisitos: Go 1.25 y PostgreSQL 16 o posterior.

```bash
cp .env.example .env
docker compose up -d db
go run ./cmd/migrate up
go run ./cmd/server
```

La API local escucha en `http://localhost:8080/v1`. Los aliases `/auth/register`, `/auth/login`, `/auth/google` y `/me` se mantienen sólo para compatibilidad temporal.

## Migraciones

```bash
go run ./cmd/migrate up
ALLOW_MIGRATION_DOWN=true go run ./cmd/migrate down
```

Las migraciones toman un advisory lock y registran `schema_migrations`. `/v1/health/ready` exige que la versión aplicada coincida con `EXPECTED_SCHEMA_VERSION`.

## Verificación

```bash
go vet ./...
go test -race ./...
TEST_DATABASE_URL='postgresql://...' go test ./internal/repository -run TestPostgresDemoSellosLifecycle -v
```

`TEST_DATABASE_URL` debe apuntar a una instancia de pruebas. El harness crea un schema aleatorio, ejecuta la migración y lo elimina al finalizar.

## Seguridad operativa

- `JWT_SECRET` y `QR_PEPPER` deben ser secretos aleatorios distintos de al menos 32 bytes.
- `DEMO_ACCESS_CODE_HASH` admite el hash bcrypt del código, nunca el código en claro.
- PostgreSQL conserva únicamente `SHA-256(pepper || qr_token)`; el token QR se deriva mediante HMAC para restaurarlo al propietario sin persistirlo.
- JSON se limita a 1 MiB; los logs estructurados registran metadatos del request, no body, JWT, QR ni secretos.
- `TRUSTED_PROXY_COUNT` controla cuántos saltos de `X-Forwarded-For` se aceptan para los rate limits.
- Las confirmaciones usan `Idempotency-Key`, transacción serializable, `SELECT FOR UPDATE` y hasta tres reintentos para `40001`/`40P01`.
