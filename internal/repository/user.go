package repository

import (
	"context"
	"errors"

	"clientesFrecuentes/internal/model"

	"github.com/jackc/pgx/v5"
)

type AuthUser struct {
	User         model.User
	PasswordHash *string
	GoogleID     *string
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
