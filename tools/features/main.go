package main

import (
	"context"
	"contourguessr-ingest/compute_features"
	"contourguessr-ingest/compute_features/elevation"
	"contourguessr-ingest/compute_features/ml"
	"contourguessr-ingest/overpass"
	"contourguessr-ingest/repos"
	"fmt"
	"github.com/joho/godotenv"
	"log"
	"log/slog"
	"os"
	"time"
)

var repo *repos.Repo
var bingClient *elevation.BingMaps
var mlClient *ml.Client
var ovpClient *overpass.Client

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level:     slog.LevelDebug,
		AddSource: true,
	}))
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

	bingKey := mustGetEnv("BING_MAPS_KEY")
	bingClient = elevation.NewBingMaps(bingKey)

	mlEndpoint := mustGetEnv("CLASSIFIER_ENDPOINT")
	mlClient = ml.New(mlEndpoint)

	ovpEndpoint := mustGetEnv("OVERPASS_ENDPOINT")
	ovpClient = overpass.New(ovpEndpoint)

	err = doMain()
	if err != nil {
		log.Fatal(err)
	}
}

func doMain() error {
	for {
		start := time.Now()
		isComplete, err := doBatch()
		slog.Info("completed batch", "duration_secs", time.Since(start).Seconds())
		if err != nil {
			return err
		}
		if isComplete {
			slog.Info("all done")
			return nil
		}
	}
}

func doBatch() (bool, error) {
	ctx := context.Background()

	photos, err := repo.GetPhotosWithoutFeatures(ctx, 100)
	if err != nil {
		return false, fmt.Errorf("get photos without features: %w", err)
	}
	if len(photos) == 0 {
		return true, nil
	}

	feats, err := compute_features.Compute(
		ctx,
		bingClient,
		mlClient,
		ovpClient,
		photos,
	)
	if err != nil {
		return false, fmt.Errorf("compute features: %w", err)
	}

	err = repo.SaveFeatures(ctx, feats)
	if err != nil {
		return false, fmt.Errorf("save features: %w", err)
	}

	return false, nil
}

func mustGetEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("%s not set", key)
	}
	return value
}
