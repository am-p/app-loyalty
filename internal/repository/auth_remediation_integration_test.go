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
		} else if !errors.Is(e, repository.ErrNotFound) {
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
