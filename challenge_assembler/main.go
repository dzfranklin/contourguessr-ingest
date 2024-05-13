package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
	"log"
	"math/rand/v2"
	"os"
	"strconv"
	"time"
)

var db *pgx.Conn

func main() {
	// Environment variables

	err := godotenv.Load(".env", ".env.local")
	if err != nil {
		log.Println(err)
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL not set")
	}

	db, err = pgx.Connect(context.Background(), databaseURL)
	if err != nil {
		log.Fatal(err)
	}

	// End setup
	for {
		startTime := time.Now()
		batch := loadBatch()

		if len(batch) == 0 {
			time.Sleep(5 * time.Second)
			continue
		}

		for _, entry := range batch {
			err := processEntry(entry)
			if err != nil {
				log.Printf("Error processing entry: %v", err)
			}
		}
		log.Printf("Processed batch of %d entries in %s", len(batch), time.Since(startTime))
	}
}

type batchEntry struct {
	FlickrId string
	RegionID int
	Lng      float64
	Lat      float64
	Sizes    struct {
		Size []json.RawMessage
	}
	Info infoData
}

type sizeData struct {
	Label  string
	Width  int
	Height int
	Source string
}

type infoData struct {
	Owner struct {
		Iconfarm   int
		Iconserver string
		NSID       string
		Username   string
		PathAlias  string `json:"path_alias"`
	}
	Title       stringContent
	Description stringContent
	Dates       struct {
		Taken string
	}
}

type stringContent struct {
	Content string `json:"_content"`
}

func loadBatch() []batchEntry {
	rows, err := db.Query(context.Background(), `
		SELECT p.flickr_id, p.region_id, ST_X(p.geo::geometry), ST_Y(p.geo::geometry), p.sizes, p.info
		FROM flickr_photos as p
		JOIN photo_scores as s ON p.flickr_id = s.flickr_photo_id
		LEFT JOIN flickr_challenge_sources as src ON p.flickr_id = src.flickr_id
		WHERE
		    s.is_accepted AND
	  		src.flickr_id IS NULL -- no existing challenge based on
			AND p.sizes IS NOT NULL AND p.info IS NOT NULL -- fully indexed
		ORDER BY random()
		LIMIT 1000
	`)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	var entries []batchEntry
	for rows.Next() {
		var entry batchEntry
		err := rows.Scan(&entry.FlickrId, &entry.RegionID, &entry.Lng, &entry.Lat, &entry.Sizes, &entry.Info)
		if err != nil {
			log.Fatal(err)
		}
		entries = append(entries, entry)
	}
	return entries
}

func processEntry(entry batchEntry) error {
	var previewSrc, regularSrc, largeSrc string
	var previewWidth, regularWidth, largeWidth int
	var previewHeight, regularHeight, largeHeight int
	for _, sizeJSON := range entry.Sizes.Size {
		// Certain images have obsolete video media entries with a different schema
		var mediaData struct {
			Media string
		}
		if err := json.Unmarshal(sizeJSON, &mediaData); err == nil {
			if mediaData.Media == "video" {
				continue
			}
		}

		var size sizeData
		if err := json.Unmarshal(sizeJSON, &size); err != nil {
			log.Printf("Failed to unmarshal size: %v: got %s", err, sizeJSON)
			continue
		}

		switch size.Label {
		case "Thumbnail":
			previewSrc = size.Source
			previewWidth = size.Width
			previewHeight = size.Height
			continue
		case "Medium":
			regularSrc = size.Source
			regularWidth = size.Width
			regularHeight = size.Height
			continue
		case "Original":
			// The original images has nuances like exif rotation that we don't want
			// to deal with.
			continue
		}

		if size.Width > largeWidth || size.Height > largeHeight {
			largeSrc = size.Source
			largeWidth = size.Width
			largeHeight = size.Height
		}
	}

	owner := entry.Info.Owner

	var photographerIcon string
	if owner.Iconfarm > 0 {
		photographerIcon = "https://farm" + strconv.Itoa(owner.Iconfarm) + ".staticflickr.com/" + owner.Iconserver + "/buddyicons/" + owner.NSID + ".jpg"
	} else {
		photographerIcon = "https://combo.staticflickr.com/pw/images/buddyicon03.png"
	}

	photographerText := owner.Username

	var photographerLink, link string
	if owner.PathAlias != "" {
		photographerLink = "https://www.flickr.com/people/" + owner.PathAlias
		link = "https://www.flickr.com/photos/" + owner.PathAlias + "/" + entry.FlickrId
	} else {
		photographerLink = "https://www.flickr.com/people/" + owner.NSID
		link = "https://www.flickr.com/photos/" + owner.NSID + "/" + entry.FlickrId
	}

	title := entry.Info.Title.Content
	descriptionHtml := entry.Info.Description.Content

	var dateTaken *time.Time
	if entry.Info.Dates.Taken == "" {
		dateTaken = nil
	} else if value, err := time.Parse("2006-01-02 15:04:05", entry.Info.Dates.Taken); err == nil {
		dateTaken = &value
	} else {
		return fmt.Errorf("failed to parse date: %v", err)
	}

	rx := rand.Float64()
	ry := rand.Float64()

	tx, err := db.Begin(context.Background())
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())

	var challengeID int64
	err = tx.QueryRow(context.Background(), `
		INSERT INTO challenges
			(region_id,
			 geo,
			 preview_src, preview_width, preview_height,
			 regular_src, regular_width, regular_height,
			 large_src, large_width, large_height,
			 photographer_icon, photographer_text, photographer_link,
			 title, description_html, date_taken, link,
			 rx, ry)
		VALUES
			(
			 $1,
			 ST_SetSRID(ST_MakePoint($2, $3), 4326),
			 $4, $5, $6,
			 $7, $8, $9,
			 $10, $11, $12,
			 $13, $14, $15,
			 $16, $17, $18, $19,
			 $20, $21
			 )
		RETURNING id
	`,
		entry.RegionID,
		entry.Lng, entry.Lat,
		previewSrc, previewWidth, previewHeight,
		regularSrc, regularWidth, regularHeight,
		largeSrc, largeWidth, largeHeight,
		photographerIcon, photographerText, photographerLink,
		title, descriptionHtml, dateTaken, link,
		rx, ry,
	).Scan(&challengeID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(context.Background(), `
		INSERT INTO flickr_challenge_sources (flickr_id, challenge_id)
		VALUES ($1, $2)
	`, entry.FlickrId, challengeID)
	if err != nil {
		return err
	}

	return tx.Commit(context.Background())
}
