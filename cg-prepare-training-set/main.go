package main

import (
	"context"
	"github.com/jackc/pgx/v4"
	"github.com/joho/godotenv"
	flag "github.com/spf13/pflag"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

var dbURL string
var outDir = "/tmp/cg-training-set"

func init() {
	// Environment variables

	err := godotenv.Load(".env", ".local.env")
	if err != nil {
		log.Println(err)
	}

	dbURL = os.Getenv("INGEST_DB")
	if dbURL == "" {
		log.Fatal("INGEST_DB not set")
	}

	// Flags

	flag.Parse()
}

func main() {
	ctx := context.Background()

	db, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close(ctx)
	if err := db.Ping(ctx); err != nil {
		log.Fatal("failed to connect to database: ", err)
	}

	entries := loadLabels(db)

	c := &http.Client{}
	c.Timeout = 10 * time.Second

	err = os.MkdirAll(outDir+"/p", 0755)
	if err != nil {
		log.Fatal(err)
	}
	err = os.MkdirAll(outDir+"/n", 0755)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Downloading %d pictures", len(entries))
	for n, entry := range entries {
		log.Printf("Downloading %s (%0.1f%%)", entry.ID, 100*float64(n)/float64(len(entries)))
		doDownload(c, entry)
	}
}

type Entry struct {
	ID         string `json:"id"`
	Src        string `json:"src"`
	IsPositive bool   `json:"is_positive"`
}

var downloadFailuresMu sync.Mutex
var consecutiveDownloadFailures int

func doDownload(c *http.Client, entry Entry) {
	path := outDir
	if entry.IsPositive {
		path += "/p"
	} else {
		path += "/n"
	}
	path += "/" + entry.ID + ".jpg"

	if _, err := os.Stat(path); err == nil {
		log.Println("already downloaded")
		return
	}

	time.Sleep(1000 * time.Millisecond)

	req, err := http.NewRequest(http.MethodGet, entry.Src, nil)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("User-Agent", "contourguessr.org (contact daniel@danielzfranklin.org)")
	resp, err := c.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Println("Download failed: status ", resp.StatusCode)
		downloadFailuresMu.Lock()
		consecutiveDownloadFailures++
		if consecutiveDownloadFailures > 10 {
			log.Fatal("Too many consecutive download failures")
		}
		downloadFailuresMu.Unlock()
		return
	}

	file, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		log.Fatal(err)
	}
}

func loadLabels(db *pgx.Conn) []Entry {
	rows, err := db.Query(context.Background(), `
SELECT 'flickr:' || p.id, p.small_src, l.is_positive
FROM flickr_photos as p
         INNER JOIN labels l on p.id = l.flickr_photo_id
ORDER BY p.id`)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var entry Entry
		if err := rows.Scan(&entry.ID, &entry.Src, &entry.IsPositive); err != nil {
			log.Fatal(err)
		}
		entries = append(entries, entry)
	}
	return entries
}
