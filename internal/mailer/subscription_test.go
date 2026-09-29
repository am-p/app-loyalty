package mailer

import (
	"clientesFrecuentes/internal/model"
	"strings"
	"testing"
	"time"
)

func TestSubscriptionConfirmationReflectsAuthorizationWithoutClaimingPayment(t *testing.T) {
	next := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	m := SubscriptionConfirmationMessage("https://testing.puntazo.pro/", "owner@example.test", model.SubscriptionConfirmationDetails{
		BrandName: "<script>marca</script>", OwnerName: "Gabriel & familia", ProviderID: "provider&status=authorized", ProgramType: "SELLOS", ActiveBranches: 1,
		MonthlyAmountMinor: 2500000, Currency: "ARS", TrialMonths: 1, ConfirmedAt: time.Date(2026, 9, 28, 20, 16, 41, 0, time.UTC), NextPaymentDate: &next,
	})
	for _, want := range []string{"ARS 25.000,00", "28/10/2026", "un mes de prueba gratuito", "No es un comprobante de una cuota pagada", "28/09/2026 17:16", "preapproval_id=provider%26status%3Dauthorized"} {
		if !strings.Contains(m.Text, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(m.HTML, "<script>") || !strings.Contains(m.HTML, "&lt;script&gt;") || !strings.Contains(m.HTML, "Gabriel &amp; familia") {
		t.Fatal("HTML did not escape supplied names")
	}
	if m.Kind != "SUBSCRIPTION_CONFIRMATION" || m.To != "owner@example.test" {
		t.Fatal("wrong email envelope")
	}
}
