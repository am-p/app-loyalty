package repository_test

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"clientesFrecuentes/internal/repository"
	"github.com/google/uuid"
)

func TestPostgresSubscriptionPriceHistory(t *testing.T) {
	pool := referralBillingPool(t)
	ctx := t.Context()
	repo := repository.New(pool)
	empty, err := repo.ListSubscriptionPriceChanges(ctx, 20, nil)
	if err != nil || empty.Items == nil || len(empty.Items) != 0 || empty.NextCursor != nil {
		t.Fatalf("empty=%+v err=%v", empty, err)
	}
	var admin int64
	if err = pool.QueryRow(ctx, `INSERT INTO backoffice_users(email,password_hash,totp_secret,role) VALUES('history-admin@example.test','hash','secret','ADMIN_SISTEMA') RETURNING id`).Scan(&admin); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		var brand int64
		if err = pool.QueryRow(ctx, `INSERT INTO marcas(nombre) VALUES('History client') RETURNING id`).Scan(&brand); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `INSERT INTO suscripciones_marca(marca_id,referencia_externa,proveedor_suscripcion_id,proveedor,estado,moneda,precio_sucursal_minor,importe_mensual_minor,cantidad_sucursales,program_type)
 VALUES($1::bigint,'history-'||$1::bigint::text,'provider-'||$1::bigint::text,'MERCADO_PAGO','AUTHORIZED','ARS',10000,10000,1,'SELLOS')`, brand); err != nil {
			t.Fatal(err)
		}
	}
	first, err := repo.ChangeSubscriptionPrice(ctx, admin, uuid.New(), "SELLOS", 11000, 0, 10000, false)
	if err != nil || first.PreviousUnitPriceMinor == nil || *first.PreviousUnitPriceMinor != 10000 || first.AdminEmail != "history-admin@example.test" {
		t.Fatalf("snapshot=%+v err=%v", first, err)
	}
	points, err := repo.ChangeSubscriptionPrice(ctx, admin, uuid.New(), "PUNTOS", 21000, 0, 20000, false)
	if err != nil {
		t.Fatal(err)
	}
	last, err := repo.ChangeSubscriptionPrice(ctx, admin, uuid.New(), "SELLOS", 12000, 1, 99999, true)
	if err != nil || last.PreviousUnitPriceMinor == nil || *last.PreviousUnitPriceMinor != 11000 {
		t.Fatalf("previous=%+v err=%v", last, err)
	}
	// Exercise all persisted outcomes in the summary without contacting a provider.
	if _, err = pool.Exec(ctx, `WITH outcomes AS (SELECT brand_id,row_number() OVER(ORDER BY brand_id) AS n FROM subscription_price_change_items WHERE change_id=$1)
 UPDATE subscription_price_change_items i SET status=CASE o.n WHEN 1 THEN 'APPLIED' WHEN 2 THEN 'PENDING' WHEN 3 THEN 'FAILED' ELSE 'SKIPPED' END
 FROM outcomes o WHERE i.change_id=$1 AND i.brand_id=o.brand_id`, last.ID); err != nil {
		t.Fatal(err)
	}
	all, err := repo.ListSubscriptionPriceChanges(ctx, 20, nil)
	if err != nil || len(all.Items) != 3 || all.Items[0].ID != last.ID {
		t.Fatalf("history=%+v err=%v", all, err)
	}
	entry := all.Items[0]
	if entry.Total != 4 || entry.Applied != 1 || entry.Pending != 1 || entry.Failed != 1 || entry.Skipped != 1 || !entry.IncludeExisting || entry.PreviousUnitPriceMinor == nil || *entry.PreviousUnitPriceMinor != 11000 {
		t.Fatalf("counts=%+v", entry)
	}
	if all.Items[1].ProgramType != "PUNTOS" || all.Items[1].Total != 0 || all.Items[1].IncludeExisting {
		t.Fatalf("points=%+v", all.Items[1])
	}
	// Force equal timestamps to prove that UUIDs break ties across page boundaries.
	if _, err = pool.Exec(ctx, `UPDATE subscription_price_changes SET created_at='2026-09-28T00:00:00Z'`); err != nil {
		t.Fatal(err)
	}
	expected := []string{first.ID, points.ID, last.ID}
	sort.Sort(sort.Reverse(sort.StringSlice(expected)))
	seen := make([]string, 0, 3)
	var cursor *uuid.UUID
	for i := 0; i < 3; i++ {
		page, err := repo.ListSubscriptionPriceChanges(ctx, 1, cursor)
		if err != nil || len(page.Items) != 1 {
			t.Fatalf("page=%+v err=%v", page, err)
		}
		seen = append(seen, page.Items[0].ID)
		if i < 2 {
			if page.NextCursor == nil {
				t.Fatal("missing cursor")
			}
			value := uuid.MustParse(*page.NextCursor)
			cursor = &value
		} else if page.NextCursor != nil {
			t.Fatal("unexpected last cursor")
		}
	}
	for i := range expected {
		if seen[i] != expected[i] {
			t.Fatalf("ordering=%v expected=%v", seen, expected)
		}
	}
	for _, limit := range []int{0, 51} {
		if _, err = repo.ListSubscriptionPriceChanges(ctx, limit, nil); !errors.Is(err, repository.ErrInvalidRequest) {
			t.Fatalf("limit=%d err=%v", limit, err)
		}
	}
	unknown := uuid.New()
	if _, err = repo.ListSubscriptionPriceChanges(ctx, 20, &unknown); !errors.Is(err, repository.ErrInvalidRequest) {
		t.Fatalf("cursor err=%v", err)
	}
	// Simulate upgrading previously recorded changes: recover known versions,
	// but do not invent the unsaved environment price of a legacy first change.
	for _, suffix := range []string{"down", "up"} {
		migration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "0026_subscription_price_change_history."+suffix+".sql"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, string(migration)); err != nil {
			t.Fatal(err)
		}
	}
	legacyFirst, err := repo.SubscriptionPriceChange(ctx, uuid.MustParse(first.ID))
	if err != nil || legacyFirst.PreviousUnitPriceMinor != nil {
		t.Fatalf("legacy first=%+v err=%v", legacyFirst, err)
	}
	legacyLast, err := repo.SubscriptionPriceChange(ctx, uuid.MustParse(last.ID))
	if err != nil || legacyLast.PreviousUnitPriceMinor == nil || *legacyLast.PreviousUnitPriceMinor != 11000 {
		t.Fatalf("legacy version=%+v err=%v", legacyLast, err)
	}
}
