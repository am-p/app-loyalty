# Puntazo · backend Sellos

API de la vertical `DEMO-01-SELLOS`, implementada con Go, Gin, pgx y PostgreSQL 16. El contrato versionado está en [`openapi.yaml`](./openapi.yaml) y el proceso de la API no crea ni modifica tablas al iniciar.

Esta versión parte de una base PostgreSQL nueva. No migra ni reutiliza cuentas de la tabla legacy `users`.

## Desarrollo local

Requisitos: Go 1.25, Docker y Docker Compose.

Para levantar PostgreSQL, aplicar las migraciones y arrancar la API con las mismas imágenes usadas en despliegue:

```bash
cp .env.example .env
# Reemplazar JWT_SECRET, QR_PEPPER, DEMO_ACCESS_CODE_HASH y la contraseña de PostgreSQL.
docker compose --env-file .env up --build
```

La API queda disponible en `http://localhost:8080/v1`. Los aliases `/auth/register`, `/auth/login`, `/auth/google` y `/me` se mantienen sólo para compatibilidad temporal.

Para ejecutar Go directamente desde el host, cambiar el host de `DATABASE_URL` de `db` a `localhost` y usar:

```bash
go run ./cmd/migrate up
go run ./cmd/server
```

## Imágenes independientes

El [`Dockerfile`](./Dockerfile) contiene dos targets:

- `api`: binario HTTP y migrador disponibles, sin ejecutar migraciones automáticamente al iniciar;
- `migrate`: migrador con los SQL versionados incluidos y `MIGRATIONS_DIR=/migrations`.

```bash
docker build --target migrate -t puntazo-migrate .
docker build --target api -t puntazo-api .
docker run --rm --env-file .env puntazo-migrate up
docker run --rm --env-file .env -p 8080:8080 puntazo-api
```

En un despliegue se debe ejecutar la migración `up` y sólo después publicar la API. El target `api` también contiene el migrador para que una plataforma pueda usar este comando previo al despliegue sin construir otra imagen:

```bash
docker run --rm --env-file .env \
  --entrypoint /usr/local/bin/puntazo-migrate puntazo-api up
```

Las migraciones toman un advisory lock, registran `schema_migrations` y son idempotentes. `/v1/health/ready` exige que la versión aplicada coincida con `EXPECTED_SCHEMA_VERSION`.

Las migraciones descendentes están deshabilitadas por defecto:

```bash
ALLOW_MIGRATION_DOWN=true go run ./cmd/migrate down
```

## Configuración

| Variable | Uso |
|---|---|
| `DATABASE_URL` | PostgreSQL nuevo y exclusivo para esta API. |
| `JWT_SECRET` | Secreto aleatorio de al menos 32 bytes. |
| `JWT_ISSUER` | Issuer firmado y validado en JWT; por defecto `puntazo`. Cambiarlo invalida sesiones previas. |
| `QR_PEPPER` | Secreto distinto de `JWT_SECRET`, de al menos 32 bytes. |
| `DEMO_ACCESS_CODE_HASH` | Hash bcrypt del código de alta; obligatorio si `DEMO_SIGNUP_ENABLED=true`. |
| `DEMO_SIGNUP_ENABLED` | Habilita o cierra nuevas altas gratuitas sin bloquear cuentas existentes. |
| `CORS_ORIGINS` | Orígenes web exactos permitidos, separados por comas. |
| `GOOGLE_CLIENT_ID` | Audiencia web de Google; opcional para el alias legado. |
| `EXPECTED_SCHEMA_VERSION` | Versión de esquema requerida por readiness; por defecto `0001`. |
| `TRUSTED_PROXY_COUNT` | Cantidad de proxies confiables para resolver la IP usada por rate limits. |

Los secretos se configuran únicamente en el runtime. Nunca deben copiarse al frontend ni a variables `EXPO_PUBLIC_*`.

## Verificación

```bash
gofmt -w ./cmd ./internal
go vet ./...
go test -race ./...
npx --yes @redocly/cli@1.34.5 lint openapi.yaml
TEST_DATABASE_URL='postgresql://...' \
  go test ./internal/repository -run TestPostgresDemoSellosLifecycle -v
```

`TEST_DATABASE_URL` debe apuntar a una instancia de pruebas. El harness crea un schema aleatorio, ejecuta la migración y lo elimina al finalizar.

## Seguridad operativa

- PostgreSQL conserva únicamente `SHA-256(pepper || qr_token)`; el token QR se deriva mediante HMAC para restaurarlo al propietario sin persistirlo.
- JSON se limita a 1 MiB y los logs estructurados no registran bodies, JWT, QR ni secretos.
- Las confirmaciones requieren `Idempotency-Key`, transacción serializable, `SELECT FOR UPDATE` y hasta tres reintentos para `40001`/`40P01`.
- Para frontend y backend en orígenes distintos, `CORS_ORIGINS` debe contener el origen HTTPS exacto del frontend.
