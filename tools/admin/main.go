package main

import (
	"context"
	"contourguessr-ingest/admin"
	"contourguessr-ingest/repos"
	"github.com/joho/godotenv"
	"github.com/minio/minio-go/v7"
	miniocredentials "github.com/minio/minio-go/v7/pkg/credentials"
	"log"
	"log/slog"
	"os"
)

func main() {
	err := godotenv.Load(".env", ".env.local")
	if err != nil {
		slog.Info("no dotenv", err)
	}

	ctx := context.Background()

	databaseURL := mustGetEnv("DATABASE_URL")
	repo, err := repos.Connect(databaseURL)
	if err != nil {
		log.Fatal(err)
	}

	adminHost := os.Getenv("ADMIN_HOST")
	if adminHost == "" {
		adminHost = "0.0.0.0"
	}
	adminPort := os.Getenv("ADMIN_PORT")
	if adminPort == "" {
		adminPort = "8080"
	}

	minioEndpoint := mustGetEnv("MINIO_ENDPOINT")
	minioAccessKey := mustGetEnv("MINIO_ACCESS_KEY")
	minioSecretKey := mustGetEnv("MINIO_SECRET_KEY")
	mc, err := minio.New(minioEndpoint, &minio.Options{
		Creds:  miniocredentials.NewStaticV4(minioAccessKey, minioSecretKey, ""),
		Secure: true,
	})
	if err != nil {
		log.Fatal(err)
	}

	admin.Serve(
		ctx,
		repo,
		mc,
		mustGetEnv("ADMIN_MAPTILER_API_KEY"),
		adminHost+":"+adminPort,
	)
}

func mustGetEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("%s not set", key)
	}
	return value
}
