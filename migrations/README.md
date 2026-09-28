# Migraciones

Se aplican con `go run ./cmd/migrate up`. El proceso API nunca crea ni modifica el esquema.
`down` sólo revierte la última versión y requiere `ALLOW_MIGRATION_DOWN=true`.

Las versiones `0009`, `0010` y `0011` son deliberadamente forward-only porque revierten
anonimización o controles de credenciales. Sus archivos `down` abortan. Para recuperar,
restaurar un backup probado en un entorno aislado y aplicar una migración forward nueva.

`0022_google_reviews` añade configuración por sucursal, historial de compras acumuladas
(backfill sin generar invitaciones) e invitaciones únicas por cliente/sucursal. El trigger
sobre el ledger registra una compra por ACUMULACION/CREDITO, incluyendo PUNTOS, dentro
de la misma transacción; no llama a Google. Triggers de deshabilitación de configuración
o sucursal cancelan invitaciones pendientes. No se persiste contenido de Places salvo
el identificador del lugar. Readiness requiere las 22 migraciones (`0022`).

Probar tanto 0021→0022 como 0020→0021→0022 antes de despliegue, con backup previo.
La reversión 0022 elimina datos de reseñas; usar únicamente antes de habilitar el flujo,
o restaurar un backup probado y aplicar un arreglo forward si ya hay actividad.
`GOOGLE_PLACES_API_KEY` es opcional y exclusivo del servidor. Su ausencia conserva
la edición y los enlaces manuales; búsqueda responde `DEPENDENCY_UNAVAILABLE`.

## Backoffice compatible con reseñas

El candidato conserva `0021_expo_push` y `0022_google_reviews`; agrega
`0023_referrals`, `0024_referral_billing`, `0025_subscription_prices`,
`0026_subscription_price_change_history`, `0027_referral_campaign_editing`
y `0028_influencer_welcome_emails`. Readiness requiere `0028`.

Aplicar esta secuencia sólo a bases con la numeración pública anterior
(`0020`, `0021` o `0022`). Una demo local que ya aplicó referidos con la
numeración experimental `0022`–`0027` necesita una base aislada nueva o una
reconciliación explícita; no apuntar este migrador automáticamente a esa base.

Las migraciones amplían las tablas existentes. Una reversión de `0028` con
correos de influencers o de `0027` con campañas de ambos programas aborta
para preservar evidencia; ante un incidente usar una corrección forward.

## Promoción al repositorio oficial

Esta secuencia depende de las reseñas del PR13 (`0022`) y conserva byte a byte
las migraciones `0023`–`0028` aplicadas en testing. No se renumera la base desplegada.
El retorno y correo de confirmación de suscripción se integra después como `0029`.
El PR14 de cambio de email/plantillas usa provisionalmente `0022`/`0023`: no
mezclarlo directamente con este stack. La copia de reconciliación reservará
`0030`/`0031` y preservará los tipos de outbox añadidos en `0028`/`0029`.

`schema_migrations` registra sólo el número de cuatro dígitos: archivos distintos
con el mismo prefijo pueden omitirse silenciosamente. Antes de aplicar migraciones,
verificar que existe exactamente un archivo up/down por versión y comprobar el
historial del ambiente. No adoptar automáticamente la numeración de demos antiguas.
