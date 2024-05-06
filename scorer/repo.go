package main

import (
	"context"
	"github.com/jackc/pgx/v4"
	"log"
)

type Entry struct {
	Id *int64

	FlickrId   string
	PreviewURL string
	Lng        float64
	Lat        float64
	Exif       *map[string]string

	RoadWithin1000m *bool
	ValidityScore   *float64
	ValidityModel   *string
}

func loadBatch(db *pgx.Conn) ([]Entry, error) {
	ctx := context.Background()
	rows, err := db.Query(ctx, `
		SELECT s.id, p.flickr_id,
			   p.summary ->> 'server', p.summary ->> 'secret',
			   ST_X(p.geo::geometry), ST_Y(p.geo::geometry), p.exif,
			   s.road_within_1000m, s.validity_score, s.validity_model
		FROM photo_scores as s
				 RIGHT JOIN flickr_photos as p ON s.flickr_photo_id = p.flickr_id
		WHERE (s.vsn is null OR s.vsn = $1)
		  	AND (s.is_complete is null OR not s.is_complete)
			AND not exists (SELECT 1
							FROM flickr_photo_fetch_failures as err
							WHERE err.flickr_id = p.flickr_id)
		ORDER BY random()
		LIMIT 100
	`, activeVsn)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Entry, 0)
	for rows.Next() {
		var server string
		var secret string
		var entry Entry
		err := rows.Scan(
			&entry.Id, &entry.FlickrId,
			&server, &secret,
			&entry.Lng, &entry.Lat, &entry.Exif,
			&entry.RoadWithin1000m, &entry.ValidityScore, &entry.ValidityModel,
		)
		if err != nil {
			return nil, err
		}
		entry.PreviewURL = "https://live.staticflickr.com/" + server + "/" + entry.FlickrId + "_" + secret + "_m.jpg"
		out = append(out, entry)
	}
	return out, nil
}

func (entry *Entry) Save(db *pgx.Conn) error {
	ctx := context.Background()
	if entry.Id == nil {
		row := db.QueryRow(ctx, `
			INSERT INTO photo_scores (vsn, updated_at, flickr_photo_id,
			                          road_within_1000m, validity_score, validity_model)
			VALUES ($1, CURRENT_TIMESTAMP, $2, $3, $4, $5)
			RETURNING id
		`, activeVsn, entry.FlickrId, entry.RoadWithin1000m, entry.ValidityScore, entry.ValidityModel)
		err := row.Scan(&entry.Id)
		if err != nil {
			return err
		}
		return nil
	} else {
		_, err := db.Exec(ctx, `
			UPDATE photo_scores
			SET updated_at = CURRENT_TIMESTAMP,
			    road_within_1000m = $1,
			    validity_score = $2,
			    validity_model = $3
			WHERE id = $4
		`, entry.RoadWithin1000m, entry.ValidityScore, entry.ValidityModel, entry.Id)
		if err != nil {
			return err
		}
		return nil
	}
}

func saveFlickrPhotoFetchFailure(db *pgx.Conn, flickrId string, err error) {
	ctx := context.Background()
	_, err = db.Exec(ctx, `
		INSERT INTO flickr_photo_fetch_failures (flickr_id, err)
		VALUES ($1, $2)
	`, flickrId, err.Error())
	if err != nil {
		log.Println("Error saving fetch failure:", err)
	}
}
