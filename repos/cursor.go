package repos

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"time"
)

type Cursor struct {
	MinUploadDate time.Time
	Page          int
	LastCheck     *time.Time
}

func (r *Repo) GetCursor(ctx context.Context, region int) (Cursor, error) {
	var cursor Cursor
	err := r.db.QueryRow(ctx, `
		SELECT min_upload_date, page, last_check
		FROM region_index_cursors
		WHERE region_id = $1
	`, region).Scan(&cursor.MinUploadDate, &cursor.Page, &cursor.LastCheck)
	if errors.Is(err, pgx.ErrNoRows) {
		return Cursor{}, nil
	}
	return cursor, err
}

func (r *Repo) SetCursor(ctx context.Context, region int, cursor Cursor) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO region_index_cursors (region_id, min_upload_date, page, last_check)
		VALUES ($1, $2, $3)
		ON CONFLICT (region_id) DO UPDATE
		SET min_upload_date = EXCLUDED.min_upload_date,
		    page = EXCLUDED.page,
		    last_check = EXCLUDED.last_check
	`, region, cursor.MinUploadDate, cursor.Page, cursor.LastCheck)
	return err
}
