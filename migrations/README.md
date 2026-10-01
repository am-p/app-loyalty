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
mezclarlo directamente con este stack. La actualización compatible de la rama original del PR14 reserva
`0030`/`0031` y preserva los tipos de outbox añadidos en `0028`/`0029`.

`schema_migrations` registra sólo el número de cuatro dígitos: archivos distintos
con el mismo prefijo pueden omitirse silenciosamente. Antes de aplicar migraciones,
verificar que existe exactamente un archivo up/down por versión y comprobar el
historial del ambiente. No adoptar automáticamente la numeración de demos antiguas.

## Confirmación de suscripción (`0029`)

La consulta autenticada de resultado verifica la propiedad de la marca y solicita
el estado al proveedor; un retorno de navegador no acredita un cobro. El email
de confirmación tiene snapshot, deduplicación por suscripción y redacción del outbox.
La migración `0029` conserva los correos de influencers de `0028`. Para una base de
testing ya en `0029`, aplicar este mismo historial no renumera ni recrea tablas.
La API requiere esquema `0029`; configurar explícitamente EXPECTED_SCHEMA_VERSION.

## Actualización compatible de la rama original del PR14 (`0030`–`0031`)

La reparación conserva el commit original de email/plantillas del PR14 en su
historia e incorpora el main oficial y las dependencias de los PR16/17/18.
Permite mantener el PR14 original como vía de integración, después de actualizar
su rama con este candidato; la rama original aún no cambia hasta integrar esa
reparación. El PR20 deja de ser la propuesta de integración sustitutiva.

Reserva `0030_email_change` y `0031_card_templates` después de `0029`, con
EXPECTED_SCHEMA_VERSION=0031. Retira del candidato los archivos experimentales
`0022_email_change`/`0023_card_templates`: no se altera una migración aplicada
ni se renumera el historial 0022–0029. La migración email conserva los tipos
INFLUENCER_WELCOME y SUBSCRIPTION_CONFIRMATION en up y down; no basta renombrar
los archivos experimentales. El down0030 elimina sólo los datos CHANGE_EMAIL;
down0031 restablece el catálogo previo y quita plantillas nuevas.

No aplicar esta secuencia sobre una base que haya ejecutado PR14 bajo sus números
experimentales; necesita diagnóstico y reconciliación específica. Orden de
integración oficial: PR16 → PR17 → PR18 → PR14 actualizado. Los commits de las
dependencias se revisan en sus respectivos PRs. La reparación incluye pruebas
con correos existentes de bienvenida/confirmación durante down/up0030 y las
nuevas plantillas durante rollback0031.

## Mes gratuito desde el primer inicio de sesión (`0032`)

Decisión seleccionada por el usuario el 30/09/2026: mostrar días restantes y
contar un mes calendario de Argentina desde el primer acceso, sin reiniciarlo
al reautenticar, cancelar o reintentar. La primera autenticación se persiste
junto con su sesión; la marca conserva ese origen. El día se limita al último
día del mes siguiente (31/01 → 28/02, o 29/02 en año bisiesto).

Los nuevos checkouts usan `auto_recurring.start_date` con el fin fijo de prueba,
sin sumar otro `free_trial`. Si el período expiró, no se concede tiempo extra.
Las operaciones del proveedor existentes conservan su contrato. Históricamente
la retención elimina sesiones: el backfill `created_at` se marca estimado, no es
una fecha exacta del primer acceso. `TRIAL_START_UNKNOWN` impide usarlo para
nuevos checkouts; exige verificar el origen antes de habilitar pago.

El rechazo HTTP concluyente (400/401/403/422) se registra como CANCELLED con
`checkout_rejected=true` y sin ID de proveedor. Timeout, 409, 429, 5xx o respuesta
incompleta permanecen CREATING; una búsqueda vacía nunca desbloquea otro POST.
Consultar la suscripción busca su referencia exacta y recupera el checkout si
Mercado Pago lo confirma. Una suscripción conocida se consulta por su ID.

Readiness requiere `0032`. Aplicar después de 0031, conservando 0001–0031.
La reversión de 0032 aborta para preservar el primer acceso y los plazos.
Este cambio está en propuesta de integración; testing no se actualiza por editar
este repositorio.


## Códigos públicos de usuario (`0033`)

Aplicar `0033_user_codes` **después** de `0032_first_login_trial` (PR #24).
Esta propuesta requiere reconciliar esa dependencia en main antes de integrar:
no omitir 0032 ni renumerar migraciones aplicadas. Readiness ahora requiere 0033.

`usuarios.codigo_usuario` es nullable y único. Las cuentas anteriores mantienen
el valor NULL y la API devuelve `#USER-` con su ID numérico (mínimo cuatro dígitos).
No hay backfill. Las nuevas altas guardan las iniciales de nombre y apellido
separados, mayúsculas sin tildes, y el contador global del prefijo desde 1.
`allocateUserCode` incrementa `contadores_codigo_usuario` mediante UPSERT atómico
**en la transacción del alta**. Un fallo revierte ambos; no reutilizar códigos
tras una baja ni borrar/reiniciar los contadores. La anonimización conserva el
código y un trigger impide cambiarlo, incluso al editar nombres.

Las PK/FK numéricas y los IDs de marcas/sucursales siguen vigentes. Código público
no es credencial: movimientos consultan sólo clientes activos y mantienen los
controles de membresía/sucursal. `down0033` aborta para preservar reservas; una
reversión funcional debe ser una migración hacia adelante.

Activación: integrar PR24, migración/API de códigos y luego publicar frontend.
Este checkout no implica migración ni despliegue de ningún ambiente.
