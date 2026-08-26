package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"clientesFrecuentes/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound              = errors.New("not found")
	ErrEmailExists           = errors.New("email exists")
	ErrIdempotencyConflict   = errors.New("idempotency conflict")
	ErrIdempotencyInProgress = errors.New("idempotency in progress")
	ErrPreviewExpired        = errors.New("preview expired")
	ErrPreviewConsumed       = errors.New("preview consumed")
	ErrPreviewChanged        = errors.New("preview changed")
	ErrInsufficientBalance   = errors.New("insufficient balance")
)

type Repository struct {
	Pool *pgxpool.Pool
	Now  func() time.Time
}

func New(pool *pgxpool.Pool) *Repository { return &Repository{Pool: pool, Now: time.Now} }

type AuthUser struct {
	User         model.User
	PasswordHash *string
	GoogleID     *string
}
type IdempotentResult struct {
	Status   int
	Body     json.RawMessage
	Replayed bool
}

func (r *Repository) CreateCustomer(ctx context.Context, email, passwordHash, name string, provisionalQRHash []byte, finalQRHash func(int64) []byte) (model.User, error) {
	tx, err := r.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return model.User{}, err
	}
	defer tx.Rollback(ctx)
	var u model.User
	err = tx.QueryRow(ctx, `INSERT INTO usuarios(email,password_hash,nombre,tipo_cuenta,qr_hash)
		VALUES($1,$2,$3,'CLIENTE_FINAL',$4) RETURNING id,email::text,nombre,tipo_cuenta,activo,created_at`, email, passwordHash, name, provisionalQRHash).
		Scan(&u.ID, &u.Email, &u.Name, &u.AccountType, &u.Active, &u.CreatedAt)
	if err != nil {
		return model.User{}, normalize(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE usuarios SET qr_hash=$1 WHERE id=$2`, finalQRHash(u.ID), u.ID); err != nil {
		return model.User{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return model.User{}, normalize(err)
	}
	return u, nil
}

func (r *Repository) GetUserByEmail(ctx context.Context, email string) (AuthUser, error) {
	var u AuthUser
	err := r.Pool.QueryRow(ctx, `SELECT id,email::text,password_hash,google_id,nombre,tipo_cuenta,activo,created_at FROM usuarios WHERE email=$1 AND deleted_at IS NULL`, email).
		Scan(&u.User.ID, &u.User.Email, &u.PasswordHash, &u.GoogleID, &u.User.Name, &u.User.AccountType, &u.User.Active, &u.User.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return AuthUser{}, ErrNotFound
	}
	return u, err
}

func (r *Repository) LoginGoogle(ctx context.Context, googleID, email, name string, provisionalQRHash []byte, finalQRHash func(int64) []byte) (model.User, error) {
	tx, err := r.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return model.User{}, err
	}
	defer tx.Rollback(ctx)
	var u model.User
	err = tx.QueryRow(ctx, `SELECT id,email::text,nombre,tipo_cuenta,activo,created_at FROM usuarios WHERE google_id=$1 AND activo AND deleted_at IS NULL FOR UPDATE`, googleID).Scan(&u.ID, &u.Email, &u.Name, &u.AccountType, &u.Active, &u.CreatedAt)
	if err == nil {
		if err = tx.Commit(ctx); err != nil {
			return model.User{}, err
		}
		return u, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return model.User{}, err
	}
	err = tx.QueryRow(ctx, `SELECT id,email::text,nombre,tipo_cuenta,activo,created_at FROM usuarios WHERE email=$1 AND activo AND deleted_at IS NULL FOR UPDATE`, email).Scan(&u.ID, &u.Email, &u.Name, &u.AccountType, &u.Active, &u.CreatedAt)
	if err == nil {
		if _, err = tx.Exec(ctx, `UPDATE usuarios SET google_id=$1 WHERE id=$2`, googleID, u.ID); err != nil {
			return model.User{}, normalize(err)
		}
		if err = tx.Commit(ctx); err != nil {
			return model.User{}, err
		}
		return u, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return model.User{}, err
	}
	err = tx.QueryRow(ctx, `INSERT INTO usuarios(email,google_id,nombre,tipo_cuenta,qr_hash) VALUES($1,$2,$3,'CLIENTE_FINAL',$4) RETURNING id,email::text,nombre,tipo_cuenta,activo,created_at`, email, googleID, name, provisionalQRHash).Scan(&u.ID, &u.Email, &u.Name, &u.AccountType, &u.Active, &u.CreatedAt)
	if err != nil {
		return model.User{}, normalize(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE usuarios SET qr_hash=$1 WHERE id=$2`, finalQRHash(u.ID), u.ID); err != nil {
		return model.User{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return model.User{}, err
	}
	return u, nil
}

func (r *Repository) GetUserByID(ctx context.Context, id int64) (model.User, error) {
	var u model.User
	err := r.Pool.QueryRow(ctx, `SELECT id,email::text,nombre,tipo_cuenta,activo,created_at FROM usuarios WHERE id=$1 AND deleted_at IS NULL`, id).
		Scan(&u.ID, &u.Email, &u.Name, &u.AccountType, &u.Active, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.User{}, ErrNotFound
	}
	return u, err
}

// ActiveAccountType is the narrow, indexed authorization lookup used for every
// authenticated request. The primary-key lookup also enforces suspension and
// logical deletion before handlers can observe an actor.
func (r *Repository) ActiveAccountType(ctx context.Context, userID int64) (string, error) {
	var accountType string
	err := r.Pool.QueryRow(ctx, `SELECT tipo_cuenta FROM usuarios WHERE id=$1 AND activo AND deleted_at IS NULL`, userID).Scan(&accountType)
	return accountType, err
}

func (r *Repository) GetCurrentUser(ctx context.Context, id int64) (model.CurrentUser, error) {
	u, err := r.GetUserByID(ctx, id)
	if err != nil {
		return model.CurrentUser{}, err
	}
	rows, err := r.Pool.Query(ctx, `SELECT m.id,m.nombre,mm.rol,COALESCE(array_agg(ms.sucursal_id ORDER BY ms.sucursal_id) FILTER(WHERE ms.activo),'{}')
		FROM membresias_marca mm JOIN marcas m ON m.id=mm.marca_id
		LEFT JOIN membresias_sucursales ms ON ms.membresia_id=mm.id
		WHERE mm.usuario_id=$1 AND mm.activo AND m.activo GROUP BY m.id,m.nombre,mm.rol ORDER BY m.id`, id)
	if err != nil {
		return model.CurrentUser{}, err
	}
	defer rows.Close()
	memberships := make([]model.Membership, 0)
	for rows.Next() {
		var m model.Membership
		if err = rows.Scan(&m.BrandID, &m.BrandName, &m.Role, &m.BranchIDs); err != nil {
			return model.CurrentUser{}, err
		}
		memberships = append(memberships, m)
	}
	if err = rows.Err(); err != nil {
		return model.CurrentUser{}, err
	}
	return model.CurrentUser{User: u, Memberships: memberships, OnboardingComplete: u.AccountType == "CLIENTE_FINAL" || len(memberships) > 0}, nil
}

func (r *Repository) CreateDemoMerchant(ctx context.Context, key string, fingerprint []byte, email, passwordHash, ownerName, brandName, branchName string, branchAddress *string, build func(model.User, model.MerchantContext) ([]byte, error)) (IdempotentResult, error) {
	tx, err := r.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return IdempotentResult{}, err
	}
	defer tx.Rollback(ctx)
	actorScope := "demo-email:" + email
	claimed, err := claimIdempotency(ctx, tx, key, actorScope, "REGISTER_DEMO_MERCHANT", fingerprint)
	if err != nil {
		return IdempotentResult{}, err
	}
	if claimed != nil {
		return *claimed, nil
	}
	var u model.User
	err = tx.QueryRow(ctx, `INSERT INTO usuarios(email,password_hash,nombre,tipo_cuenta) VALUES($1,$2,$3,'PERSONAL_MARCA') RETURNING id,email::text,nombre,tipo_cuenta,activo,created_at`, email, passwordHash, ownerName).
		Scan(&u.ID, &u.Email, &u.Name, &u.AccountType, &u.Active, &u.CreatedAt)
	if err != nil {
		return IdempotentResult{}, normalize(err)
	}
	var brandID, membershipID, branchID, programID, benefitID int64
	var started time.Time
	if err = tx.QueryRow(ctx, `INSERT INTO marcas(nombre) VALUES($1) RETURNING id`, brandName).Scan(&brandID); err != nil {
		return IdempotentResult{}, err
	}
	if err = tx.QueryRow(ctx, `INSERT INTO membresias_marca(usuario_id,marca_id,rol) VALUES($1,$2,'PROPIETARIO') RETURNING id`, u.ID, brandID).Scan(&membershipID); err != nil {
		return IdempotentResult{}, err
	}
	if err = tx.QueryRow(ctx, `INSERT INTO sucursales(marca_id,nombre,direccion) VALUES($1,$2,$3) RETURNING id`, brandID, branchName, branchAddress).Scan(&branchID); err != nil {
		return IdempotentResult{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO membresias_sucursales(membresia_id,sucursal_id) VALUES($1,$2)`, membershipID, branchID); err != nil {
		return IdempotentResult{}, err
	}
	if err = tx.QueryRow(ctx, `INSERT INTO programas_fidelidad(marca_id,tipo,sellos_por_acumulacion) VALUES($1,'SELLOS',1) RETURNING id`, brandID).Scan(&programID); err != nil {
		return IdempotentResult{}, err
	}
	if err = tx.QueryRow(ctx, `INSERT INTO beneficios(programa_id,nombre,requisito_sellos) VALUES($1,'Beneficio de prueba',5) RETURNING id`, programID).Scan(&benefitID); err != nil {
		return IdempotentResult{}, err
	}
	if err = tx.QueryRow(ctx, `INSERT INTO accesos_demo(marca_id,tipo,precio_minor,moneda,cobro_automatico) VALUES($1,'SELLOS_FREE_TRIAL',0,'ARS',false) RETURNING started_at`, brandID).Scan(&started); err != nil {
		return IdempotentResult{}, err
	}
	merchant := model.MerchantContext{BrandID: brandID, BrandName: brandName, Role: "PROPIETARIO", Branch: model.Branch{ID: branchID, BrandID: brandID, Name: branchName, Address: branchAddress, Active: true}, Program: model.Program{ID: programID, BrandID: brandID, Type: "SELLOS", StampsPerAccumulation: 1, Active: true}, Benefit: model.Benefit{ID: benefitID, ProgramID: programID, Name: "Beneficio de prueba", RequiredStamps: 5, Active: true}, DemoAccess: model.DemoAccess{Kind: "SELLOS_FREE_TRIAL", PriceMinor: 0, Currency: "ARS", AutomaticCharge: false, Active: true, StartedAt: started}}
	body, err := build(u, merchant)
	if err != nil {
		return IdempotentResult{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE solicitudes_idempotentes SET estado='COMPLETED',response_status=201,response_body=$2,completed_at=now() WHERE idempotency_key=$1`, key, []byte(body)); err != nil {
		return IdempotentResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return IdempotentResult{}, normalize(err)
	}
	return IdempotentResult{Status: 201, Body: body}, nil
}

func claimIdempotency(ctx context.Context, tx pgx.Tx, key, actorScope, operation string, fingerprint []byte) (*IdempotentResult, error) {
	// Bound only the reservation wait. A concurrent speculative insert for the
	// same key must become IDEMPOTENCY_IN_PROGRESS instead of waiting for the
	// first request until the HTTP deadline.
	if _, err := tx.Exec(ctx, `SET LOCAL lock_timeout='150ms'`); err != nil {
		return nil, err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO solicitudes_idempotentes(idempotency_key,actor_scope,operacion,fingerprint,estado) VALUES($1,$2,$3,$4,'PENDING') ON CONFLICT DO NOTHING`, key, actorScope, operation, fingerprint)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "55P03" {
			return nil, ErrIdempotencyInProgress
		}
		return nil, err
	}
	if _, err = tx.Exec(ctx, `SET LOCAL lock_timeout='0'`); err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 1 {
		return nil, nil
	}
	var storedActor, storedOperation, state string
	var storedFingerprint []byte
	var status *int
	var body []byte
	err = tx.QueryRow(ctx, `SELECT actor_scope,operacion,fingerprint,estado,response_status,response_body FROM solicitudes_idempotentes WHERE idempotency_key=$1 FOR UPDATE`, key).Scan(&storedActor, &storedOperation, &storedFingerprint, &state, &status, &body)
	if err != nil {
		return nil, err
	}
	if storedActor != actorScope || storedOperation != operation || !bytes.Equal(storedFingerprint, fingerprint) {
		return nil, ErrIdempotencyConflict
	}
	if state != "COMPLETED" || status == nil {
		return nil, ErrIdempotencyInProgress
	}
	return &IdempotentResult{Status: *status, Body: json.RawMessage(body), Replayed: true}, nil
}

func normalize(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		if pgErr.ConstraintName == "usuarios_email_key" {
			return ErrEmailExists
		}
	}
	return err
}

func IsRetryable(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == "40001" || pgErr.Code == "40P01")
}

func (r *Repository) CheckSchema(ctx context.Context, expected string) error {
	var got string
	err := r.Pool.QueryRow(ctx, `SELECT version FROM schema_migrations ORDER BY applied_at DESC LIMIT 1`).Scan(&got)
	if err != nil {
		return err
	}
	if got != expected {
		return fmt.Errorf("schema version %s, expected %s", got, expected)
	}
	return nil
}
