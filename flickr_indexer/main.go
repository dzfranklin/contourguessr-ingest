package main

import (
	"context"
	"contourguessr-ingest/flickr"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v4"
	"github.com/joho/godotenv"
	flag "github.com/spf13/pflag"
	"log"
	"math/rand"
	"os"
	"strconv"
	"time"
)

// Constants

var minInitialDelay = 15 * time.Second
var maxInitialDelay = 1 * time.Minute
var indexLoopSleepBase = time.Minute*5 + time.Second
var minDate = time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)
var minCheckInterval = time.Hour * 24 * 7
var overlapPeriod = time.Hour * 24

// Environment variables
var databaseURL string

// Arguments
var onlyRegion = flag.Int("only-region", -1, "Only process this region")

var db *pgx.Conn

// TODO: Add retry logic to flickr.Call

func main() {
	// Environment variables

	err := godotenv.Load(".env", ".env.local")
	if err != nil {
		log.Println(err)
	}

	if os.Getenv("DEBUG_SHORT_DELAYS") != "" {
		maxInitialDelay = 5 * time.Second
		indexLoopSleepBase = 15 * time.Second
	}

	databaseURL = os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL not set")
	}

	flag.Parse()

	ctx := context.Background()
	db, err = pgx.Connect(ctx, databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close(ctx)

	initialDelay := time.Duration(rand.Intn(int(maxInitialDelay)))
	if initialDelay < minInitialDelay {
		initialDelay = minInitialDelay
	}
	log.Printf("sleeping for initial delay of %s", initialDelay)
	time.Sleep(initialDelay)

	for {
		log.Println("starting index run")
		doIndex()
		log.Println("completed index run")

		indexLoopSleep := indexLoopSleepBase + time.Duration(rand.Intn(30))*time.Second
		log.Printf("sleeping for %s", indexLoopSleep)
		time.Sleep(indexLoopSleep)
	}
}

func doIndex() {
	regions, err := listRegions()
	if err != nil {
		log.Fatal(err)
	}
	rand.Shuffle(len(regions), func(i, j int) {
		regions[i], regions[j] = regions[j], regions[i]
	})
	indexTime := time.Now()

	for _, region := range regions {
		if *onlyRegion != -1 && region.RegionID != *onlyRegion {
			log.Printf("Skipping region %d because of flag", region.RegionID)
			continue
		}

		if region.LatestRequest.Valid && time.Since(region.LatestRequest.Time) < minCheckInterval {
			log.Printf("Skipping %+v", region)
			continue
		}

		bbox := fmt.Sprintf("%f,%f,%f,%f", region.MinLng, region.MinLat, region.MaxLng, region.MaxLat)

		var startDate time.Time
		if !region.LatestRequest.Valid {
			log.Printf("No progress for region %d, starting from %s", region.RegionID, minDate)
			startDate = minDate
		} else {
			startDate = region.LatestRequest.Time.Add(-overlapPeriod)
			log.Printf("Resuming region %d from %s", region.RegionID, startDate)
			if startDate.Before(minDate) {
				startDate = minDate
				log.Printf("Clamping to %s", startDate)
			}
		}

		// We search in steps of 300 days because flickr seems to limit searches to the
		// low hundreds of pages. This is clumsy but ends up working for our region sizes.
		// By searching in a step that doesn't line up with seasons we ensure in the long run the distribution is okay.

		latestRequest := startDate
		stepSize := time.Hour * 24 * 300
		for stepStart := startDate; !stepStart.After(indexTime); stepStart = stepStart.Add(stepSize) {
			stepEnd := stepStart.Add(stepSize)
			log.Printf("Downloading region %d %s to %s", region.RegionID, stepStart, stepEnd)
			for page := 1; ; page++ {
				resp, err := callFlickrSearch(bbox, stepStart, stepEnd, page)
				if err != nil {
					log.Fatal(err)
				}

				log.Printf("Processing page %d of %d", resp.Photos.Page, resp.Photos.Pages)

				for _, photo := range resp.Photos.Photo {
					var p flickr.Photo
					if err := json.Unmarshal(photo, &p); err != nil {
						log.Printf("Failed to unmarshal photo: %s (got %s)", err, photo)
					}

					dateUploadInt, err := strconv.ParseInt(p.DateUpload, 10, 64)
					if err != nil {
						log.Printf("Failed to parse dateupload: %s (got %s)", err, p.DateUpload)
						continue
					}
					dateUpload := time.Unix(dateUploadInt, 0)

					if dateUploadInt > latestRequest.Unix() {
						latestRequest = dateUpload
					}

					lng, err := strconv.ParseFloat(p.Longitude, 64)
					if err != nil {
						log.Printf("Failed to parse longitude: %s (got %s)", err, p.Longitude)
						continue
					}
					lat, err := strconv.ParseFloat(p.Latitude, 64)
					if err != nil {
						log.Printf("Failed to parse latitude: %s (got %s)", err, p.Latitude)
						continue
					}
					accuracy, err := strconv.ParseInt(p.Accuracy, 10, 64)
					if err != nil {
						log.Printf("Failed to parse accuracy: %s (got %s)", err, p.Accuracy)
						continue
					}

					inside, err := queryPointInsideRegion(lng, lat, region)
					if err != nil {
						log.Fatal(err)
					}

					if !inside {
						continue
					}

					err = savePhoto(p.ID, lng, lat, int(accuracy), photo, region.RegionID)
					if err != nil {
						log.Printf("Failed to save photo %s: %s", p.ID, err)
					}
				}

				err = updateProgress(region.RegionID, latestRequest)
				if err != nil {
					log.Fatal("failed to update progress", err)
				}

				if resp.Photos.Page >= resp.Photos.Pages {
					break
				}
			}

			latestRequest = stepEnd
			err = updateProgress(region.RegionID, latestRequest)
			if err != nil {
				log.Fatal("failed to update progress", err)
			}
		}
	}
}

type regionProgress struct {
	RegionID      int
	LatestRequest sql.NullTime
	MinLng        float64
	MinLat        float64
	MaxLng        float64
	MaxLat        float64
}

func listRegions() ([]regionProgress, error) {
	rows, err := db.Query(context.Background(), `
		SELECT r.id, p.latest_seen, r.min_lng, r.min_lat, r.max_lng, r.max_lat
		FROM regions as r
		LEFT JOIN flickr_indexer_progress as p ON r.id = p.region_id
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var regions []regionProgress
	for rows.Next() {
		var r regionProgress
		if err := rows.Scan(&r.RegionID, &r.LatestRequest,
			&r.MinLng, &r.MinLat, &r.MaxLng, &r.MaxLat,
		); err != nil {
			return nil, err
		}
		regions = append(regions, r)
	}
	return regions, nil
}

func queryPointInsideRegion(lng, lat float64, region regionProgress) (bool, error) {
	// It's a bit silly to do a whole database round trip just for this but there
	// isn't a good go library that supports this check.
	row := db.QueryRow(context.Background(), `
		SELECT ST_Covers(geo, ST_Point($1, $2, 4326))
		FROM regions
		WHERE id = $3
`, lng, lat, region.RegionID)
	var inside bool
	if err := row.Scan(&inside); err != nil {
		return false, err
	}
	return inside, nil
}

func savePhoto(id string, lng, lat float64, accuracy int, summary json.RawMessage, regionId int) error {
	_, err := db.Exec(context.Background(), `
		INSERT INTO flickr_photos (flickr_id, geo, geo_accuracy, summary, region_id)
		VALUES ($1, ST_Point($2, $3, 4326), $4, $5, $6)
		ON CONFLICT (flickr_id) DO NOTHING
`, id, lng, lat, accuracy, summary, regionId)
	return err
}

func updateProgress(regionID int, latestRequest time.Time) error {
	_, err := db.Exec(context.Background(), `
		INSERT INTO flickr_indexer_progress (region_id, latest_seen)
		VALUES ($1, $2)
		ON CONFLICT (region_id) DO UPDATE SET latest_seen = $2
	`, regionID, latestRequest)
	return err
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

func callFlickrSearch(bbox string, stepStart, stepEnd time.Time, page int) (flickrSearchPage, error) {
	time.Sleep(1 * time.Second)
	var resp flickrSearchPage
	err := flickr.Call("flickr.photos.search", &resp, map[string]string{
		"bbox":            bbox,
		"min_upload_date": fmt.Sprintf("%d", stepStart.Unix()),
		"max_upload_date": fmt.Sprintf("%d", stepEnd.Unix()),
		"sort":            "date-posted-asc",
		"safe_search":     "1",
		"content_type":    "1", // photos only
		"extras":          "geo,date_upload,date_taken",
		"page":            fmt.Sprintf("%d", page),
	})
	return resp, err
}
