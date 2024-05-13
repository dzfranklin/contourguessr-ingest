package main

import (
	"context"
	"contourguessr-ingest/admin"
	"contourguessr-ingest/flickr"
	"contourguessr-ingest/flickrindexer"
	"contourguessr-ingest/repos"
	"fmt"
	"github.com/joho/godotenv"
	"github.com/minio/minio-go/v7"
	miniocredentials "github.com/minio/minio-go/v7/pkg/credentials"
	"log"
	"math/rand"
	"os"
	"time"
)

var repo *repos.Repo
var mc *minio.Client
var fc *flickr.Client

func main() {
	ctx := context.Background()

	err := godotenv.Load(".env", ".env.local")
	if err != nil {
		log.Println(err)
	}

	databaseURL := mustGetEnv("DATABASE_URL")
	repo, err = repos.Connect(databaseURL)
	if err != nil {
		log.Fatal(err)
	}

	minioEndpoint := mustGetEnv("MINIO_ENDPOINT")
	minioAccessKey := mustGetEnv("MINIO_ACCESS_KEY")
	minioSecretKey := mustGetEnv("MINIO_SECRET_KEY")
	mc, err = minio.New(minioEndpoint, &minio.Options{
		Creds:  miniocredentials.NewStaticV4(minioAccessKey, minioSecretKey, ""),
		Secure: true,
	})
	if err != nil {
		log.Fatal(err)
	}

	flickrApiKey := mustGetEnv("FLICKR_API_KEY")
	flickrEndpoint := mustGetEnv("FLICKR_ENDPOINT")
	fc, err = flickr.New(flickrApiKey, flickrEndpoint)
	if err != nil {
		log.Fatal(err)
	}
	if os.Getenv("FLICKR_SKIP_WAITS") != "" {
		fc.SkipWaits = true
	}

	if os.Getenv("VERBOSE_INDEXER") != "" {
		ctx = flickrindexer.NewVerboseContext(ctx)
	}

	adminHost := os.Getenv("ADMIN_HOST")
	if adminHost == "" {
		adminHost = "0.0.0.0"
	}
	adminPort := os.Getenv("ADMIN_PORT")
	if adminPort == "" {
		adminPort = "8080"
	}

	go admin.Serve(
		ctx,
		repo.Pool(),
		mc,
		mustGetEnv("ADMIN_MAPTILER_API_KEY"),
		adminHost+":"+adminPort,
	)

	log.Println("Starting indexer")
	for {
		complete := doStep(ctx)
		if complete {
			if ctx.Err() != nil {
				log.Fatal(ctx.Err())
			}
			time.Sleep(1*time.Minute + time.Duration(rand.Intn(60))*time.Second)
		}
	}
}

func doStep(ctx context.Context) bool {
	start := time.Now()

	regions, err := repo.ListRegions(ctx)
	if err != nil {
		log.Fatal(err)
	}

	var candidates []regionState
	for _, region := range regions {
		cursor, err := repo.GetCursor(ctx, region.Id)
		if err != nil {
			log.Fatal(err)
		}

		if cursor.LastCheck == nil || cursor.LastCheck.Add(24*time.Hour*7).Before(time.Now()) {
			candidates = append(candidates, regionState{region, cursor})
		}
	}

	if len(candidates) == 0 {
		return true
	}

	pick := candidates[rand.Intn(len(candidates))]
	log.Printf("step start: %d of %d regions need checking, picked region %d (%s)", len(candidates), len(regions), pick.Id, pick.Name)

	cursor, photos, err := flickrindexer.Step(ctx, fc, mc, pick.Region, pick.Cursor)
	if err != nil {
		log.Fatal(fmt.Errorf("error doing step on region %d: %w", pick.Id, err))
	}

	err = repo.SaveStep(ctx, pick.Id, cursor, photos)
	if err != nil {
		log.Fatal(fmt.Errorf("error saving step of region %d: %w", pick.Id, err))
	}

	elapsed := time.Since(start)
	log.Printf("step end: inserted %d photos of region %d in %s", len(photos), pick.Id, elapsed)

	return false
}

type regionState struct {
	repos.Region
	Cursor repos.Cursor
}

func mustGetEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("%s not set", key)
	}
	return value
}
