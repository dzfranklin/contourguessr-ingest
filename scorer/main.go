package main

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v4"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"log"
	"os"
	"time"
)

// TODO: Fetch exif. Compute elevation over terrain and whether the coordinates match the gps exif data (or if it's present)
// 		 Update only those new scores keeping the existing ones the same

const activeVsn = 1

var databaseURL string
var redisAddr string
var overpassEndpoint string
var classifierEndpoint string

var minFlickrImgRequestDelay = 250 * time.Millisecond
var maxFlickrImgRequestDelay = 5 * time.Second
var minIdleWait = 4 * time.Minute
var maxIdleWait = 5 * time.Minute
var minErrWait = 2 * time.Minute
var maxErrWait = 5 * time.Minute

func main() {
	// Environment variables

	err := godotenv.Load(".env", ".env.local")
	if err != nil {
		log.Println(err)
	}

	databaseURL = os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL not set")
	}

	redisAddr = os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		log.Fatal("REDIS_ADDR not set")
	}

	overpassEndpoint = os.Getenv("OVERPASS_ENDPOINT")
	if overpassEndpoint == "" {
		log.Fatal("OVERPASS_ENDPOINT not set")
	}

	classifierEndpoint = os.Getenv("CLASSIFIER_ENDPOINT")
	if classifierEndpoint == "" {
		log.Fatal("CLASSIFIER_ENDPOINT not set")
	}

	// End setup

	for {
		startTime := time.Now()
		count, err := scoreOneBatch()
		elapsedTime := time.Since(startTime)

		if err != nil {
			log.Println(err)
			randSleep(minErrWait, maxErrWait)
			continue
		}

		if count == 0 {
			log.Println("No photos to score")
			randSleep(minIdleWait, maxIdleWait)
		} else {
			log.Printf("Scored %d photos in %s", count, elapsedTime)
		}
	}
}

func scoreOneBatch() (int, error) {
	ctx := context.Background()

	db, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return 0, fmt.Errorf("error connecting to database: %w", err)
	}
	defer db.Close(ctx)

	rdb := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})

	batch, err := loadBatch(db)
	if err != nil {
		return 0, fmt.Errorf("error loading batch: %w", err)
	}

	for _, entry := range batch {
		if err := scoreEntry(db, rdb, entry); err != nil {
			return 0, fmt.Errorf("error scoring entry %+v: %w", entry, err)
		}
	}

	return len(batch), nil
}

func scoreEntry(db *pgx.Conn, rdb *redis.Client, entry Entry) error {
	ctx := context.Background()

	if entry.RoadWithin1000m == nil {
		value, err := queryRoadWithin1000m(entry.Lng, entry.Lat)
		if err != nil {
			return fmt.Errorf("error querying road within 1000m of %+v: %w\n", entry, err)
		}
		entry.RoadWithin1000m = &value
	}

	if entry.ValidityScore == nil && !*entry.RoadWithin1000m {
		photoData, err := fetchFlickrPhoto(db, entry.FlickrId, entry.PreviewURL)
		if err != nil {
			return fmt.Errorf("error fetching flickr photo %+v: %w", entry, err)
		}

		validity, err := queryValidity(photoData)
		if err != nil {
			return fmt.Errorf("error querying validity of %+v: %w", entry, err)
		}

		entry.ValidityScore = &validity.Score
		entry.ValidityModel = &validity.Model
	}

	if !*entry.RoadWithin1000m && entry.ValidityScore != nil && *entry.ValidityScore > 0.5 {
		if entry.Exif == nil {
			if err := rdb.LPush(ctx, "cg-flickr-indexer:want-exif", entry.FlickrId).Err(); err != nil {
				return err
			}
		} else {
			// TODO: use
		}
	}

	if err := entry.Save(db); err != nil {
		return fmt.Errorf("error saving score: %w", err)
	}

	return nil
}
