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
	"log/slog"
	"math/rand"
	"os"
	"strconv"
	"time"
)

var repo *repos.Repo
var mc *minio.Client
var fc *flickr.Client

var debugOnlyRegion *int

func main() {
	ctx := context.Background()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if os.Getenv("APP_ENV") == "development" {
		logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
	}
	slog.SetDefault(logger)

	err := godotenv.Load(".env", ".env.local")
	if err != nil {
		slog.Info("no dotenv", "err", err)
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

	debugOnlyRegionS := os.Getenv("DEBUG_ONLY_REGION")
	if debugOnlyRegionS != "" {
		val, err := strconv.Atoi(debugOnlyRegionS)
		if err != nil {
			log.Fatal("DEBUG_ONLY_REGION must be an integer")
		}
		debugOnlyRegion = &val
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

	slog.Info("Starting indexer")
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

		if debugOnlyRegion != nil && region.Id != *debugOnlyRegion {
			slog.Warn("Skipping due to DEBUG_ONLY_REGION", "region_id", region.Id, "region_name", region.Name)
			continue
		}

		if cursor.LastCheck == nil || cursor.LastCheck.Add(24*time.Hour*7).Before(time.Now()) {
			candidates = append(candidates, regionState{region, cursor})
		}
	}

	if len(candidates) == 0 {
		return true
	}

	pick := candidates[rand.Intn(len(candidates))]
	slog.Info("step start", "region_id", pick.Id, "region_name", pick.Name, "candidate_regions", len(candidates), "total_regions", len(regions))

	cursor, photos, err := flickrindexer.Step(ctx, fc, mc, pick.Region, pick.Cursor)
	if err != nil {
		log.Fatal(fmt.Errorf("error doing step on region %d: %w", pick.Id, err))
	}

	err = repo.SaveStep(ctx, pick.Id, cursor, photos)
	if err != nil {
		log.Fatal(fmt.Errorf("error saving step of region %d: %w", pick.Id, err))
	}

	elapsed := time.Since(start)
	slog.Info("step end", "region", pick.Id, "photos", len(photos), "elapsed", elapsed)

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
