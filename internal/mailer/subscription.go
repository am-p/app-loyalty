package mailer

import (
	"fmt"
	"html"
	"net/url"
	"strings"
	"time"

	"clientesFrecuentes/internal/model"
)

func subscriptionMoney(minor int64, currency string) string {
	whole := fmt.Sprintf("%d", minor/100)
	for i := len(whole) - 3; i > 0; i -= 3 {
		whole = whole[:i] + "." + whole[i:]
	}
	return fmt.Sprintf("%s %s,%02d", currency, whole, minor%100)
}

func SubscriptionConfirmationMessage(appURL, to string, d model.SubscriptionConfirmationDetails) model.EmailMessage {
	zone := time.FixedZone("Argentina", -3*60*60)
	program := "Sellos"
	if d.ProgramType == "PUNTOS" {
		program = "Puntos"
	}
	next := "Consultá el próximo cobro en Plan y facturación."
	if d.NextPaymentDate != nil {
		next = d.NextPaymentDate.In(zone).Format("02/01/2006") + " (hora de Argentina)"
	}
	trial := "Sin período de prueba gratuito."
	if d.TrialMonths > 0 {
		trial = "Al alta se concedió un mes de prueba gratuito."
	}
	amount := subscriptionMoney(d.MonthlyAmountMinor, d.Currency)
	link := strings.TrimRight(appURL, "/") + "/suscripcion/resultado?preapproval_id=" + url.QueryEscape(d.ProviderID)
	note := "Esta confirmación acredita la autorización de la suscripción. No es un comprobante de una cuota pagada. Los cobros se confirman por separado."
	text := fmt.Sprintf("Hola, %s. Tu suscripción de Puntazo está autorizada.\n\nComercio: %s\nPrograma: %s\nSucursales: %d\nImporte mensual autorizado: %s\nAutorización confirmada: %s\nPróximo cobro programado: %s\n%s\nReferencia: %s\n\n%s\n\nConsultar la suscripción: %s", d.OwnerName, d.BrandName, program, d.ActiveBranches, amount, d.ConfirmedAt.In(zone).Format("02/01/2006 15:04"), next, trial, d.ProviderID, note, link)
	content := fmt.Sprintf(`<div style="padding:30px 38px 8px;text-align:center;"><img src="cid:%s" width="128" alt="Mr. Puntazo" style="width:128px;max-width:100%%;height:auto;border:0;"></div>
 <div style="padding:8px 38px 0;"><h1 style="color:#10213a;font-size:26px;">Tu suscripción está autorizada</h1><p>Hola, %s. Ya confirmamos la autorización de tu suscripción.</p></div>
 <div style="padding:20px 38px 0;"><table role="presentation" width="100%%" cellspacing="0" cellpadding="0" style="background:#f5f9fe;border-radius:16px;">%s%s%s%s%s%s</table></div>
 <div style="padding:20px 38px 0;color:#50647e;"><p>%s</p><p>%s</p></div>%s%s`, mascotContentID, html.EscapeString(d.OwnerName), detailRow("Comercio", d.BrandName, true), detailRow("Programa", program, true), detailRow("Sucursales", fmt.Sprint(d.ActiveBranches), true), detailRow("Importe mensual autorizado", amount, true), detailRow("Próximo cobro programado", next, true), detailRow("Referencia", d.ProviderID, false), html.EscapeString(trial), html.EscapeString(note), actionBlock("Consultar mi suscripción", link, "Iniciá sesión con la cuenta propietaria del comercio."), fallbackBlock(link, "Confirmación de suscripción", note))
	return model.EmailMessage{Kind: "SUBSCRIPTION_CONFIRMATION", To: to, Subject: "Tu suscripción de Puntazo está autorizada", Text: text, HTML: emailShell("Suscripción autorizada", "Tu suscripción de Puntazo", "Guardá la referencia y las condiciones de tu suscripción.", content, note)}
}
