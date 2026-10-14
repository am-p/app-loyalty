# Mercado Pago: preparación de producción con cobros apagados

Esta guía prepara la integración de suscripciones mensuales sin habilitarla.
La plantilla [de configuración](../deploy/mercadopago.production.env.example)
es un overlay de facturación, no un archivo de entorno completo ni un despliegue.
No incluye credenciales ni modifica precios en la base.

## Preparación sin cobros

1. Seleccionar `am-p/app-loyalty:main`, hacer fetch y resolver su HEAD remoto
   vigente. Construir desde un checkout limpio verificado con `verify-context.py
   --require-clean`; no reutilizar el SHA de un informe anterior. Conciliar las
   migraciones con la base desplegada, respaldar y ensayar el upgrade antes de
   publicar. `EXPECTED_SCHEMA_VERSION` debe corresponder a las migraciones del
   árbol que se publica; no bajar la versión a la de una guía histórica.
2. Conservar `MERCADO_PAGO_PROVIDER=disabled` en Coolify. Mantener vacíos el
   access token y el secreto del webhook en producción hasta la preparación de
   credenciales. Guardarlos después como secretos exclusivos del backend nunca
   los activa por sí solo: el proveedor sigue apagado hasta seleccionar `api` y
   reiniciar. La opción más simple durante esta etapa es dejarlos vacíos.
3. Preparar `PUBLIC_APP_URL=https://puntazo.pro`, API de Mercado Pago HTTPS y el
   timeout de 10 segundos. Mantener prorrateo y simulador apagados; el bootstrap
   rechaza el prorrateo en modo producción porque todavía no está certificado.
4. Preparar el modo completo `APP_ENV=production` por separado: el código actual
   exige verificación de email, SMTP, una clave independiente para cifrar el
   outbox, S3 con HTTPS/AES256 y Redis con TLS. No cambiar sólo `APP_ENV` en un
   runtime que todavía no cumple esos requisitos.
5. Confirmar los precios comerciales antes de la activación. Los defaults son
   Sellos ARS 25.000 y Puntos ARS 20.000 por sucursal activa; `subscription_prices`
   los reemplaza cuando existe una fila. Verificar tanto la pantalla de registro
   como el importe de Plan y facturación. No copiar precios, usuarios,
   suscripciones, reservas `CREATING` ni datos de testing a producción.

Con el proveedor apagado, el bootstrap no crea el cliente de cobros y el worker
de cambios de precios no consulta Mercado Pago. Checkout y cancelación devuelven
`503 BILLING_UNAVAILABLE`; `provider_configured=false` deshabilita esas acciones
en la interfaz. El webhook devuelve `200` vacío y no procesa ni persiste eventos.
Ese `200` sólo confirma recepción: no valida la firma ni demuestra un cobro.
No enviar ni redirigir eventos de una integración activa a este receptor apagado.
Apagar Puntazo tampoco cancela suscripciones ya autorizadas en Mercado Pago.

## Contrato del webhook habilitado

- URL directa: `https://api.puntazo.pro/v1/mercado-pago/webhooks`.
- Tópicos usados: `subscription_preapproval`, `subscription_authorized_payment`
  y `payment`. No se crean planes asociados; `subscription_preapproval_plan`
  no es parte del flujo actual.
- Firma `x-signature` y `x-request-id`, con identificación del recurso en
  `data.id`. El cuerpo y la query deben identificar el mismo recurso.
- HTTP `200` vacío después de completar la transacción, para duplicados ya
  procesados o para tópicos no soportados con firma válida. Firma inválida:
  `401`; payload inválido: `422`; fallo del proveedor: `503`. No confirmar un
  fallo como éxito: Mercado Pago debe poder reintentar.
- Se consulta el recurso al proveedor; el estado recibido en el JSON del
  webhook o en el retorno del navegador no acredita autorización ni pago.
- El retorno se construye como
  `https://puntazo.pro/suscripcion/resultado`; probar el parámetro
  `preapproval_id`, recuperación de sesión y consulta autoritativa.

La [documentación de Webhooks](https://www.mercadopago.com.ar/developers/es/docs/subscriptions/additional-content/your-integrations/notifications/webhooks)
exige `200` o `201` para confirmar recepción. Esa misma página contiene una
excepción al método por panel para Suscripciones y también enumera sus tópicos.
El cliente actual de `POST /preapproval` no envía `notification_url`.
Antes de activar, validar con la aplicación seleccionada la vía admitida para
notificaciones de suscripciones y una entrega firmada real; guardar una URL en
el panel no certifica el circuito. No añadir un campo no verificado ni aceptar
IPN sin firma como reemplazo de Webhooks.

## Activación posterior, fuera de esta preparación

1. Elegir la cuenta receptora real y su aplicación de suscripciones. Activar las
   [credenciales productivas](https://www.mercadopago.com.ar/developers/es/docs/subscriptions/additional-content/your-integrations/credentials)
   en Mercado Pago con el sitio `https://puntazo.pro`. Verificar la identidad y
   país de la cuenta; el prefijo del token no distingue por sí solo una cuenta
   productiva de una cuenta de prueba. La conexión MCP puede corresponder a otra
   aplicación y no certifica la configuración del backend.
2. Guardar el token y el secreto de esa aplicación en Coolify por su API/UI
   oficial, sin escribirlos en Git, frontend, logs, capturas o informes.
3. En un ambiente aislado, comprobar checkout, retorno, firma válida/inválida,
   repetición de eventos, rechazo, cancelación y cambios de importe. Mantener
   separados tokens, secretos, callback y datos de testing y producción.
4. Con autorización explícita de activar cobros, resolver otra vez el HEAD
   remoto, verificar el checkout y configurar `MERCADO_PAGO_PROVIDER=api`.
   Validar `/v1/health/ready`, `/v1/version`, esquema y `provider_configured=true`.
5. Hacer una operación real controlada autorizada y conciliar autorización,
   cuota efectivamente aprobada, primer cobro/fin de prueba y renovación.
   `AUTHORIZED` no acredita cada cuota. No reiniciar el mes gratis al contratar:
   se conserva el fin del primer mes calendario desde el primer acceso.

Ante una falla de activación, detener altas nuevas y conciliar las suscripciones
en Mercado Pago antes de apagar el proveedor: una baja sólo en configuración no
detiene renovaciones externas. Conservar callbacks y reservas pendientes hasta
resolverlos. No cambiar precios de suscripciones existentes como parte del
rollback. La vuelta a la imagen anterior se realiza por Coolify sin revertir
migraciones aplicadas.
