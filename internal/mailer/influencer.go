package mailer

import (
	"fmt"
	"html"
	"net/url"
	"strconv"
	"strings"
	"time"

	"clientesFrecuentes/internal/model"
)

func InfluencerWelcomeMessage(appURL, to string, d model.InfluencerWelcomeDetails) model.EmailMessage {
	link := influencerReferralURL(appURL, d.Code)
	program := "Sellos"
	if len(d.ProgramTypes) == 2 {
		program = "Sellos y puntos"
	} else if len(d.ProgramTypes) == 1 && d.ProgramTypes[0] == "PUNTOS" {
		program = "Puntos"
	}
	discount := influencerBenefit(d.DiscountBPS, d.DiscountCharges, "Sin descuento")
	commission := influencerBenefit(d.RewardBPS, d.RewardCharges, "Sin comisión")
	zone := time.FixedZone("Argentina", -3*60*60)
	dates := d.StartsAt.In(zone).Format("02/01/2006 15:04") + " al " + d.EndsAt.In(zone).Format("02/01/2006 15:04") + " (hora de Argentina)"
	state := "El código puede utilizarse durante la vigencia de la campaña, mientras la campaña y el código estén activos."
	if !d.Active {
		state = "La campaña estaba pausada al darte de alta. El código podrá utilizarse cuando se active y esté dentro de su vigencia."
	}
	note := "Tu comisión se calcula sobre el importe efectivamente cobrado a cada comercio referido, después del descuento. Sólo generan comisión los cobros aprobados y verificados; las devoluciones ajustan las comisiones."
	text := fmt.Sprintf("Hola, %s. Ya sos parte de Puntazo.\n\nTu código: %s\nCampaña: %s\nTipo: %s\nDescuento para el comercio: %s\nTu comisión por comercio: %s\nVigencia: %s\n\n%s\n\n%s\n\nCompartí este enlace: %s\n\nEstas son las condiciones de la campaña al darte de alta. El comercio verá las condiciones vigentes antes de contratar.", d.Name, d.Code, d.CampaignName, program, discount, commission, dates, state, note, link)
	content := fmt.Sprintf(`
 <div style="padding:30px 38px 8px;text-align:center;"><img src="cid:%s" width="128" alt="Mr. Puntazo te da la bienvenida" style="width:128px;max-width:100%%;height:auto;border:0;"></div>
 <div style="padding:8px 38px 0;text-align:center;">
 <p style="margin:0 0 10px;color:#1687ff;font-size:13px;font-weight:800;text-transform:uppercase;letter-spacing:1px;">Programa de referidos</p>
 <h1 style="margin:0;color:#10213a;font-size:30px;line-height:1.18;">¡Hola, %s!</h1>
 <p style="margin:16px 0;color:#50647e;font-size:16px;line-height:1.55;">Ya sos parte de Puntazo. Compartí tu código con los comercios y acompañalos a crecer.</p>
 <div style="padding:18px;background:#e8f3ff;border:1px dashed #1687ff;border-radius:14px;">
 <p style="margin:0 0 8px;color:#50647e;font-size:13px;">TU CÓDIGO DE REFERIDO</p>
 <p style="margin:0;color:#10213a;font-size:26px;font-weight:800;letter-spacing:1px;overflow-wrap:anywhere;">%s</p></div></div>
 <div style="padding:26px 38px 0;"><table role="presentation" width="100%%" cellspacing="0" cellpadding="0" border="0" style="width:100%%;background:#f5f9fe;border:1px solid #dbe8f7;border-radius:16px;">%s%s%s%s%s</table></div>
 <div style="padding:20px 38px 0;color:#50647e;font-size:14px;line-height:1.55;"><p style="margin:0 0 12px;">%s</p><p style="margin:0;">%s</p></div>
 %s
 %s`, mascotContentID, html.EscapeString(d.Name), html.EscapeString(d.Code),
		detailRow("Campaña", d.CampaignName, true), detailRow("Tipo de campaña", program, true),
		detailRow("Descuento para el comercio", discount, true), detailRow("Tu comisión por comercio", commission, true), detailRow("Vigencia", dates, false),
		html.EscapeString(state), html.EscapeString(note),
		actionBlock("Ver el enlace para compartir", link, "Copiá este enlace y compartilo con los comercios. El código se cargará al iniciar el registro."),
		fallbackBlock(link, "Condiciones al darte de alta", "El comercio verá las condiciones vigentes antes de contratar. Guardá este correo para consultar tu código y los beneficios de la campaña."))
	return model.EmailMessage{Kind: "INFLUENCER_WELCOME", To: to, Subject: "Tu código y beneficios en Puntazo", Text: text,
		HTML: emailShell("Tu código y beneficios en Puntazo", "Bienvenida a Puntazo", "Tu código de referido y las condiciones de tu campaña ya están listos.", content, "Este correo confirma tu alta en el programa de referidos de Puntazo.")}
}

func influencerBenefit(bps, charges int, zero string) string {
	if bps == 0 || charges == 0 {
		return zero
	}
	percent := strconv.Itoa(bps / 100)
	if bps%100 != 0 {
		percent += "," + strings.TrimRight(fmt.Sprintf("%02d", bps%100), "0")
	}
	if charges == 1 {
		return percent + "% en el primer cobro"
	}
	return fmt.Sprintf("%s%% en los primeros %d cobros", percent, charges)
}

func influencerReferralURL(base, code string) string {
	parsed, err := url.Parse(base)
	if err != nil {
		return base
	}
	query := parsed.Query()
	query.Set("ref", code)
	parsed.RawQuery = query.Encode()
	parsed.Fragment = ""
	return parsed.String()
}
