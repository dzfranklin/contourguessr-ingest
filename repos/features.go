package repos

import (
	"context"
	"github.com/jackc/pgx/v5"
)

type Features struct {
	PhotoId                string  `json:"photo_id"`
	TerrainElevationMeters int     `json:"terrain_elevation_meters"`
	NoGPSAltitude          bool    `json:"no_gps_altitude"`
	GPSAltitudeMeters      int     `json:"gps_altitude_meters"`
	ValidityScore          float64 `json:"validity_score"`
	ValidityModel          string  `json:"validity_model"`
	NearestRoadMeters      int     `json:"nearest_road_meters"`
	IsOK                   bool    `json:"is_ok"` // computed
}

func (r *Repo) GetFeatures(ctx context.Context, photoId string) (Features, error) {
	rows, err := r.db.Query(ctx, `
		SELECT *
		FROM features
		WHERE photo_id = $1
	`, photoId)
	if err != nil {
		return Features{}, err
	}
	defer rows.Close()
	return pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[Features])
}

func (r *Repo) GetFeaturesIn(ctx context.Context, photoIds []string) ([]Features, error) {
	rows, err := r.db.Query(ctx, `
		SELECT *
		FROM features
		WHERE photo_id = ANY($1)
	`, photoIds)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[Features])
}

func (r *Repo) GetPhotosWithoutFeatures(ctx context.Context, limit int) ([]Photo, error) {
	rows, err := r.db.Query(ctx, `
		SELECT flickr.*
		FROM flickr
		LEFT JOIN features ON flickr.id = features.photo_id
		WHERE features.photo_id IS NULL AND flickr.medium->>'source' != ''
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[Photo])
}

func (r *Repo) SaveFeatures(ctx context.Context, features []Features) error {
	_, err := r.db.CopyFrom(ctx,
		pgx.Identifier{"features"},
		[]string{
			"photo_id", "terrain_elevation_meters", "gps_altitude_meters", "validity_score", "validity_model",
			"nearest_road_meters",
		},
		pgx.CopyFromSlice(len(features), func(i int) ([]interface{}, error) {
			f := features[i]
			return []any{
				f.PhotoId, f.TerrainElevationMeters, f.GPSAltitudeMeters, f.ValidityScore, f.ValidityModel,
				f.NearestRoadMeters,
			}, nil
		}),
	)
	return err
}
