package repos

import (
	"context"
	"math/rand"
	"time"
)

type InsertChallengeData struct {
	FlickrId         string
	RegionId         int
	Lng              float64
	Lat              float64
	RegularSrc       string
	RegularWidth     int
	RegularHeight    int
	LargeSrc         string
	LargeWidth       int
	LargeHeight      int
	PhotographerIcon string
	PhotographerText string
	PhotographerLink string
	Title            string
	DescriptionHTML  string
	DateTaken        *time.Time
	Link             string
	RX               float64
	RY               float64
}

func (r *Repo) InsertChallenge(ctx context.Context, v InsertChallengeData) error {
	if v.RX == 0 {
		v.RX = rand.Float64()
	}
	if v.RY == 0 {
		v.RY = rand.Float64()
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var challengeID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO challenges
			(region_id,
			 geo,
			 regular_src, regular_width, regular_height,
			 large_src, large_width, large_height,
			 photographer_icon, photographer_text, photographer_link,
			 title, description_html, date_taken, link,
			 rx, ry)
		VALUES (
			 $1,
			 ST_SetSRID(ST_MakePoint($2, $3), 4326),
			 $4, $5, $6,
			 $7, $8, $9,
			 $10, $11, $12,
			 $13, $14, $15, $16,
			 $17, $18
 		)
		ON CONFLICT (id) DO UPDATE SET
			region_id = EXCLUDED.region_id,
			geo = EXCLUDED.geo,
			regular_src = EXCLUDED.regular_src,
			regular_width = EXCLUDED.regular_width,
			regular_height = EXCLUDED.regular_height,
			large_src = EXCLUDED.large_src,
			large_width = EXCLUDED.large_width,
			large_height = EXCLUDED.large_height,
			photographer_icon = EXCLUDED.photographer_icon,
			photographer_text = EXCLUDED.photographer_text,
			photographer_link = EXCLUDED.photographer_link,
			title = EXCLUDED.title,
			description_html = EXCLUDED.description_html,
			date_taken = EXCLUDED.date_taken,
			link = EXCLUDED.link,
			rx = EXCLUDED.rx,
			ry = EXCLUDED.ry
		RETURNING id
	`,
		v.RegionId,
		v.Lng, v.Lat,
		v.RegularSrc, v.RegularWidth, v.RegularHeight,
		v.LargeSrc, v.LargeWidth, v.LargeHeight,
		v.PhotographerIcon, v.PhotographerText, v.PhotographerLink,
		v.Title, v.DescriptionHTML, v.DateTaken, v.Link,
		v.RX, v.RY,
	).Scan(&challengeID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(context.Background(), `
		INSERT INTO flickr_challenge_sources (flickr_id, challenge_id)
		VALUES ($1, $2)
	`, v.FlickrId, challengeID)
	if err != nil {
		return err
	}

	return tx.Commit(context.Background())
}
