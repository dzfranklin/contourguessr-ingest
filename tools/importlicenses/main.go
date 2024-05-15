package main

import (
	"context"
	"contourguessr-ingest/flickr"
	"contourguessr-ingest/repos"
	"github.com/joho/godotenv"
	"log"
	"log/slog"
	"os"
)

func main() {
	err := godotenv.Load(".env", ".env.local")
	if err != nil {
		slog.Info("no dotenv", "err", err)
	}

	databaseURL := mustGetEnv("DATABASE_URL")
	repo, err := repos.Connect(databaseURL)
	if err != nil {
		log.Fatal(err)
	}

	flickrApiKey := mustGetEnv("FLICKR_API_KEY")
	flickrEndpoint := mustGetEnv("FLICKR_ENDPOINT")
	fc, err := flickr.New(flickrApiKey, flickrEndpoint)
	if err != nil {
		log.Fatal(err)
	}
	if os.Getenv("FLICKR_SKIP_WAITS") != "" {
		fc.SkipWaits = true
	}

	ctx := context.Background()

	var resp struct {
		Licenses struct {
			License []struct {
				Id   int
				Name string
				URL  string
			}
		}
	}
	err = fc.Call(ctx, "flickr.photos.licenses.getInfo", &resp, nil)
	if err != nil {
		log.Fatal(err)
	}

	for _, license := range resp.Licenses.License {
		repo.UpsertFlickrLicense(ctx, license.Id, license.Name, license.URL)
	}
}

func mustGetEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("%s not set", key)
	}
	return value
}
