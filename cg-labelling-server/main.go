package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/joho/godotenv"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

/*
CREATE TABLE labels
(
    id              SERIAL PRIMARY KEY,
    flickr_photo_id TEXT REFERENCES flickr_photos (id) ON DELETE CASCADE NOT NULL,
    is_positive     BOOLEAN
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL
);
*/

var regions []string
var pool *pgxpool.Pool

//go:embed index.html
var indexHTML string

//go:embed list.html
var listHTML string

func main() {
	err := godotenv.Load(".env", ".local.env")
	if err != nil {
		log.Println("failed to load dotenv file(s): ", err)
	}

	dbURL := os.Getenv("INGEST_DB")
	if dbURL == "" {
		panic("missing INGEST_DB env var")
	}

	appEnv := os.Getenv("APP_ENV")

	ctx := context.Background()

	pool, err = pgxpool.Connect(ctx, dbURL)
	if err != nil {
		panic("failed to connect to INGEST_DB")
	}

	regions = listRegionsInDB()

	mux := http.NewServeMux()
	mux.Handle("GET /", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if appEnv == "dev" {
			value, err := os.ReadFile("cg-labelling-server/index.html")
			if err != nil {
				panic(err)
			}
			_, _ = w.Write(value)
		} else {
			_, _ = w.Write([]byte(indexHTML))
		}
	}))
	mux.Handle("GET /list", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if appEnv == "dev" {
			value, err := os.ReadFile("cg-labelling-server/list.html")
			if err != nil {
				panic(err)
			}
			_, _ = w.Write(value)
		} else {
			_, _ = w.Write([]byte(listHTML))
		}
	}))

	mux.Handle("GET /api/v0/batch", http.HandlerFunc(getBatchHandler))
	mux.Handle("POST /api/v0/batch", http.HandlerFunc(postBatchHandler))
	mux.Handle("GET /api/v0/list", http.HandlerFunc(getListHandler))
	mux.Handle("GET /api/v0/stats", http.HandlerFunc(getStatsHandler))

	addr := "localhost:5050"
	log.Println("Listening on", addr)
	err = http.ListenAndServe(addr, mux)
	if err != nil {
		panic(err)
	}
}

func getStatsHandler(w http.ResponseWriter, r *http.Request) {
	stats, err := getStats()
	if err != nil {
		log.Println("error in getStats: ", err)
		http.Error(w, fmt.Sprintf("%s", err), http.StatusInternalServerError)
		return
	}

	err = json.NewEncoder(w).Encode(stats)
	if err != nil {
		panic(err)
		return
	}
}

type Stats struct {
	Positive int `json:"positive"`
	Negative int `json:"negative"`
}

func getStats() (stats Stats, err error) {
	ctx := context.Background()
	err = pool.QueryRow(ctx, `
SELECT
	(SELECT COUNT(*) FROM labels WHERE is_positive = true) as positive,
	(SELECT COUNT(*) FROM labels WHERE is_positive = false) as negative
`).Scan(&stats.Positive, &stats.Negative)
	return
}

func getListHandler(w http.ResponseWriter, r *http.Request) {
	pageS := r.URL.Query().Get("page")
	page := 0
	if pageS != "" {
		var err error
		page, err = strconv.Atoi(pageS)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid parameter page: %s", err), http.StatusBadRequest)
			return
		}
	}

	batch, err := listLabelled(r.Context(), page)
	if err != nil {
		log.Println("error in listLabelled: ", err)
		http.Error(w, fmt.Sprintf("%s", err), http.StatusInternalServerError)
		return
	}

	err = json.NewEncoder(w).Encode(batch)
	if err != nil {
		panic(err)
		return
	}
}

type LabelledListEntry struct {
	Id         string `json:"id"`
	IsPositive bool   `json:"is_positive"`
	Region     string `json:"region"`
	WebURL     string `json:"web_url"`
	SmallSrc   string `json:"small_src"`
}

func listLabelled(ctx context.Context, page int) (batch []LabelledListEntry, err error) {
	pageSize := 50
	var rows pgx.Rows
	rows, err = pool.Query(ctx, `
SELECT l.is_positive, p.id, p.region, p.web_url, p.small_src
FROM labels as l
         INNER JOIN flickr_photos p on p.id = l.flickr_photo_id
ORDER BY l.created_at DESC
LIMIT $1
OFFSET $1::int * $2::int`, pageSize, page)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		entry := LabelledListEntry{}
		err = rows.Scan(&entry.IsPositive, &entry.Id, &entry.Region, &entry.WebURL, &entry.SmallSrc)
		if err != nil {
			return
		}
		batch = append(batch, entry)
	}

	err = rows.Err()
	return
}

func getBatchHandler(w http.ResponseWriter, r *http.Request) {
	sizeS := r.URL.Query().Get("per_region_size")
	size := 10
	if sizeS != "" {
		var err error
		size, err = strconv.Atoi(sizeS)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid parameter size: %s", err), http.StatusBadRequest)
			return
		}
	}

	batch, err := loadBatchToLabel(size)
	if err != nil {
		log.Println("error in loadBatchToLabel: ", err)
		http.Error(w, fmt.Sprintf("%s", err), http.StatusInternalServerError)
		return
	}

	err = json.NewEncoder(w).Encode(batch)
	if err != nil {
		panic(err)
		return
	}
}

func postBatchHandler(w http.ResponseWriter, r *http.Request) {
	var batch LabelledBatch
	err := json.NewDecoder(r.Body).Decode(&batch)
	if err != nil {
		http.Error(w, fmt.Sprintf("%s", err), http.StatusBadRequest)
		return
	}

	err = saveLabelledBatch(batch)
	if err != nil {
		log.Println("error in saveLabelledBatch: ", err)
		http.Error(w, fmt.Sprintf("%s", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

type LabelledBatch struct {
	Positive []string `json:"positive"`
	Negative []string `json:"negative"`
}

func saveLabelledBatch(batch LabelledBatch) (err error) {
	start := time.Now()
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return
	}
	defer tx.Rollback(ctx)

	for _, id := range batch.Positive {
		err = saveLabel(ctx, tx, id, true)
		if err != nil {
			return
		}
	}
	for _, id := range batch.Negative {
		err = saveLabel(ctx, tx, id, false)
		if err != nil {
			return
		}
	}

	err = tx.Commit(ctx)
	if err != nil {
		return
	}

	log.Printf("Saved labelled batch in %s", time.Since(start))
	return
}

func saveLabel(ctx context.Context, tx pgx.Tx, id string, isPositive bool) (err error) {
	if strings.HasPrefix(id, "flickr:") {
		flickrId := strings.TrimPrefix(id, "flickr:")
		_, err = tx.Exec(ctx, `INSERT INTO labels (flickr_photo_id, is_positive) VALUES ($1, $2)`, flickrId, isPositive)
		return
	} else {
		return errors.New("invalid id")
	}

}

type PicToLabel struct {
	Id         string `json:"id"`
	Region     string `json:"region"`
	WebURL     string `json:"web_url"`
	PreviewSrc string `json:"preview_src"`
}

func loadBatchToLabel(size int) (batch []PicToLabel, err error) {
	start := time.Now()
	ctx := context.Background()

	var conn *pgxpool.Conn
	conn, err = pool.Acquire(ctx)
	if err != nil {
		return
	}
	defer conn.Release()

	for _, region := range regions {
		var regionBatch []PicToLabel
		regionBatch, err = loadBatchToLabelForRegion(ctx, conn, region, size)
		batch = append(batch, regionBatch...)
	}

	log.Printf("Loaded batch to train in %s", time.Since(start))
	return
}

func loadBatchToLabelForRegion(ctx context.Context, conn *pgxpool.Conn, region string, size int) (batch []PicToLabel, err error) {
	var rows pgx.Rows
	rows, err = conn.Query(ctx, `
SELECT 'flickr:' || p.id as id, p.web_url, p.medium_src
FROM offroad_flickr_photos
         INNER JOIN flickr_photos as p ON p.id = offroad_flickr_photos.id
         LEFT JOIN labels as l ON p.id = l.flickr_photo_id
WHERE p.region = $1 AND l IS NULL
ORDER BY random()
LIMIT $2`, region, size)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		entry := PicToLabel{Region: region}
		err = rows.Scan(&entry.Id, &entry.WebURL, &entry.PreviewSrc)
		if err != nil {
			return
		}
		batch = append(batch, entry)
	}

	err = rows.Err()
	if err != nil {
		return
	}

	return
}

func listRegionsInDB() []string {
	var out []string
	rows, err := pool.Query(context.Background(),
		`SELECT DISTINCT REGION FROM flickr_photos ORDER BY REGION`)
	if err != nil {
		log.Fatal("listRegionsInDB: ", err)
	}
	defer rows.Close()
	for rows.Next() {
		var region string
		if err := rows.Scan(&region); err != nil {
			log.Fatal("listRegionsInDB: ", err)
		}
		out = append(out, region)
	}
	if err := rows.Err(); err != nil {
		log.Fatal("listRegionsInDB: ", err)
	}
	return out
}
