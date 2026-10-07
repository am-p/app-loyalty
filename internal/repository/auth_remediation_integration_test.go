package repository_test

import (
	"clientesFrecuentes/internal/auth"
	"clientesFrecuentes/internal/config"
	"clientesFrecuentes/internal/model"
	"clientesFrecuentes/internal/repository"
	"clientesFrecuentes/internal/service"
	"context"
	"crypto/sha256"
	"errors"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"sync"
	"testing"
	"time"
)

func TestPostgresGoogleExplicitLinkAndSessionFence(t *testing.T) {
	p := reviewsDB(t)
	r := repository.New(p)
	ctx := context.Background()
	hashed, _ := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)
	var id int64
	if err := p.QueryRow(ctx, `INSERT INTO usuarios(email,password_hash,nombre,tipo_cuenta,email_verified_at) VALUES('link@example.test',$1,'Link','PERSONAL_MARCA',now()) RETURNING id`, string(hashed)).Scan(&id); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		if _, err := r.ResolveGoogleUser(ctx, "link-sub", "link@example.test"); !errors.Is(err, repository.ErrGoogleLinkRequired) {
			t.Fatal(err)
		}
	}
	var subject *string
	var count int
	p.QueryRow(ctx, `SELECT google_id FROM usuarios WHERE id=$1`, id).Scan(&subject)
	p.QueryRow(ctx, `SELECT count(*) FROM sesiones_auth WHERE usuario_id=$1`, id).Scan(&count)
	if subject != nil || count != 0 {
		t.Fatal("login changed identity or sessions")
	}
	tokens := auth.NewTokens("12345678901234567890123456789012", "test")
	s := service.New(r, tokens, config.Config{})
	s.VerifyGoogleToken = func(context.Context, string) (auth.GoogleIdentity, error) {
		return auth.GoogleIdentity{GoogleID: "link-sub", Email: "link@example.test"}, nil
	}
	login, err := s.Login(ctx, model.LoginRequest{Email: "link@example.test", Password: "test-password"})
	if err != nil {
		t.Fatal(err)
	}
	_, _, sid, version, _, _ := tokens.ParseSessionContext(login.Session.AccessToken)
	if _, err = s.LinkGoogle(ctx, id, sid, version, model.GoogleLinkRequest{IDToken: "test", Password: "wrong"}); !errors.Is(err, service.ErrInvalidCredentials) {
		t.Fatal(err)
	}
	linked, err := s.LinkGoogle(ctx, id, sid, version, model.GoogleLinkRequest{IDToken: "test", Password: "test-password"})
	if err != nil {
		t.Fatal(err)
	}
	if linked.User.AuthVersion != version+1 {
		t.Fatal("version not advanced")
	}
	if _, err = r.ActiveSessionAccountType(ctx, id, sid, version); err == nil {
		t.Fatal("old session active")
	}
	if err = r.CreateSession(ctx, uuid.NewString(), id, []byte("stale"), time.Now().Add(time.Hour), time.Now(), version); !errors.Is(err, repository.ErrNotFound) {
		t.Fatal("stale login crossed revocation", err)
	}
	_, _, newSID, newVersion, _, _ := tokens.ParseSessionContext(linked.Session.AccessToken)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, e := s.Reauthenticate(ctx, id, newSID, newVersion, model.ReauthenticateRequest{Password: "test-password"})
			results <- e
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	success := 0
	for e := range results {
		if e == nil {
			success++
		} else if !errors.Is(e, service.ErrInvalidCredentials) {
			t.Fatal(e)
		}
	}
	if success != 1 {
		t.Fatalf("reauth successes=%d", success)
	}
	if _, err = r.ResolveGoogleUser(ctx, "another-sub", "link@example.test"); !errors.Is(err, repository.ErrGoogleIdentityConflict) {
		t.Fatal(err)
	}
	if _, err = r.ResolveGoogleUser(ctx, "link-sub", "renamed@example.test"); err != nil {
		t.Fatal("subject must survive provider email change", err)
	}
	// Logout proof revokes a refresh family even if access has expired.
	hash := sha256.Sum256([]byte(linked.Session.RefreshToken))
	if err = r.RevokeRefreshFamily(ctx, hash[:]); err != nil {
		t.Fatal(err)
	}
	if err = r.RevokeRefreshFamily(ctx, hash[:]); err != nil {
		t.Fatal("logout must be idempotent", err)
	}
}

func TestPostgresLogoutSerializesConcurrentRefreshAndRevokesDescendants(t *testing.T) {
	p := reviewsDB(t)
	r := repository.New(p)
	ctx := context.Background()
	var id int64
	if err := p.QueryRow(ctx, `INSERT INTO usuarios(email,nombre,tipo_cuenta,email_verified_at) VALUES('logout-race@example.test','Logout','PERSONAL_MARCA',now()) RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	for round := 0; round < 16; round++ {
		hash := sha256.Sum256([]byte(uuid.NewString()))
		sid := uuid.NewString()
		if err := r.CreateSession(ctx, sid, id, hash[:], time.Now().Add(time.Hour), time.Now(), 1); err != nil {
			t.Fatal(err)
		}
		newHash := sha256.Sum256([]byte(uuid.NewString()))
		var wg sync.WaitGroup
		start := make(chan struct{})
		results := make(chan error, 2)
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, e := r.RotateSession(ctx, hash[:], uuid.NewString(), newHash[:], time.Now().Add(time.Hour))
			results <- e
		}()
		go func() { defer wg.Done(); <-start; results <- r.RevokeRefreshFamily(ctx, hash[:]) }()
		close(start)
		wg.Wait()
		close(results)
		for e := range results {
			if e != nil && !errors.Is(e, repository.ErrSessionReuse) && !errors.Is(e, repository.ErrNotFound) {
				t.Fatal(e)
			}
		}
		var active int
		if err := p.QueryRow(ctx, `SELECT count(*) FROM sesiones_auth WHERE family_id=$1 AND revoked_at IS NULL`, sid).Scan(&active); err != nil {
			t.Fatal(err)
		}
		if active != 0 {
			t.Fatalf("round%d active descendants%d", round, active)
		}
	}
}
func TestPostgresLinkRejectsOtherAccountIdentityAndPreservesBoth(t *testing.T) {
	p := reviewsDB(t)
	r := repository.New(p)
	ctx := context.Background()
	hash, _ := bcrypt.GenerateFromPassword([]byte("link-password"), bcrypt.MinCost)
	var id, other int64
	if e := p.QueryRow(ctx, `INSERT INTO usuarios(email,password_hash,nombre,tipo_cuenta,email_verified_at) VALUES('victim@example.test',$1,'Victim','PERSONAL_MARCA',now()) RETURNING id`, string(hash)).Scan(&id); e != nil {
		t.Fatal(e)
	}
	if e := p.QueryRow(ctx, `INSERT INTO usuarios(email,google_id,nombre,tipo_cuenta,email_verified_at) VALUES('other@example.test','owned-sub','Other','PERSONAL_MARCA',now()) RETURNING id`).Scan(&other); e != nil {
		t.Fatal(e)
	}
	s := service.New(r, auth.NewTokens("12345678901234567890123456789012", "test"), config.Config{})
	s.VerifyGoogleToken = func(context.Context, string) (auth.GoogleIdentity, error) {
		return auth.GoogleIdentity{GoogleID: "owned-sub", Email: "victim@example.test"}, nil
	}
	login, e := s.Login(ctx, model.LoginRequest{Email: "victim@example.test", Password: "link-password"})
	if e != nil {
		t.Fatal(e)
	}
	_, _, sid, version, _, _ := s.Tokens.ParseSessionContext(login.Session.AccessToken)
	if _, e = s.LinkGoogle(ctx, id, sid, version, model.GoogleLinkRequest{IDToken: "provider-token", Password: "link-password"}); !errors.Is(e, repository.ErrGoogleIdentityConflict) {
		t.Fatal("foreign subject linked", e)
	}
	var subject *string
	var authVersion int
	p.QueryRow(ctx, `SELECT google_id,auth_version FROM usuarios WHERE id=$1`, id).Scan(&subject, &authVersion)
	if subject != nil || authVersion != version {
		t.Fatal("conflict mutated victim")
	}
	if _, e = r.ActiveSessionAccountType(ctx, id, sid, version); e != nil {
		t.Fatal("conflict revoked prior session", e)
	}
	if e = p.QueryRow(ctx, `SELECT google_id FROM usuarios WHERE id=$1`, other).Scan(&subject); e != nil || subject == nil || *subject != "owned-sub" {
		t.Fatal("foreign subject replaced", e)
	}
}

func TestPostgresSessionFencePreservesDependencyTimeoutErrors(t *testing.T) {
	p := reviewsDB(t)
	r := repository.New(p)
	ctx := context.Background()
	var id int64
	if e := p.QueryRow(ctx, `INSERT INTO usuarios(email,nombre,tipo_cuenta) VALUES('timeout@example.test','Timeout','PERSONAL_MARCA') RETURNING id`).Scan(&id); e != nil {
		t.Fatal(e)
	}
	tx, e := p.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `UPDATE usuarios SET nombre=nombre WHERE id=$1`, id); e != nil {
		t.Fatal(e)
	}
	bounded, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	e = r.CreateSession(bounded, uuid.NewString(), id, []byte("deadline"), time.Now().Add(time.Hour), time.Now(), 1)
	if e == nil || errors.Is(e, repository.ErrNotFound) {
		t.Fatal("dependency timeout collapsed into invalid session", e)
	}
}
