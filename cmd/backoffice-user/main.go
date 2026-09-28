// backoffice-user provisions internal identities independently from customer accounts.
// Usage: DATABASE_URL=... BACKOFFICE_EMAIL=... BACKOFFICE_PASSWORD=... BACKOFFICE_ROLE=ADMIN_SISTEMA go run ./cmd/backoffice-user
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	email := strings.ToLower(strings.TrimSpace(os.Getenv("BACKOFFICE_EMAIL")))
	password := os.Getenv("BACKOFFICE_PASSWORD")
	role := os.Getenv("BACKOFFICE_ROLE")
	if email == "" || len(password) < 14 || (role != "ADMIN_SISTEMA" && role != "FINANZAS") || os.Getenv("DATABASE_URL") == "" {
		log.Fatal("DATABASE_URL, BACKOFFICE_EMAIL, BACKOFFICE_PASSWORD (14+ chars), and BACKOFFICE_ROLE are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatal(err)
	}
	// The legacy TOTP column remains for schema compatibility and is unused.
	_, err = pool.Exec(ctx, `INSERT INTO backoffice_users(email,password_hash,totp_secret,role) VALUES($1,$2,'',$3)`, email, string(hash), role)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Internal account created. Sign in with email and password.")
}
