package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Earnings amounts are recorded ARS minor units, never estimates from today's campaign.
type InfluencerEarningsTotals struct {
	PaidCharges      int64 `json:"paid_charges"`
	PaidAmountMinor  int64 `json:"paid_amount_minor"`
	GeneratedMinor   int64 `json:"generated_minor"`
	PendingMinor     int64 `json:"pending_minor"`
	SettledMinor     int64 `json:"settled_minor"`
	VoidMinor        int64 `json:"void_minor"`
	RecoveryDueMinor int64 `json:"recovery_due_minor"`
}

func (t *InfluencerEarningsTotals) add(x InfluencerEarningsTotals) {
	t.PaidCharges += x.PaidCharges
	t.PaidAmountMinor += x.PaidAmountMinor
	t.GeneratedMinor += x.GeneratedMinor
	t.PendingMinor += x.PendingMinor
	t.SettledMinor += x.SettledMinor
	t.VoidMinor += x.VoidMinor
	t.RecoveryDueMinor += x.RecoveryDueMinor
}

type InfluencerEarningsClient struct {
	BrandID       int64  `json:"brand_id"`
	BrandName     string `json:"brand_name"`
	Code          string `json:"code"`
	RewardBPS     int    `json:"reward_bps"`
	RewardCharges int    `json:"reward_charges"`
	InfluencerEarningsTotals
}

type InfluencerEarningsMonth struct {
	Month   string                     `json:"month"`
	Totals  InfluencerEarningsTotals   `json:"totals"`
	Clients []InfluencerEarningsClient `json:"clients"`
}

type InfluencerEarnings struct {
	InfluencerID   int64                      `json:"influencer_id"`
	InfluencerName string                     `json:"influencer_name"`
	Currency       string                     `json:"currency"`
	TimeZone       string                     `json:"time_zone"`
	DateBasis      string                     `json:"date_basis"`
	Totals         InfluencerEarningsTotals   `json:"totals"`
	Clients        []InfluencerEarningsClient `json:"clients"`
	Months         []InfluencerEarningsMonth  `json:"months"`
	NextCursor     *string                    `json:"next_cursor"`
}

const influencerEarningsCTE = `WITH clients AS (
 SELECT a.brand_id,b.nombre AS brand_name,rc.code,a.reward_bps,a.reward_charges
 FROM referral_attributions a JOIN marcas b ON b.id=a.brand_id
 JOIN referral_codes rc ON rc.id=a.code_id
 WHERE a.source_kind='INFLUENCER' AND a.influencer_id=$1
), events AS (
 SELECT cl.*,to_char(c.created_at AT TIME ZONE 'America/Argentina/Buenos_Aires','YYYY-MM') AS month,
 CASE WHEN c.status='APPROVED' AND c.amount_minor>0 THEN 1 ELSE 0 END::bigint AS paid_charges,
 CASE WHEN c.status='APPROVED' THEN c.amount_minor ELSE 0 END AS paid_amount_minor,
 COALESCE(rw.amount_minor,0) AS generated_minor,
 CASE WHEN rw.status='PENDING' THEN rw.amount_minor ELSE 0 END AS pending_minor,
 CASE WHEN rw.status='SETTLED' THEN rw.amount_minor ELSE 0 END AS settled_minor,
 CASE WHEN rw.status='VOID' THEN rw.amount_minor ELSE 0 END AS void_minor,
 CASE WHEN rw.status='RECOVERY_DUE' THEN rw.amount_minor ELSE 0 END AS recovery_due_minor
 FROM clients cl JOIN referral_charges c ON c.brand_id=cl.brand_id
 LEFT JOIN referral_rewards rw ON rw.provider_invoice_id=c.provider_invoice_id
 AND rw.source_kind='INFLUENCER' AND rw.influencer_id=$1
) `

const earningsSums = `COALESCE(sum(e.paid_charges),0)::bigint,COALESCE(sum(e.paid_amount_minor),0)::bigint,
 COALESCE(sum(e.generated_minor),0)::bigint,COALESCE(sum(e.pending_minor),0)::bigint,
 COALESCE(sum(e.settled_minor),0)::bigint,COALESCE(sum(e.void_minor),0)::bigint,
 COALESCE(sum(e.recovery_due_minor),0)::bigint`

func scanEarningsClient(row pgx.Row, x *InfluencerEarningsClient, month *string) error {
	dest := []any{&x.BrandID, &x.BrandName, &x.Code, &x.RewardBPS, &x.RewardCharges,
		&x.PaidCharges, &x.PaidAmountMinor, &x.GeneratedMinor, &x.PendingMinor,
		&x.SettledMinor, &x.VoidMinor, &x.RecoveryDueMinor}
	if month != nil {
		dest = append([]any{month}, dest...)
	}
	return row.Scan(dest...)
}

// Read all-time totals independently of the monthly page, in one consistent snapshot.
// created_at is the verification's local ledger registration date, not a provider billing period.
func (r *Repository) InfluencerEarnings(ctx context.Context, influencerID int64, before string, limit int) (InfluencerEarnings, error) {
	out := InfluencerEarnings{InfluencerID: influencerID, Currency: "ARS", TimeZone: "America/Argentina/Buenos_Aires",
		DateBasis: "CHARGE_RECORDED_AT", Clients: []InfluencerEarningsClient{}, Months: []InfluencerEarningsMonth{}}
	if influencerID < 1 || limit < 1 || limit > 24 {
		return out, ErrInvalidRequest
	}
	if before != "" {
		date, err := time.Parse("2006-01", before)
		if err != nil || date.Year() < 1 {
			return out, ErrInvalidRequest
		}
	}
	tx, err := r.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = tx.QueryRow(ctx, `SELECT name FROM referral_influencers WHERE id=$1`, influencerID).Scan(&out.InfluencerName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			err = ErrNotFound
		}
		return out, err
	}
	rows, err := tx.Query(ctx, influencerEarningsCTE+`SELECT cl.brand_id,cl.brand_name,cl.code,cl.reward_bps,cl.reward_charges,`+earningsSums+`
 FROM clients cl LEFT JOIN events e ON e.brand_id=cl.brand_id
 GROUP BY cl.brand_id,cl.brand_name,cl.code,cl.reward_bps,cl.reward_charges ORDER BY cl.brand_name,cl.brand_id`, influencerID)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var x InfluencerEarningsClient
		if err = scanEarningsClient(rows, &x, nil); err != nil {
			rows.Close()
			return out, err
		}
		out.Clients = append(out.Clients, x)
		out.Totals.add(x.InfluencerEarningsTotals)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	rows, err = tx.Query(ctx, influencerEarningsCTE+`, page AS (
 SELECT DISTINCT month FROM events WHERE ($2='' OR month<$2) ORDER BY month DESC LIMIT $3
 ) SELECT e.month,e.brand_id,e.brand_name,e.code,e.reward_bps,e.reward_charges,`+earningsSums+`
 FROM events e JOIN page p ON p.month=e.month
 GROUP BY e.month,e.brand_id,e.brand_name,e.code,e.reward_bps,e.reward_charges
 ORDER BY e.month DESC,e.brand_name,e.brand_id`, influencerID, before, limit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var month string
		var x InfluencerEarningsClient
		if err = scanEarningsClient(rows, &x, &month); err != nil {
			rows.Close()
			return out, err
		}
		if len(out.Months) == 0 || out.Months[len(out.Months)-1].Month != month {
			out.Months = append(out.Months, InfluencerEarningsMonth{Month: month, Clients: []InfluencerEarningsClient{}})
		}
		item := &out.Months[len(out.Months)-1]
		item.Clients = append(item.Clients, x)
		item.Totals.add(x.InfluencerEarningsTotals)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Months) > limit {
		out.Months = out.Months[:limit]
		cursor := out.Months[limit-1].Month
		out.NextCursor = &cursor
	}
	return out, tx.Commit(ctx)
}
