package repository_test

import (
	"bytes"
	"clientesFrecuentes/internal/config"
	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/repository"
	"clientesFrecuentes/internal/service"
	"context"
	"errors"
	"github.com/google/uuid"
	"sync"
	"testing"
)

func TestPostgresPointsCreditRedemptionIdempotencyAndTenantInvariants(t *testing.T) {
	p := reviewsDB(t)
	applyReviews(t, p)
	f := fixtureReviews(t, p)
	other := fixtureReviews(t, p)
	ctx := context.Background()
	r := repository.New(p)
	s := service.New(r, nil, config.Config{QRPepper: "12345678901234567890123456789012"})
	token, hash := s.QRForUser(f.customer)
	if _, e := p.Exec(ctx, `UPDATE usuarios SET qr_hash=$2 WHERE id=$1;`, f.customer, hash); e != nil {
		t.Fatal(e)
	}
	if _, e := p.Exec(ctx, `INSERT INTO accesos_demo(marca_id,tipo,precio_minor,moneda,cobro_automatico) VALUES($1,'PUNTOS_FREE_TRIAL',0,'ARS',false),($2,'PUNTOS_FREE_TRIAL',0,'ARS',false)`, f.brand, other.brand); e != nil {
		t.Fatal(e)
	}
	redeemReq := model.MovementPreviewRequest{Operation: "CANJE", QRToken: token, BranchID: f.branch, BenefitID: &f.benefit}
	if _, e := s.Preview(ctx, f.owner, redeemReq); !errors.Is(e, repository.ErrInsufficientBalance) {
		t.Fatalf("zero points redemption must reject insufficient balance: %v", e)
	}
	amount := int64(10)
	creditReq := model.MovementPreviewRequest{Operation: "ACUMULACION", QRToken: token, BranchID: f.branch, PointsAmount: &amount}
	credit := func() (model.Preview, model.ConfirmAccumulationRequest) {
		preview, e := s.Preview(ctx, f.owner, creditReq)
		if e != nil {
			t.Fatal(e)
		}
		return preview, model.ConfirmAccumulationRequest{PreviewID: preview.ID, QRToken: token, BranchID: f.branch}
	}
	_, confirmation := credit()
	key := uuid.NewString()
	var wg sync.WaitGroup
	results := make(chan repository.IdempotentResult, 4)
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, e := s.ConfirmAccumulation(ctx, f.owner, key, uuid.NewString(), confirmation)
			results <- result
			errs <- e
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var body []byte
	original := 0
	for result := range results {
		if !result.Replayed {
			original++
		}
		if body == nil {
			body = result.Body
		} else if !bytes.Equal(body, result.Body) {
			t.Fatal("concurrent replay body differs")
		}
	}
	if original != 1 {
		t.Fatal("credit originals", original)
	}
	preview, e := s.Preview(ctx, f.owner, redeemReq)
	if e != nil {
		t.Fatal("points redemption preview", e)
	}
	redeem := model.ConfirmRedemptionRequest{PreviewID: preview.ID, QRToken: token, BranchID: f.branch, BenefitID: f.benefit}
	key = uuid.NewString()
	results = make(chan repository.IdempotentResult, 4)
	errs = make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, e := s.ConfirmRedemption(ctx, f.owner, key, uuid.NewString(), redeem)
			results <- result
			errs <- e
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	original = 0
	body = nil
	for result := range results {
		if !result.Replayed {
			original++
		}
		if body == nil {
			body = result.Body
		} else if !bytes.Equal(body, result.Body) {
			t.Fatal("redemption replay body differs")
		}
	}
	if original != 1 {
		t.Fatal("redemption originals", original)
	}
	_, confirmation = credit()
	if _, e = s.ConfirmAccumulation(ctx, f.owner, uuid.NewString(), uuid.NewString(), confirmation); e != nil {
		t.Fatal(e)
	}
	previews := make([]model.Preview, 2)
	for i := range previews {
		previews[i], e = s.Preview(ctx, f.owner, redeemReq)
		if e != nil {
			t.Fatal(e)
		}
	}
	errs = make(chan error, 2)
	for _, preview := range previews {
		wg.Add(1)
		go func(preview model.Preview) {
			defer wg.Done()
			_, e := s.ConfirmRedemption(ctx, f.owner, uuid.NewString(), uuid.NewString(), model.ConfirmRedemptionRequest{PreviewID: preview.ID, QRToken: token, BranchID: f.branch, BenefitID: f.benefit})
			errs <- e
		}(preview)
	}
	wg.Wait()
	close(errs)
	success := 0
	for e := range errs {
		if e == nil {
			success++
		} else if !errors.Is(e, repository.ErrPreviewChanged) && !errors.Is(e, repository.ErrInsufficientBalance) {
			t.Fatal(e)
		}
	}
	if success != 1 {
		t.Fatal("double redemption", success)
	}
	foreign := creditReq
	foreign.BranchID = other.branch
	if _, e = s.Preview(ctx, f.owner, foreign); !errors.Is(e, repository.ErrNotFound) {
		t.Fatal("cross-brand preview allowed", e)
	}
	if _, e = s.ConfirmAccumulation(ctx, other.owner, uuid.NewString(), uuid.NewString(), confirmation); !errors.Is(e, repository.ErrPreviewChanged) {
		t.Fatal("cross-actor confirmation allowed", e)
	}
	var balance, movements, invalid int64
	if e = p.QueryRow(ctx, `SELECT saldo_puntos FROM tarjetas WHERE id=$1`, f.card).Scan(&balance); e != nil {
		t.Fatal(e)
	}
	if e = p.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE saldo_posterior<0 OR (sentido='CREDITO' AND saldo_posterior-saldo_anterior<>cantidad) OR (sentido='DEBITO' AND saldo_anterior-saldo_posterior<>cantidad)) FROM historial_movimientos WHERE tarjeta_id=$1`, f.card).Scan(&movements, &invalid); e != nil {
		t.Fatal(e)
	}
	if balance != 0 || movements != 4 || invalid != 0 {
		t.Fatalf("ledger balance%d movements%d invalid%d", balance, movements, invalid)
	}
}
