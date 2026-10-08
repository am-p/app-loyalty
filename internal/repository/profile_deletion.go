package repository

import "context"

func (r *Repository) PendingProfileDeletions(ctx context.Context, limit int) ([]string, error) {
	rows, err := r.Pool.Query(ctx, `SELECT object_key FROM profile_media_deletions ORDER BY requested_at,object_key LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := []string{}
	for rows.Next() {
		var key string
		if err = rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}
func (r *Repository) CompleteProfileDeletion(ctx context.Context, key string) error {
	_, err := r.Pool.Exec(ctx, `DELETE FROM profile_media_deletions WHERE object_key=$1`, key)
	return err
}
