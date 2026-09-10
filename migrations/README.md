# Migraciones

Se aplican con `go run ./cmd/migrate up`. El proceso API nunca crea ni modifica el esquema.
`down` sólo revierte la última versión y requiere `ALLOW_MIGRATION_DOWN=true`.
