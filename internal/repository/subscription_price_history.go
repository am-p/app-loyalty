package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type SubscriptionPriceHistoryEntry struct {
	ID                     string    `json:"id"`
	ProgramType            string    `json:"program_type"`
	UnitPriceMinor         int64     `json:"unit_price_minor"`
	PreviousUnitPriceMinor *int64    `json:"previous_unit_price_minor"`
	AdminEmail             string    `json:"admin_email"`
	IncludeExisting        bool      `json:"include_existing"`
	CreatedAt              time.Time `json:"created_at"`
	Total                  int64     `json:"total"`
	Applied                int64     `json:"applied"`
	Pending                int64     `json:"pending"`
	Failed                 int64     `json:"failed"`
	Skipped                int64     `json:"skipped"`
}

type SubscriptionPriceHistoryPage struct {
	Items      []SubscriptionPriceHistoryEntry `json:"items"`
	NextCursor *string                         `json:"next_cursor"`
}

// The timestamp/UUID pair yields stable pages, including simultaneous changes.
// Newer writes do not shift or duplicate entries when loading older pages.
func (r *Repository) ListSubscriptionPriceChanges(ctx context.Context, limit int, before *uuid.UUID) (SubscriptionPriceHistoryPage, error) {
	page := SubscriptionPriceHistoryPage{Items: make([]SubscriptionPriceHistoryEntry, 0)}
	if limit < 1 || limit > 50 {
		return page, ErrInvalidRequest
	}
	if before != nil {
		var exists bool
		if err := r.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM subscription_price_changes WHERE id=$1)`, *before).Scan(&exists); err != nil {
			return page, err
		}
		if !exists {
			return page, ErrInvalidRequest
		}
	}
	rows, err := r.Pool.Query(ctx, `SELECT j.id,j.program_type,j.unit_price_minor,j.previous_unit_price_minor,u.email,j.include_existing,j.created_at,
 c.total,c.applied,c.pending,c.failed,c.skipped
 FROM subscription_price_changes j JOIN backoffice_users u ON u.id=j.admin_id
 CROSS JOIN LATERAL (SELECT count(*) AS total,
 count(*) FILTER(WHERE status='APPLIED') AS applied, count(*) FILTER(WHERE status='PENDING') AS pending,
 count(*) FILTER(WHERE status='FAILED') AS failed, count(*) FILTER(WHERE status='SKIPPED') AS skipped
 FROM subscription_price_change_items WHERE change_id=j.id) c
 WHERE $2::uuid IS NULL OR (j.created_at,j.id)<(SELECT created_at,id FROM subscription_price_changes WHERE id=$2)
 ORDER BY j.created_at DESC,j.id DESC LIMIT $1`, limit+1, before)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var item SubscriptionPriceHistoryEntry
		if err = rows.Scan(&item.ID, &item.ProgramType, &item.UnitPriceMinor, &item.PreviousUnitPriceMinor, &item.AdminEmail, &item.IncludeExisting, &item.CreatedAt, &item.Total, &item.Applied, &item.Pending, &item.Failed, &item.Skipped); err != nil {
			return page, err
		}
		page.Items = append(page.Items, item)
	}
	if err = rows.Err(); err != nil {
		return page, err
	}
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		last := page.Items[limit-1].ID
		page.NextCursor = &last
	}
	return page, nil
}
