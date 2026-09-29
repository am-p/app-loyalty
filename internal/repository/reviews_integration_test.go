package repository_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"clientesFrecuentes/internal/config"
	"clientesFrecuentes/internal/handler"
	"clientesFrecuentes/internal/middleware"
	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/places"
	"clientesFrecuentes/internal/repository"
	"clientesFrecuentes/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func reviewsDB(t *testing.T, historical ...bool) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	admin, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	schema := "reviews_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, e = admin.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS citext;CREATE SCHEMA `+schema); e != nil {
		t.Fatal(e)
	}
	pc, e := pgxpool.ParseConfig(dsn)
	if e != nil {
		t.Fatal(e)
	}
	pc.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, e := pgxpool.NewWithConfig(ctx, pc)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		pool.Close()
		admin.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`)
		admin.Close()
	})
	if _, e = pool.Exec(ctx, `CREATE TABLE schema_migrations(version TEXT PRIMARY KEY,applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); e != nil {
		t.Fatal(e)
	}
	files, e := filepath.Glob("../../migrations/*.up.sql")
	if e != nil {
		t.Fatal(e)
	}
	sort.Strings(files)
	for _, f := range files {
		// Preserve the 0021 fixture for the reviews migration upgrade test.
		// Other tests include later candidate migrations for account compatibility.
		if strings.HasPrefix(filepath.Base(f), "0022") || (len(historical) > 0 && historical[0] && filepath.Base(f)[:4] > "0021") {
			continue
		}
		sql, e := os.ReadFile(f)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = pool.Exec(ctx, string(sql)); e != nil {
			t.Fatalf("%s: %v", f, e)
		}
		if _, e = pool.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES($1)`, filepath.Base(f)[:4]); e != nil {
			t.Fatal(e)
		}
	}
	return pool
}
func applyReviews(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	sql, e := os.ReadFile("../../migrations/0022_google_reviews.up.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = pool.Exec(context.Background(), string(sql)); e != nil {
		t.Fatal(e)
	}
	if _, e = pool.Exec(context.Background(), `INSERT INTO schema_migrations(version) VALUES('0022')`); e != nil {
		t.Fatal(e)
	}
}

type reviewFixture struct{ owner, customer, other, operator, brand, branch, card, program, benefit int64 }

func fixtureReviews(t *testing.T, p *pgxpool.Pool) reviewFixture {
	t.Helper()
	ctx := context.Background()
	var f reviewFixture
	for i, v := range []*int64{&f.owner, &f.customer, &f.other, &f.operator} {
		typ := "PERSONAL_MARCA"
		if i == 1 || i == 2 {
			typ = "CLIENTE_FINAL"
		}
		if e := p.QueryRow(ctx, `INSERT INTO usuarios(email,nombre,tipo_cuenta,qr_hash) VALUES($1,'Test',$2,CASE WHEN $2='CLIENTE_FINAL' THEN $3::bytea ELSE NULL END) RETURNING id`, uuid.NewString()+"@example.test", typ, []byte(uuid.NewString())).Scan(v); e != nil {
			t.Fatal(e)
		}
	}
	must := func(sql string, args ...any) {
		t.Helper()
		if _, e := p.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	if e := p.QueryRow(ctx, `INSERT INTO marcas(nombre) VALUES('Brand') RETURNING id`).Scan(&f.brand); e != nil {
		t.Fatal(e)
	}
	if e := p.QueryRow(ctx, `INSERT INTO sucursales(marca_id,nombre,direccion) VALUES($1,'Main','Street 42') RETURNING id`, f.brand).Scan(&f.branch); e != nil {
		t.Fatal(e)
	}
	must(`INSERT INTO membresias_marca(usuario_id,marca_id,rol) VALUES($1,$3,'PROPIETARIO'),($2,$3,'OPERADOR')`, f.owner, f.operator, f.brand)
	if e := p.QueryRow(ctx, `INSERT INTO programas_fidelidad(marca_id,tipo,sellos_por_acumulacion,nombre_unidad) VALUES($1,'PUNTOS',NULL,'punto') RETURNING id`, f.brand).Scan(&f.program); e != nil {
		t.Fatal(e)
	}
	if e := p.QueryRow(ctx, `INSERT INTO beneficios(programa_id,nombre,requisito_sellos,requisito_puntos) VALUES($1,'Reward',NULL,10) RETURNING id`, f.program).Scan(&f.benefit); e != nil {
		t.Fatal(e)
	}
	if e := p.QueryRow(ctx, `INSERT INTO tarjetas(usuario_id,marca_id) VALUES($1,$2) RETURNING id`, f.customer, f.brand).Scan(&f.card); e != nil {
		t.Fatal(e)
	}
	return f
}
func insertReviewMovement(t *testing.T, p *pgxpool.Pool, f reviewFixture, credit bool) string {
	t.Helper()
	op := uuid.NewString()
	operation, direction, before, after := "ACUMULACION", "CREDITO", 0, 900
	if !credit {
		operation, direction, before, after = "CANJE", "DEBITO", 900, 0
	}
	_, e := p.Exec(context.Background(), `INSERT INTO historial_movimientos(operation_id,tarjeta_id,marca_id,sucursal_id,usuario_operador_id,operacion,sentido,cantidad,saldo_anterior,saldo_posterior,programa_tipo,marca_nombre_snapshot,sucursal_nombre_snapshot,programa_id_snapshot) VALUES($1,$2,$3,$4,$5,$6,$7,900,$8,$9,'PUNTOS','Brand','Main',$10)`, op, f.card, f.brand, f.branch, f.owner, operation, direction, before, after, f.program)
	if e != nil {
		t.Fatal(e)
	}
	return op
}
func enabledReviewInput(threshold int) model.ReviewSettingsInput {
	url := "https://g.page/r/test/review"
	return model.ReviewSettingsInput{Enabled: true, PurchaseThreshold: threshold, Message: "Contanos tu experiencia", DestinationType: "MANUAL_LINK", ManualReviewURL: &url}
}
func TestReviewsMigrationBackfillAndThreshold(t *testing.T) {
	p := reviewsDB(t, true)
	f := fixtureReviews(t, p)
	insertReviewMovement(t, p, f, true)
	insertReviewMovement(t, p, f, true)
	insertReviewMovement(t, p, f, false)
	applyReviews(t, p)
	ctx := context.Background()
	repo := repository.New(p)
	svc := service.New(repo, nil, config.Config{})
	var count int64
	if e := p.QueryRow(ctx, `SELECT purchases FROM progreso_resenas WHERE usuario_id=$1 AND sucursal_id=$2`, f.customer, f.branch).Scan(&count); e != nil || count != 2 {
		t.Fatalf("backfill=%d %v", count, e)
	}
	pending, e := svc.PendingReviews(ctx, f.customer)
	if e != nil || len(pending) != 0 {
		t.Fatalf("retroactive=%v %v", pending, e)
	}
	initial, e := svc.GetReviewSettings(ctx, f.owner, f.brand, f.branch)
	if e != nil || initial.Enabled || initial.Version != 1 || initial.PurchaseThreshold != 1 || initial.PlacesAvailable {
		t.Fatalf("defaults=%+v %v", initial, e)
	}
	settings, e := svc.PutReviewSettings(ctx, f.owner, f.brand, f.branch, 1, enabledReviewInput(2))
	if e != nil {
		t.Fatal(e)
	}
	insertReviewMovement(t, p, f, true)
	pending, e = svc.PendingReviews(ctx, f.customer)
	if e != nil || len(pending) != 0 {
		t.Fatal("threshold below historical count generated")
	}
	settings, e = svc.PutReviewSettings(ctx, f.owner, f.brand, f.branch, settings.Version, enabledReviewInput(4))
	if e != nil {
		t.Fatal(e)
	}
	operation := insertReviewMovement(t, p, f, true)
	pending, e = svc.PendingReviews(ctx, f.customer)
	if e != nil || len(pending) != 1 || pending[0].OperationID != operation {
		t.Fatalf("pending=%+v %v", pending, e)
	}
	insertReviewMovement(t, p, f, true)
	if e = p.QueryRow(ctx, `SELECT count(*) FROM invitaciones_resenas`).Scan(&count); e != nil || count != 1 {
		t.Fatalf("lifetime count=%d %v", count, e)
	}
	if e = repo.CheckSchema(ctx, "0022"); e != nil {
		t.Fatal(e)
	}
	if e = repo.CheckSchema(ctx, "0021"); e == nil {
		t.Fatal("accepted stale schema")
	}
	// Reversible expansion migration was exercised on clean 0021 and historical 0021.
	down, e := os.ReadFile("../../migrations/0022_google_reviews.down.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = p.Exec(ctx, string(down)); e != nil {
		t.Fatal(e)
	}
}
func TestReviewsConcurrencyLeaseEventsAndPermissions(t *testing.T) {
	p := reviewsDB(t)
	f := fixtureReviews(t, p)
	applyReviews(t, p)
	ctx := context.Background()
	repo := repository.New(p)
	svc := service.New(repo, nil, config.Config{})
	for _, actor := range []int64{f.operator, f.other} {
		if _, e := svc.GetReviewSettings(ctx, actor, f.brand, f.branch); e == nil {
			t.Fatalf("unauthorized actor %d", actor)
		}
		if _, e := svc.PutReviewSettings(ctx, actor, f.brand, f.branch, 1, enabledReviewInput(1)); e == nil {
			t.Fatal("unauthorized write")
		}
		if _, e := svc.SearchReviewPlaces(ctx, actor, f.brand, f.branch, "coffee"); e == nil || errors.Is(e, service.ErrPlacesUnavailable) {
			t.Fatal("provider before auth")
		}
		if _, e := repo.ReviewMetrics(ctx, actor, f.brand, f.branch); e == nil {
			t.Fatal("unauthorized metrics")
		}
	}
	if _, e := svc.GetReviewSettings(ctx, f.owner, f.brand+1, f.branch); !errors.Is(e, repository.ErrNotFound) {
		t.Fatalf("tenant mismatch=%v", e)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := svc.PutReviewSettings(ctx, f.owner, f.brand, f.branch, 1, enabledReviewInput(1))
			results <- e
		}()
	}
	wg.Wait()
	close(results)
	success, stale := 0, 0
	for e := range results {
		if e == nil {
			success++
		} else if errors.Is(e, repository.ErrPreconditionFailed) {
			stale++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || stale != 1 {
		t.Fatalf("initial writes success=%d stale=%d", success, stale)
	}
	insertReviewMovement(t, p, f, true)
	pending, e := svc.PendingReviews(ctx, f.customer)
	if e != nil || len(pending) != 1 {
		t.Fatalf("pending=%v %v", pending, e)
	}
	id := pending[0].ID
	if _, e = svc.ReserveReview(ctx, f.other, id); !errors.Is(e, repository.ErrNotFound) {
		t.Fatalf("foreign reserve=%v", e)
	}
	leases := make(chan model.ReviewReservation, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := svc.ReserveReview(ctx, f.customer, id)
			if e == nil {
				leases <- v
			} else {
				errs <- e
			}
		}()
	}
	wg.Wait()
	close(leases)
	close(errs)
	if len(leases) != 1 {
		t.Fatalf("lease winners=%d", len(leases))
	}
	for e := range errs {
		if !errors.Is(e, repository.ErrConflict) {
			t.Fatal(e)
		}
	}
	lease := <-leases
	if delta := time.Until(lease.ExpiresAt); delta < 58*time.Second || delta > 61*time.Second {
		t.Fatalf("lease duration=%v", delta)
	}
	if pending, e = svc.PendingReviews(ctx, f.customer); e != nil || len(pending) != 0 {
		t.Fatal("reserved visible")
	}
	if _, e = svc.ReviewEvent(ctx, f.customer, id, model.ReviewEventRequest{ReservationToken: lease.ReservationToken, Event: "CLIC"}); !errors.Is(e, repository.ErrConflict) {
		t.Fatal("click before render")
	}
	if _, e = p.Exec(ctx, `UPDATE invitaciones_resenas SET lease_until=now()-interval '1 second' WHERE id=$1`, id); e != nil {
		t.Fatal(e)
	}
	if _, e = svc.ReviewEvent(ctx, f.customer, id, model.ReviewEventRequest{ReservationToken: lease.ReservationToken, Event: "PRESENTADA"}); !errors.Is(e, repository.ErrConflict) {
		t.Fatal("expired render")
	}
	replacement, e := svc.ReserveReview(ctx, f.customer, id)
	if e != nil || replacement.ReservationToken == lease.ReservationToken {
		t.Fatalf("replacement=%+v %v", replacement, e)
	}
	if _, e = svc.ReviewEvent(ctx, f.customer, id, model.ReviewEventRequest{ReservationToken: lease.ReservationToken, Event: "PRESENTADA"}); !errors.Is(e, repository.ErrConflict) {
		t.Fatal("old token accepted")
	}
	request := model.ReviewEventRequest{ReservationToken: replacement.ReservationToken, Event: "PRESENTADA"}
	event, e := svc.ReviewEvent(ctx, f.customer, id, request)
	if e != nil || !event.Shown {
		t.Fatalf("shown=%+v %v", event, e)
	}
	if _, e = p.Exec(ctx, `UPDATE invitaciones_resenas SET lease_until=now()-interval '1 second' WHERE id=$1`, id); e != nil {
		t.Fatal(e)
	}
	for _, kind := range []string{"PRESENTADA", "OMITIDA", "CLIC", "CLIC", "OMITIDA"} {
		request.Event = kind
		if _, e = svc.ReviewEvent(ctx, f.customer, id, request); e != nil {
			t.Fatalf("event %s=%v", kind, e)
		}
	}
	metrics, e := repo.ReviewMetrics(ctx, f.owner, f.brand, f.branch)
	if e != nil || metrics.Shown != 1 || metrics.Skipped != 1 || metrics.Clicked != 1 {
		t.Fatalf("metrics=%+v %v", metrics, e)
	}
	if _, e = svc.ReserveReview(ctx, f.customer, id); !errors.Is(e, repository.ErrConflict) {
		t.Fatal("shown replay reserved")
	}
	export, e := repo.ExportAccount(ctx, f.customer)
	if e != nil || len(export.ReviewProgress) != 1 || len(export.ReviewInvitations) != 1 {
		t.Fatalf("export=%+v %v", export, e)
	}
	if _, e = repo.AnonymizeAccount(ctx, f.customer, export.User.Version); e != nil {
		t.Fatal(e)
	}
	var associated int
	if e = p.QueryRow(ctx, `SELECT (SELECT count(*) FROM invitaciones_resenas WHERE usuario_id=$1 OR tarjeta_id=$2 OR operation_id IS NOT NULL)+(SELECT count(*) FROM progreso_resenas WHERE usuario_id=$1)`, f.customer, f.card).Scan(&associated); e != nil || associated != 0 {
		t.Fatalf("associated=%d %v", associated, e)
	}
	metrics, e = repo.ReviewMetrics(ctx, f.owner, f.brand, f.branch)
	if e != nil || metrics.Clicked != 1 {
		t.Fatal("anonymous metrics lost")
	}
}
func TestReviewsDisableCancelsLifetimeInvitation(t *testing.T) {
	for _, branchDisable := range []bool{false, true} {
		t.Run(map[bool]string{false: "settings", true: "branch"}[branchDisable], func(t *testing.T) {
			p := reviewsDB(t)
			f := fixtureReviews(t, p)
			applyReviews(t, p)
			ctx := context.Background()
			repo := repository.New(p)
			svc := service.New(repo, nil, config.Config{})
			in := enabledReviewInput(1)
			setting, e := svc.PutReviewSettings(ctx, f.owner, f.brand, f.branch, 1, in)
			if e != nil {
				t.Fatal(e)
			}
			insertReviewMovement(t, p, f, true)
			pending, e := svc.PendingReviews(ctx, f.customer)
			if e != nil || len(pending) != 1 {
				t.Fatal(e)
			}
			lease, e := svc.ReserveReview(ctx, f.customer, pending[0].ID)
			if e != nil {
				t.Fatal(e)
			}
			if branchDisable {
				_, e = p.Exec(ctx, `UPDATE sucursales SET activo=false WHERE id=$1`, f.branch)
			} else {
				in.Enabled = false
				_, e = svc.PutReviewSettings(ctx, f.owner, f.brand, f.branch, setting.Version, in)
			}
			if e != nil {
				t.Fatal(e)
			}
			if _, e = svc.ReviewEvent(ctx, f.customer, lease.Invitation.ID, model.ReviewEventRequest{ReservationToken: lease.ReservationToken, Event: "PRESENTADA"}); !errors.Is(e, repository.ErrConflict) {
				t.Fatalf("disabled presentation=%v", e)
			}
			if branchDisable {
				_, e = p.Exec(ctx, `UPDATE sucursales SET activo=true WHERE id=$1`, f.branch)
			} else {
				in.Enabled = true
				_, e = svc.PutReviewSettings(ctx, f.owner, f.brand, f.branch, setting.Version+1, in)
			}
			if e != nil {
				t.Fatal(e)
			}
			pending, e = svc.PendingReviews(ctx, f.customer)
			if e != nil || len(pending) != 0 {
				t.Fatal("cancelled revived")
			}
			in.PurchaseThreshold = 2
			in.Enabled = true
			current, e := svc.GetReviewSettings(ctx, f.owner, f.brand, f.branch)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = svc.PutReviewSettings(ctx, f.owner, f.brand, f.branch, current.Version, in); e != nil {
				t.Fatal(e)
			}
			insertReviewMovement(t, p, f, true)
			var count int
			if e = p.QueryRow(ctx, `SELECT count(*) FROM invitaciones_resenas WHERE usuario_id=$1`, f.customer).Scan(&count); e != nil || count != 1 {
				t.Fatalf("lifetime count=%d %v", count, e)
			}
		})
	}
}

func TestReviewsConcurrentCreditsCountEachPurchase(t *testing.T) {
	p := reviewsDB(t)
	f := fixtureReviews(t, p)
	applyReviews(t, p)
	ctx := context.Background()
	repo := repository.New(p)
	svc := service.New(repo, nil, config.Config{})
	if _, e := svc.PutReviewSettings(ctx, f.owner, f.brand, f.branch, 1, enabledReviewInput(6)); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); insertReviewMovement(t, p, f, true) }()
	}
	wg.Wait()
	var count, total int
	if e := p.QueryRow(ctx, `SELECT purchases,(SELECT count(*) FROM invitaciones_resenas WHERE usuario_id=$1 AND sucursal_id=$2) FROM progreso_resenas WHERE usuario_id=$1 AND sucursal_id=$2`, f.customer, f.branch).Scan(&count, &total); e != nil || count != 12 || total != 1 {
		t.Fatalf("concurrent purchases=%d invitations=%d err=%v", count, total, e)
	}
}

type reviewProviderFake struct {
	fail  bool
	calls int
}

func (f *reviewProviderFake) Available() bool { return true }
func (f *reviewProviderFake) Search(context.Context, string) ([]model.ReviewPlace, error) {
	f.calls++
	if f.fail {
		return nil, places.ErrUnavailable
	}
	return []model.ReviewPlace{}, nil
}
func (f *reviewProviderFake) Resolve(context.Context, string) (string, error) {
	f.calls++
	if f.fail {
		return "", places.ErrUnavailable
	}
	return "https://g.page/r/test/review", nil
}
func TestReviewProviderFailureDefersWithoutLosingSettings(t *testing.T) {
	p := reviewsDB(t)
	f := fixtureReviews(t, p)
	applyReviews(t, p)
	ctx := context.Background()
	svc := service.New(repository.New(p), nil, config.Config{})
	provider := &reviewProviderFake{fail: true}
	svc.Places = provider
	placeID := "ChIJ_test"
	in := model.ReviewSettingsInput{Enabled: true, PurchaseThreshold: 1, Message: "Contanos", DestinationType: "GOOGLE_PLACE", GooglePlaceID: &placeID}
	settings, e := svc.PutReviewSettings(ctx, f.owner, f.brand, f.branch, 1, in)
	if e != nil || settings.ReviewURL != nil || settings.GooglePlaceID == nil || !settings.PlacesAvailable || settings.Version != 2 {
		t.Fatalf("editable settings=%+v %v", settings, e)
	}
	calls := provider.calls
	insertReviewMovement(t, p, f, true)
	if provider.calls != calls {
		t.Fatal("provider called by movement")
	}
	pending, e := svc.PendingReviews(ctx, f.customer)
	if e != nil || len(pending) != 0 {
		t.Fatalf("provider pending=%v %v", pending, e)
	}
	var id string
	if e = p.QueryRow(ctx, `SELECT id::text FROM invitaciones_resenas WHERE usuario_id=$1`, f.customer).Scan(&id); e != nil {
		t.Fatal(e)
	}
	if _, e = svc.ReserveReview(ctx, f.customer, id); !errors.Is(e, service.ErrPlacesUnavailable) {
		t.Fatalf("reserve provider error=%v", e)
	}
	var reserved bool
	if e = p.QueryRow(ctx, `SELECT reservation_token IS NOT NULL FROM invitaciones_resenas WHERE id=$1`, id).Scan(&reserved); e != nil || reserved {
		t.Fatal("provider failure leased invitation")
	}
	provider.fail = false
	pending, e = svc.PendingReviews(ctx, f.customer)
	if e != nil || len(pending) != 1 {
		t.Fatalf("recovery pending=%+v %v", pending, e)
	}
	if _, e = svc.ReserveReview(ctx, f.customer, id); e != nil {
		t.Fatal(e)
	}
	var storedID string
	var storedURL *string
	if e = p.QueryRow(ctx, `SELECT google_place_id,manual_review_url FROM configuracion_resenas WHERE sucursal_id=$1`, f.branch).Scan(&storedID, &storedURL); e != nil || storedID != placeID || storedURL != nil {
		t.Fatal("provider data persisted beyond ID")
	}
}
func TestReviewHTTPSettingsContractAndPermission(t *testing.T) {
	p := reviewsDB(t)
	f := fixtureReviews(t, p)
	applyReviews(t, p)
	svc := service.New(repository.New(p), nil, config.Config{})
	h := &handler.Handler{Repo: svc.Repo, Service: svc, Limiter: middleware.NewRateLimiter()}
	route := gin.New()
	actorID := f.owner
	route.Use(func(c *gin.Context) {
		c.Set(middleware.ActorKey, middleware.Actor{ID: actorID, AccountType: "PERSONAL_MARCA"})
		c.Next()
	})
	base := "/marcas/:brand_id/sucursales/:resource_id/resenas"
	route.GET(base, h.ReviewSettings)
	route.PUT(base, h.PutReviewSettings)
	route.POST(base+"/busqueda", h.SearchReviewPlaces)
	url := "/marcas/" + fmt.Sprint(f.brand) + "/sucursales/" + fmt.Sprint(f.branch) + "/resenas"
	request := func(method, path, etag string, body []byte) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if etag != "" {
			req.Header.Set("If-Match", etag)
		}
		w := httptest.NewRecorder()
		route.ServeHTTP(w, req)
		return w
	}
	w := request(http.MethodGet, url, "", nil)
	if w.Code != 200 || w.Header().Get("ETag") != `"1"` || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("GET status=%d headers=%v body=%s", w.Code, w.Header(), w.Body.String())
	}
	body, e := json.Marshal(enabledReviewInput(1))
	if e != nil {
		t.Fatal(e)
	}
	for _, etag := range []string{"", `W/"1"`, `"99"`} {
		if w = request(http.MethodPut, url, etag, body); w.Code != 412 {
			t.Fatalf("etag %s status=%d body=%s", etag, w.Code, w.Body.String())
		}
	}
	invalid := bytes.Replace(body, []byte(`"enabled":true`), []byte(`"enabled":true,"version":999`), 1)
	if w = request(http.MethodPut, url, `"1"`, invalid); w.Code != 422 {
		t.Fatalf("unknown fields=%d %s", w.Code, w.Body.String())
	}
	if w = request(http.MethodPut, url, `"1"`, body); w.Code != 200 || w.Header().Get("ETag") != `"2"` {
		t.Fatalf("PUT=%d %s", w.Code, w.Body.String())
	}
	actorID = f.operator
	if w = request(http.MethodGet, url, "", nil); w.Code != 403 {
		t.Fatalf("operator GET=%d %s", w.Code, w.Body.String())
	}
	if w = request(http.MethodPut, url, `"2"`, body); w.Code != 403 {
		t.Fatalf("operator PUT=%d", w.Code)
	}
	provider := &reviewProviderFake{}
	svc.Places = provider
	if w = request(http.MethodPost, url+"/busqueda", "", []byte(`{"query":"coffee"}`)); w.Code != 403 || provider.calls != 0 {
		t.Fatalf("operator search=%d calls=%d", w.Code, provider.calls)
	}
	// Administrator is accepted server-side; cross-brand actors remain hidden.
	if _, e = p.Exec(context.Background(), `UPDATE membresias_marca SET rol='ADMINISTRADOR' WHERE usuario_id=$1`, f.operator); e != nil {
		t.Fatal(e)
	}
	if w = request(http.MethodGet, url, "", nil); w.Code != 200 {
		t.Fatalf("administrator=%d", w.Code)
	}
	actorID = f.owner
	provider.fail = true
	if w = request(http.MethodPost, url+"/busqueda", "", []byte(`{"query":"coffee"}`)); w.Code != 503 || !strings.Contains(w.Body.String(), "DEPENDENCY_UNAVAILABLE") {
		t.Fatalf("provider HTTP=%d %s", w.Code, w.Body.String())
	}
	provider.fail = false
	for i := 0; i < 19; i++ {
		if w = request(http.MethodPost, url+"/busqueda", "", []byte(`{"query":"coffee"}`)); w.Code != 200 {
			t.Fatalf("search quota before limit=%d", w.Code)
		}
	}
	if w = request(http.MethodPost, url+"/busqueda", "", []byte(`{"query":"coffee"}`)); w.Code != 429 || w.Header().Get("Retry-After") == "" || provider.calls != 20 {
		t.Fatalf("quota HTTP=%d calls=%d retry=%s", w.Code, provider.calls, w.Header().Get("Retry-After"))
	}
	actorID = f.other
	if w = request(http.MethodGet, url, "", nil); w.Code != 404 {
		t.Fatalf("cross tenant=%d", w.Code)
	}
}

func TestReviewsCountWhileDisabledNoRetroactiveInvitation(t *testing.T) {
	p := reviewsDB(t)
	f := fixtureReviews(t, p)
	applyReviews(t, p)
	ctx := context.Background()
	svc := service.New(repository.New(p), nil, config.Config{})
	insertReviewMovement(t, p, f, true)
	in := enabledReviewInput(1)
	setting, e := svc.PutReviewSettings(ctx, f.owner, f.brand, f.branch, 1, in)
	if e != nil {
		t.Fatal(e)
	}
	pending, e := svc.PendingReviews(ctx, f.customer)
	if e != nil || len(pending) != 0 {
		t.Fatal("enable generated retroactive invitation")
	}
	insertReviewMovement(t, p, f, true)
	pending, e = svc.PendingReviews(ctx, f.customer)
	if e != nil || len(pending) != 0 {
		t.Fatal("disabled history ignored")
	}
	in.PurchaseThreshold = 3
	if _, e = svc.PutReviewSettings(ctx, f.owner, f.brand, f.branch, setting.Version, in); e != nil {
		t.Fatal(e)
	}
	insertReviewMovement(t, p, f, true)
	pending, e = svc.PendingReviews(ctx, f.customer)
	if e != nil || len(pending) != 1 {
		t.Fatalf("historical threshold=%v %v", pending, e)
	}
}
