package repos

import (
	"context"
	"encoding/json"
	"time"
)

type Photo struct {
	Id         string          `json:"id"`
	RegionId   int             `json:"region_id"`
	InsertedAt *time.Time      `json:"inserted_at"`
	Info       json.RawMessage `json:"info"`
	Sizes      json.RawMessage `json:"sizes"`
	Exif       json.RawMessage `json:"exif"`

	Medium PhotoSize `json:"medium"` // corresponds to no suffix, 500px longest edge
	Large  PhotoSize `json:"large"`  // largest available size besides original
}

type PhotoSize struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Source string `json:"source"`
}

func (r *Repo) GetPhoto(ctx context.Context, id string) (*Photo, error) {
	var photo Photo
	err := r.db.QueryRow(ctx, `
		SELECT id, region_id, inserted_at, info, sizes, exif, medium, large
		FROM flickr
		WHERE id = $1
	`, id).Scan(&photo.Id, &photo.RegionId, &photo.InsertedAt, &photo.Info, &photo.Sizes, &photo.Exif, &photo.Medium, &photo.Large)
	if err != nil {
		return nil, err
	}
	return &photo, nil
}
