package repos

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"strconv"
	"time"
)

type Photo struct {
	Id         string          `json:"id"`
	RegionId   int             `json:"region_id" db:"region_id"`
	InsertedAt *time.Time      `json:"inserted_at" db:"inserted_at"`
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
	rows, err := r.db.Query(ctx, `
		SELECT *
		FROM flickr
		WHERE id = $1
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByName[Photo])
}

func (p Photo) ParseLngLat() (float64, float64, error) {
	var info struct {
		Location struct {
			Latitude  string
			Longitude string
		}
	}
	if err := json.Unmarshal(p.Info, &info); err != nil {
		return 0, 0, err
	}

	lng, err := strconv.ParseFloat(info.Location.Longitude, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parse longitude: %w", err)
	}

	lat, err := strconv.ParseFloat(info.Location.Latitude, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parse latitude: %w", err)
	}

	return lng, lat, nil
}

func (p Photo) ParseExif() (Exif, error) {
	var out Exif
	err := json.Unmarshal(p.Exif, &out)
	return out, err
}
