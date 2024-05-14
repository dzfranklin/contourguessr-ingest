package repos

import (
	"context"
	"github.com/jackc/pgx/v5"
)

func (r *Repo) SaveStep(ctx context.Context, region int, cursor Cursor, photos []Photo) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	batch := &pgx.Batch{}
	for _, p := range photos {
		batch.Queue(`
			INSERT INTO flickr (id, region_id, info, sizes, exif,
			                    medium, large)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (id) DO NOTHING
		`, p.Id, region, p.Info, p.Sizes, p.Exif, p.Medium, p.Large)
	}

	results := tx.SendBatch(ctx, batch)
	defer results.Close()
	for i := 0; i < len(photos); i++ {
		_, err := results.Exec()
		if err != nil {
			return err
		}
	}
	_ = results.Close()

	_, err = tx.Exec(ctx, `
		INSERT INTO region_index_cursors (region_id, min_upload_date, page, last_check)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (region_id) DO UPDATE
		SET min_upload_date = EXCLUDED.min_upload_date,
		    page = EXCLUDED.page,
		    last_check = EXCLUDED.last_check
	`, region, cursor.MinUploadDate, cursor.Page, cursor.LastCheck)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}
