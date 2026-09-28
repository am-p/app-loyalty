package repository

import (
	"context"
	"errors"
	"time"

	"clientesFrecuentes/internal/model"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const reviewSettingsColumns = `s.id,COALESCE(c.enabled,false),COALESCE(c.purchase_threshold,1),COALESCE(c.message,'¿Cómo fue tu experiencia? Compartí tu opinión en Google.'),COALESCE(c.destination_type,'MANUAL_LINK'),c.google_place_id,c.manual_review_url,COALESCE(c.version,1)`

func scanReviewSettings(row rowScanner, out *model.ReviewSettings) error {
	return row.Scan(&out.BranchID, &out.Enabled, &out.PurchaseThreshold, &out.Message, &out.DestinationType, &out.GooglePlaceID, &out.ManualReviewURL, &out.Version)
}

// Locking the branch protects the initial absent configuration and synchronizes leases/disable.
func reviewBranch(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, actor, brand, branch int64, lock bool) (model.Branch, error) {
	var b model.Branch
	var role string
	sql := `SELECT s.id,s.marca_id,s.nombre,s.direccion,s.activo,mm.rol FROM sucursales s JOIN marcas m ON m.id=s.marca_id AND m.activo AND m.deleted_at IS NULL JOIN membresias_marca mm ON mm.marca_id=m.id AND mm.usuario_id=$1 AND mm.activo JOIN usuarios u ON u.id=mm.usuario_id AND u.activo AND u.deleted_at IS NULL WHERE s.id=$3 AND s.marca_id=$2 AND s.deleted_at IS NULL`
	if lock {
		sql += ` FOR UPDATE OF s`
	}
	e := q.QueryRow(ctx, sql, actor, brand, branch).Scan(&b.ID, &b.BrandID, &b.Name, &b.Address, &b.Active, &role)
	if errors.Is(e, pgx.ErrNoRows) {
		return b, ErrNotFound
	}
	if e != nil {
		return b, e
	}
	if !mutableRole(role) {
		return b, ErrForbidden
	}
	return b, nil
}
func (r *Repository) ReviewBranch(ctx context.Context, a, b, id int64) (model.Branch, error) {
	return reviewBranch(ctx, r.Pool, a, b, id, false)
}
func (r *Repository) ReviewSettings(ctx context.Context, a, b, id int64) (model.ReviewSettings, error) {
	if _, e := r.ReviewBranch(ctx, a, b, id); e != nil {
		return model.ReviewSettings{}, e
	}
	var out model.ReviewSettings
	e := scanReviewSettings(r.Pool.QueryRow(ctx, `SELECT `+reviewSettingsColumns+` FROM sucursales s LEFT JOIN configuracion_resenas c ON c.sucursal_id=s.id WHERE s.id=$1`, id), &out)
	return out, e
}
func (r *Repository) SaveReviewSettings(ctx context.Context, a, b, id int64, v int, in model.ReviewSettingsInput) (model.ReviewSettings, error) {
	tx, e := r.Pool.Begin(ctx)
	if e != nil {
		return model.ReviewSettings{}, e
	}
	defer tx.Rollback(ctx)
	branch, e := reviewBranch(ctx, tx, a, b, id, true)
	if e != nil {
		return model.ReviewSettings{}, e
	}
	if in.Enabled && !branch.Active {
		return model.ReviewSettings{}, ErrConflict
	}
	var out model.ReviewSettings
	e = scanReviewSettings(tx.QueryRow(ctx, `SELECT `+reviewSettingsColumns+` FROM sucursales s LEFT JOIN configuracion_resenas c ON c.sucursal_id=s.id WHERE s.id=$1`, id), &out)
	if e != nil {
		return out, e
	}
	if out.Version != v {
		return out, ErrPreconditionFailed
	}
	e = tx.QueryRow(ctx, `INSERT INTO configuracion_resenas(sucursal_id,enabled,purchase_threshold,message,destination_type,google_place_id,manual_review_url,version) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(sucursal_id) DO UPDATE SET enabled=EXCLUDED.enabled,purchase_threshold=EXCLUDED.purchase_threshold,message=EXCLUDED.message,destination_type=EXCLUDED.destination_type,google_place_id=EXCLUDED.google_place_id,manual_review_url=EXCLUDED.manual_review_url,version=EXCLUDED.version RETURNING version`, id, in.Enabled, in.PurchaseThreshold, in.Message, in.DestinationType, in.GooglePlaceID, in.ManualReviewURL, v+1).Scan(&out.Version)
	if e != nil {
		return out, e
	}
	out.ReviewSettingsInput = in
	return out, tx.Commit(ctx)
}
func (r *Repository) ReviewMetrics(ctx context.Context, a, b, id int64) (model.ReviewMetrics, error) {
	if _, e := r.ReviewBranch(ctx, a, b, id); e != nil {
		return model.ReviewMetrics{}, e
	}
	var out model.ReviewMetrics
	e := r.Pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE shown_at IS NOT NULL),count(*) FILTER(WHERE skipped_at IS NOT NULL),count(*) FILTER(WHERE clicked_at IS NOT NULL) FROM invitaciones_resenas WHERE sucursal_id=$1`, id).Scan(&out.Shown, &out.Skipped, &out.Clicked)
	return out, e
}

const reviewCandidateSelect = `SELECT i.id::text,i.tarjeta_id,i.marca_id,i.sucursal_id,s.nombre,i.operation_id::text,c.message,i.occurred_at,` + reviewSettingsColumns + ` FROM invitaciones_resenas i JOIN sucursales s ON s.id=i.sucursal_id AND s.activo AND s.deleted_at IS NULL JOIN marcas m ON m.id=s.marca_id AND m.activo AND m.deleted_at IS NULL JOIN configuracion_resenas c ON c.sucursal_id=s.id AND c.enabled JOIN usuarios u ON u.id=i.usuario_id AND u.activo AND u.deleted_at IS NULL AND u.tipo_cuenta='CLIENTE_FINAL' JOIN tarjetas t ON t.id=i.tarjeta_id AND t.activo WHERE i.usuario_id=$1 AND i.cancelled_at IS NULL AND i.shown_at IS NULL AND (i.lease_until IS NULL OR i.lease_until<=now())`

func scanReviewCandidate(row rowScanner, out *model.ReviewCandidate) error {
	v := &out.Invitation
	c := &out.Settings
	return row.Scan(&v.ID, &v.CardID, &v.BrandID, &v.BranchID, &v.BranchName, &v.OperationID, &v.Message, &v.OccurredAt, &c.BranchID, &c.Enabled, &c.PurchaseThreshold, &c.Message, &c.DestinationType, &c.GooglePlaceID, &c.ManualReviewURL, &c.Version)
}
func (r *Repository) PendingReviews(ctx context.Context, a int64) ([]model.ReviewCandidate, error) {
	var customer bool
	if e := r.Pool.QueryRow(ctx, `SELECT tipo_cuenta='CLIENTE_FINAL' FROM usuarios WHERE id=$1 AND activo AND deleted_at IS NULL`, a).Scan(&customer); errors.Is(e, pgx.ErrNoRows) {
		return nil, ErrNotFound
	} else if e != nil {
		return nil, e
	}
	if !customer {
		return nil, ErrForbidden
	}
	rows, e := r.Pool.Query(ctx, reviewCandidateSelect+` ORDER BY i.occurred_at,i.id LIMIT 50`, a)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []model.ReviewCandidate{}
	for rows.Next() {
		var c model.ReviewCandidate
		if e = scanReviewCandidate(rows, &c); e != nil {
			return nil, e
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (r *Repository) ReviewCandidate(ctx context.Context, a int64, id string) (model.ReviewCandidate, error) {
	var out model.ReviewCandidate
	e := scanReviewCandidate(r.Pool.QueryRow(ctx, reviewCandidateSelect+` AND i.id=$2`, a, id), &out)
	if errors.Is(e, pgx.ErrNoRows) {
		var own bool
		if x := r.Pool.QueryRow(ctx, `SELECT true FROM invitaciones_resenas WHERE id=$1 AND usuario_id=$2`, id, a).Scan(&own); errors.Is(x, pgx.ErrNoRows) {
			return out, ErrNotFound
		} else if x != nil {
			return out, x
		}
		return out, ErrConflict
	}
	return out, e
}
func (r *Repository) ReserveReview(ctx context.Context, a int64, c model.ReviewCandidate) (model.ReviewReservation, error) {
	tx, e := r.Pool.Begin(ctx)
	if e != nil {
		return model.ReviewReservation{}, e
	}
	defer tx.Rollback(ctx)
	if e = lockReviewCustomer(ctx, tx, a); e != nil {
		return model.ReviewReservation{}, e
	}
	// Settings updates take the same branch lock, then update/cancel invitations.
	var v int
	e = tx.QueryRow(ctx, `SELECT c.version FROM sucursales s JOIN marcas m ON m.id=s.marca_id AND m.activo JOIN configuracion_resenas c ON c.sucursal_id=s.id AND c.enabled WHERE s.id=$1 AND s.activo AND s.deleted_at IS NULL FOR SHARE OF s`, c.Invitation.BranchID).Scan(&v)
	if errors.Is(e, pgx.ErrNoRows) {
		return model.ReviewReservation{}, ErrConflict
	}
	if e != nil {
		return model.ReviewReservation{}, e
	}
	if v != c.Settings.Version {
		return model.ReviewReservation{}, ErrConflict
	}
	out := model.ReviewReservation{Invitation: c.Invitation, ReservationToken: uuid.NewString()}
	e = tx.QueryRow(ctx, `UPDATE invitaciones_resenas i SET reservation_token=$3,lease_until=clock_timestamp()+interval '60 seconds' WHERE i.id=$1 AND i.usuario_id=$2 AND i.shown_at IS NULL AND i.cancelled_at IS NULL AND (i.lease_until IS NULL OR i.lease_until<=now()) AND EXISTS(SELECT 1 FROM usuarios u WHERE u.id=$2 AND u.activo AND u.deleted_at IS NULL AND u.tipo_cuenta='CLIENTE_FINAL') RETURNING lease_until`, c.Invitation.ID, a, out.ReservationToken).Scan(&out.ExpiresAt)
	if errors.Is(e, pgx.ErrNoRows) {
		return out, ErrConflict
	}
	if e != nil {
		return out, e
	}
	return out, tx.Commit(ctx)
}
func (r *Repository) ReviewEvent(ctx context.Context, a int64, id string, in model.ReviewEventRequest) (model.ReviewEvents, error) {
	tx, e := r.Pool.Begin(ctx)
	if e != nil {
		return model.ReviewEvents{}, e
	}
	defer tx.Rollback(ctx)
	if e = lockReviewCustomer(ctx, tx, a); e != nil {
		return model.ReviewEvents{}, e
	}
	var branch int64
	e = tx.QueryRow(ctx, `SELECT sucursal_id FROM invitaciones_resenas WHERE id=$1 AND usuario_id=$2`, id, a).Scan(&branch)
	if errors.Is(e, pgx.ErrNoRows) {
		return model.ReviewEvents{}, ErrNotFound
	}
	if e != nil {
		return model.ReviewEvents{}, e
	}
	var active bool
	e = tx.QueryRow(ctx, `SELECT s.activo AND s.deleted_at IS NULL AND m.activo AND COALESCE(c.enabled,false) FROM sucursales s JOIN marcas m ON m.id=s.marca_id LEFT JOIN configuracion_resenas c ON c.sucursal_id=s.id WHERE s.id=$1 FOR SHARE OF s`, branch).Scan(&active)
	if e != nil {
		return model.ReviewEvents{}, e
	}
	var token *string
	var until, shown, skipped, clicked, cancelled *time.Time
	e = tx.QueryRow(ctx, `SELECT reservation_token::text,lease_until,shown_at,skipped_at,clicked_at,cancelled_at FROM invitaciones_resenas WHERE id=$1 AND usuario_id=$2 FOR UPDATE`, id, a).Scan(&token, &until, &shown, &skipped, &clicked, &cancelled)
	if e != nil {
		return model.ReviewEvents{}, e
	}
	if token == nil || *token != in.ReservationToken || cancelled != nil {
		return model.ReviewEvents{}, ErrConflict
	}
	column := ""
	switch in.Event {
	case "PRESENTADA":
		if shown == nil {
			if !active || until == nil || !until.After(r.Now()) {
				return model.ReviewEvents{}, ErrConflict
			}
			column = "shown_at"
		}
	case "OMITIDA":
		if shown == nil {
			return model.ReviewEvents{}, ErrConflict
		}
		if skipped == nil {
			column = "skipped_at"
		}
	case "CLIC":
		if shown == nil {
			return model.ReviewEvents{}, ErrConflict
		}
		if clicked == nil {
			column = "clicked_at"
		}
	default:
		return model.ReviewEvents{}, ErrInvalidRequest
	}
	if column != "" {
		if _, e = tx.Exec(ctx, `UPDATE invitaciones_resenas SET `+column+`=now() WHERE id=$1`, id); e != nil {
			return model.ReviewEvents{}, e
		}
	}
	out := model.ReviewEvents{ID: id}
	e = tx.QueryRow(ctx, `SELECT shown_at IS NOT NULL,skipped_at IS NOT NULL,clicked_at IS NOT NULL FROM invitaciones_resenas WHERE id=$1`, id).Scan(&out.Shown, &out.Skipped, &out.Clicked)
	if e != nil {
		return out, e
	}
	return out, tx.Commit(ctx)
}

// Current account state is authoritative even when deletion races middleware authentication.
func lockReviewCustomer(ctx context.Context, tx pgx.Tx, actor int64) error {
	var kind string
	e := tx.QueryRow(ctx, `SELECT tipo_cuenta FROM usuarios WHERE id=$1 AND activo AND deleted_at IS NULL FOR SHARE`, actor).Scan(&kind)
	if errors.Is(e, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if e != nil {
		return e
	}
	if kind != "CLIENTE_FINAL" {
		return ErrForbidden
	}
	return nil
}
