package repos

import (
	"context"
	"log"
)

func (r *Repo) UpsertFlickrLicense(ctx context.Context, id int, name, url string) {
	maybeURL := &url
	if *maybeURL == "" {
		maybeURL = nil
	}

	_, err := r.db.Exec(ctx, `
		INSERT INTO flickr_licenses (id, name, url)
		VALUES ($1, $2, $3)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			url = EXCLUDED.url
	`, id, name, maybeURL)
	if err != nil {
		log.Fatal(err)
	}
}
