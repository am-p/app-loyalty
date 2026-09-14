# Migraciones

Se aplican con `go run ./cmd/migrate up`. El proceso API nunca crea ni modifica el esquema.
`down` sólo revierte la última versión y requiere `ALLOW_MIGRATION_DOWN=true`.

Las versiones `0009`, `0010` y `0011` son deliberadamente forward-only porque revierten
anonimización o controles de credenciales. Sus archivos `down` abortan. Para recuperar,
restaurar un backup probado en un entorno aislado y aplicar una migración forward nueva.
