package main

import (
	"context"
	"contourguessr-ingest/flickr"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v4"
	"github.com/joho/godotenv"
	flag "github.com/spf13/pflag"
	"log"
	"os"
	"strings"
	"time"
)

/*
CREATE TABLE flickr_photos
(
id            TEXT PRIMARY KEY,
region        TEXT,
inserted_at   TIMESTAMP DEFAULT NOW(),

owner         TEXT,
shared_secret TEXT,
server        TEXT,
date_taken    TIMESTAMP,
geom          GEOMETRY(Point, 4326),
geom_accuracy FLOAT,

summary       jsonb, -- from flickr.photos.search
info          jsonb, -- from flickr.photos.getInfo
sizes         jsonb, -- from flickr.photos.getSizes
exif          jsonb, -- from flickr.photos.getExif
raw_exif      jsonb  -- from flickr.photos.getExif

gps_altitude FLOAT GENERATED ALWAYS AS (
        CASE
            WHEN exif ->> 'GPSAltitude' LIKE '% m' THEN TRIM((exif ->> 'GPSAltitude'), ' m')::float
            ELSE NULL
            END
        ) STORED
)
web_url TEXT GENERATED ALWAYS AS ('https://flickr.com/photos/' || owner || '/' || id) STORED
medium_src TEXT GENERATED ALWAYS AS ('https://live.staticflickr.com/' || server || '/' || id || '_' || shared_secret || '.jpg') STORED
small_src TEXT GENERATED ALWAYS AS ('https://live.staticflickr.com/' || server || '/' || id || '_' || shared_secret || '_m.jpg') STORED
*/

var minDate = time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)
var maxDate = time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)

var regions map[string]string

var regionPrefixFilter = flag.StringP("region", "r", "", "Prefix of region IDs to ingest")
var skipRegionPrefixFilter = flag.StringP("skip-region", "s", "", "Prefix of region IDs to skip")
var hydrateExif = flag.Bool("hydrate-exif", false, "Instead of scraping new photos hydrate exif data")
var hydrateOneExif = flag.String("hydrate-one-exif", "", "Hydrate exif data for a single photo")

var databaseURL string

func init() {
	regionsFile, err := os.Open("regions.json")
	if err != nil {
		log.Fatal(err)
	}
	defer regionsFile.Close()
	dec := json.NewDecoder(regionsFile)
	if err := dec.Decode(&regions); err != nil {
		log.Fatal(err)
	}

	// Environment variables

	err = godotenv.Load(".env", ".local.env")
	if err != nil {
		log.Println(err)
	}

	databaseURL = os.Getenv("INGEST_DB")
	if databaseURL == "" {
		log.Fatal("INGEST_DB not set")
	}

	// Flags

	flag.Parse()
}

func main() {
	ctx := context.Background()

	db, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close(ctx)
	err = db.Ping(ctx)
	if err != nil {
		log.Fatal("failed to connect to database: ", err)
	}

	if *hydrateOneExif != "" {
		exifMap, rawExif := getExif(*hydrateOneExif)
		exif, err := json.Marshal(exifMap)
		if err != nil {
			log.Fatal(err)
		}

		if _, err := db.Exec(context.Background(), `UPDATE flickr_photos
			SET exif = $2, raw_exif = $3
			WHERE id = $1`,
			*hydrateOneExif, exif, rawExif,
		); err != nil {
			log.Fatal("update ", err)
		}
		return
	}

	if *hydrateExif {
		completeRegions := make(map[string]bool)
		for len(completeRegions) < len(regions) {
			for region := range regions {
				if completeRegions[region] {
					continue
				}
				if doHydrateExif(db, region, 100) {
					completeRegions[region] = true
				}
			}
		}
	}

	for region, bbox := range regions {
		if *regionPrefixFilter != "" && !strings.HasPrefix(region, *regionPrefixFilter) {
			log.Printf("Skipping region %s (doesn't match prefix)", region)
			continue
		}
		if *skipRegionPrefixFilter != "" && strings.HasPrefix(region, *skipRegionPrefixFilter) {
			log.Printf("Skipping region %s (matches prefix)", region)
			continue
		}

		log.Printf("Scraping region %s", region)
		doScrape(db, region, bbox)
		log.Printf("Finished scraping region %s", region)
	}
}

// doHydrateExif hydrates exif data for photos in the given region
// that don't already have exif data.
//
// Returns true if there are no more photos to hydrate.
func doHydrateExif(db *pgx.Conn, region string, count int) bool {
	rows, err := db.Query(context.Background(),
		`SELECT id FROM flickr_photos
          WHERE region = $1 AND exif IS NULL
          ORDER BY random()
          LIMIT $2`,
		region, count,
	)
	if err != nil {
		log.Fatal("query", err)
	}
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return true
			}
			log.Fatal("scan", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		log.Fatal("rows", err)
	}
	rows.Close()

	for i, id := range ids {
		exifMap, rawExif := getExif(id)
		exif, err := json.Marshal(exifMap)
		if err != nil {
			log.Fatal(err)
		}

		if _, err := db.Exec(context.Background(), `UPDATE flickr_photos
			SET exif = $2, raw_exif = $3
			WHERE id = $1`,
			id, exif, rawExif,
		); err != nil {
			log.Fatal("update ", err)
		}

		log.Printf("%s: Hydrated %d of %d (%0.2f%%)", region, i+1, count, 100*float64(i+1)/float64(count))
	}

	return false
}

func getExif(id string) (map[string]string, json.RawMessage) {
	var resp struct {
		Photo struct {
			Exif json.RawMessage `json:"exif"`
		} `json:"photo"`
	}
	err := flickr.Call("flickr.photos.getExif", &resp, map[string]string{
		"photo_id": id,
	})
	if err != nil {
		log.Fatal(err)
	}

	if resp.Photo.Exif == nil {
		return nil, nil
	}

	var entries []struct {
		Raw struct {
			Content string `json:"_content"`
		} `json:"raw"`
		Tag        string `json:"tag"`
		Label      string `json:"label"`
		TagSpace   string `json:"tagspace"`
		TagSpaceID int    `json:"tagspaceid"`
	}
	if err := json.Unmarshal(resp.Photo.Exif, &entries); err != nil {
		log.Printf("Failed to parse exif: %s", err)
		return nil, nil
	}

	data := make(map[string]string)
	for _, value := range entries {
		data[value.Tag] = value.Raw.Content
	}

	return data, resp.Photo.Exif
}

type flickrSearchPage struct {
	Photos struct {
		Page    int               `json:"page"`
		Pages   int               `json:"pages"`
		PerPage int               `json:"perpage"`
		Total   int               `json:"total"`
		Photo   []json.RawMessage `json:"photo"`
	} `json:"photos"`
}

type flickrPhoto struct {
	ID        string `json:"id"`
	Owner     string `json:"owner"`
	Secret    string `json:"secret"`
	Server    string `json:"server"`
	DateTaken string `json:"datetaken"`
	Latitude  string `json:"latitude"`
	Accuracy  string `json:"accuracy"`
	Longitude string `json:"longitude"`
}

func doScrape(db *pgx.Conn, region string, bbox string) {
	page := 1
	for {
		resp := doSearch(bbox, page)

		if page == 1 {
			log.Printf("Scraping %d photos in region %s", resp.Photos.Total, region)
		}

		var batch pgx.Batch
		for _, photo := range resp.Photos.Photo {
			var parsed flickrPhoto
			if err := json.Unmarshal(photo, &parsed); err != nil {
				log.Printf("Failed to parse photo: %s", err)
				continue
			}

			batch.Queue(`INSERT INTO flickr_photos (id, region, owner, shared_secret, server, date_taken, geom, geom_accuracy, summary)
			VALUES ($1, $2, $3, $4, $5, $6, ST_SetSRID(ST_MakePoint($7, $8), 4326), $9, $10)
			ON CONFLICT (id) DO NOTHING`,
				parsed.ID, region, parsed.Owner, parsed.Secret, parsed.Server, parsed.DateTaken, parsed.Longitude, parsed.Latitude, parsed.Accuracy, photo)
		}

		results := db.SendBatch(context.Background(), &batch)
		for i := 0; i < batch.Len(); i++ {
			if _, err := results.Exec(); err != nil {
				log.Fatal(err)
			}
		}
		if err := results.Close(); err != nil {
			log.Fatal(err)
		}

		log.Printf("Scraped %d pictures on page %d of %d (%0.0f%%)", len(resp.Photos.Photo), resp.Photos.Page, resp.Photos.Pages,
			100*float64(resp.Photos.Page)/float64(resp.Photos.Pages))

		if resp.Photos.Page >= resp.Photos.Pages {
			break
		}
		page++
	}
}

func doSearch(bbox string, page int) flickrSearchPage {
	var resp flickrSearchPage
	err := flickr.Call("flickr.photos.search", &resp, map[string]string{
		"bbox":           bbox,
		"min_taken_date": fmt.Sprintf("%d", minDate.Unix()),
		"max_taken_date": fmt.Sprintf("%d", maxDate.Unix()),
		"sort":           "date-taken-asc",
		"safe_search":    "1",
		"content_type":   "1", // photos only
		"extras":         "geo,date_taken",
		"page":           fmt.Sprintf("%d", page),
	})
	if err != nil {
		log.Fatal(err)
	}
	return resp
}
