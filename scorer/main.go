package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v4"
	"github.com/joho/godotenv"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// TODO: Fetch exif. Compute elevation over terrain and whether the coordinates match the gps exif data (or if it's present)
// 		 Update only those new scores keeping the existing ones the same

const activeVsn = 1

var databaseURL string
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

	batch, err := loadUnscoredBatch(db)
	if err != nil {
		return 0, fmt.Errorf("error loading unscored batch: %w", err)
	}

	for _, photo := range batch {
		roadWithin1000m, err := queryRoadWithin1000m(photo.Lng, photo.Lat)
		if err != nil {
			return 0, fmt.Errorf("error querying road within 1000m of %v: %w\n", photo, err)
		}

		var validity *validityResult
		if !roadWithin1000m {
			photoData, err := fetchFlickrPhoto(db, photo.FlickrId, photo.PreviewURL)
			if err != nil {
				return 0, fmt.Errorf("error fetching flickr photo %+v: %w", photo, err)
			}

			validity, err = queryValidity(photoData)
			if err != nil {
				return 0, fmt.Errorf("error querying validity of %+v: %w", photo, err)
			}
		}

		err = saveScore(db, photo.FlickrId, roadWithin1000m, validity)
		if err != nil {
			return 0, fmt.Errorf("error saving score for %+v: %w", photo, err)
		}
	}

	return len(batch), nil
}

type unscoredPhoto struct {
	FlickrId   string
	PreviewURL string
	Lng        float64
	Lat        float64
}

func loadUnscoredBatch(db *pgx.Conn) ([]unscoredPhoto, error) {
	ctx := context.Background()
	rows, err := db.Query(ctx, `
		SELECT p.flickr_id,
			   summary ->> 'server',
			   summary ->> 'secret',
			   ST_X(p.geo::geometry),
			   ST_Y(p.geo::geometry)
		FROM flickr_photos as p
		WHERE not exists (SELECT 1
						  FROM photo_scores as s
						  WHERE s.vsn = $1
							AND flickr_photo_id = p.flickr_id
							AND s.is_complete)
		  and not exists (SELECT 1
						  FROM flickr_photo_fetch_failures as err
						  WHERE err.flickr_id = p.flickr_id)
		ORDER BY random()
		LIMIT 100
	`, activeVsn)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]unscoredPhoto, 0)
	for rows.Next() {
		var server string
		var secret string
		var p unscoredPhoto
		err := rows.Scan(&p.FlickrId, &server, &secret, &p.Lng, &p.Lat)
		if err != nil {
			return nil, err
		}
		p.PreviewURL = "https://live.staticflickr.com/" + server + "/" + p.FlickrId + "_" + secret + "_m.jpg"
		out = append(out, p)
	}
	return out, nil
}

func queryRoadWithin1000m(lng, lat float64) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	reqBody := strings.NewReader(fmt.Sprintf(`
		[out:json];
		wr(around:1000,%0.6f,%0.6f)[highway];
		out tags;
	`, lat, lng))

	req, err := http.NewRequestWithContext(ctx, "POST", overpassEndpoint+"/interpreter", reqBody)
	if err != nil {
		return false, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	var ovpResp struct {
		Elements []struct {
			Tags map[string]string `json:"tags"`
		} `json:"elements"`
	}
	err = json.NewDecoder(resp.Body).Decode(&ovpResp)
	if err != nil {
		return false, err
	}

	for _, elem := range ovpResp.Elements {
		highway := elem.Tags["highway"]
		surface := elem.Tags["surface"]

		// If paved then it is a road
		if tagValueContains(surface,
			"paved",    // A feature that is predominantly paved; i.e., it is covered with paving stones, concrete or bitumen
			"asphalt",  // Short for asphalt concrete
			"chipseal", // Less expensive alternative to asphalt concrete. Rarely tagged
			"concrete", // Portland cement concrete, forming a large surface
		) {
			return true, nil
		}

		// If the highway value matches the denylist then assume it is a road. There is a long tail of weird tags that
		// we err on the side of assuming aren't roads.
		if tagValueContains(highway,
			// From OSM wiki
			"motorway",      // A restricted access major divided highway,
			"trunk",         // The most important roads in a country's system that aren't motorways
			"primary",       // After trunk
			"secondary",     // After primary
			"tertiary",      // After secondary
			"unclassified",  // The least important through roads in a country's system. The word 'unclassified' is a historical artefact of the UK road system and does not mean that the classification is unknown
			"residential",   // Roads which serve as access to housing, without function of connecting settlements
			"motorway_link", // The link roads (sliproads/ramps) leading to/from a motorway from/to a motorway or lower class highway
			"trunk_link",
			"primary_link",
			"secondary_link",
			"tertiary_link",
			"living_street", // residential streets where pedestrians have legal priority over cars
			"service",       // For access roads to, or within an industrial estate, camp site, business park, car park, alleys, etc
			"raceway",       // A course or track for (motor) racing
			"busway",        // Dedicated roadway for buses
			"rest_area",
			// From our examples
		) {
			return true, nil
		}
	}

	return false, nil
}

func tagValueContains(tagValue string, needles ...string) bool {
	values := strings.Split(tagValue, ";")
	for _, v := range values {
		for _, n := range needles {
			if strings.Contains(v, n) {
				return true
			}
			if strings.HasPrefix(v, n+":") {
				return true
			}
		}
	}
	return false
}

type validityResult struct {
	Score float64 `json:"validity_score"`
	Model string  `json:"model"`
}

func queryValidity(photoData []byte) (*validityResult, error) {
	for retry := 0; ; retry++ {
		randSleep(time.Duration(retry)*time.Second, time.Duration(retry+1)*time.Second)
		result, err := queryValidityNoRetry(photoData)
		if err == nil {
			return result, nil
		}

		log.Printf("Error querying validity, retry %d: %v", retry, err)
		if retry >= 5 {
			return nil, err
		}
	}
}

func queryValidityNoRetry(photoData []byte) (*validityResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	reqData := struct {
		ImageBase64 string `json:"image_base64"`
	}{base64.StdEncoding.EncodeToString(photoData)}
	reqBody, err := json.Marshal(reqData)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", classifierEndpoint+"/api/v0/classify", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result validityResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

func saveScore(db *pgx.Conn, flickrId string, roadWithin1000m bool, validity *validityResult) error {
	var validityScore *float64
	var validityModel *string
	if validity != nil {
		validityScore = &validity.Score
		validityModel = &validity.Model
	}

	ctx := context.Background()
	_, err := db.Exec(ctx, `
		INSERT INTO photo_scores (vsn, updated_at, flickr_photo_id,
		                          road_within_1000m, validity_score, validity_model)
		VALUES ($1, CURRENT_TIMESTAMP, $2, $3, $4, $5)
	`, activeVsn, flickrId, roadWithin1000m, validityScore, validityModel)
	return err
}

var fetchFlickrPhotoMu sync.Mutex

func fetchFlickrPhoto(db *pgx.Conn, flickrId string, photoURL string) ([]byte, error) {
	startTime := time.Now()

	fetchFlickrPhotoMu.Lock()
	defer fetchFlickrPhotoMu.Unlock()

	randSleep(minFlickrImgRequestDelay, maxFlickrImgRequestDelay)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	imgReq, err := http.NewRequestWithContext(ctx, "GET", photoURL, nil)
	if err != nil {
		return nil, err
	}
	imgReq.Header.Set("User-Agent", "contourguessr.org (contact github.com/dzfranklin/contourguessr-ingest or daniel@danielzfranklin.org)")

	reqTime := time.Now()
	imgResp, err := http.DefaultClient.Do(imgReq)
	if err != nil {
		saveFlickrPhotoFetchFailure(db, flickrId, fmt.Errorf("request: %w", err))
		return nil, err
	}
	defer imgResp.Body.Close()

	if imgResp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(imgResp.Body)
		if err != nil {
			body = []byte(fmt.Sprintf("<error reading body: %s>", err))
		}
		err = fmt.Errorf("HTTP status %d: %s", imgResp.StatusCode, body)
		saveFlickrPhotoFetchFailure(db, flickrId, err)
		return nil, err
	}

	body, err := io.ReadAll(imgResp.Body)
	if err != nil {
		saveFlickrPhotoFetchFailure(db, flickrId, fmt.Errorf("read body: %w", err))
		return nil, err
	}

	log.Printf("Fetched flickr photo %s in %s (%s requesting)",
		photoURL, time.Since(startTime), time.Since(reqTime))

	return body, nil
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

func randSleep(min time.Duration, max time.Duration) {
	dur := time.Duration(rand.Int63n(int64(max-min))) + min
	if dur > 5*time.Minute {
		log.Printf("Sleeping for %s", dur)
	}
	time.Sleep(dur)
}
