package main

import (
	"context"
	"contourguessr-ingest/repos"
	"errors"
	"flag"
	"fmt"
	"github.com/joho/godotenv"
	"github.com/minio/minio-go/v7"
	miniocredentials "github.com/minio/minio-go/v7/pkg/credentials"
	"log"
	"log/slog"
	"os"
	"strings"
)

const bucket = "contourguessr-photos"

var dryRun = flag.Bool("dry-run", false, "Don't make any changes, just print what would be done")

var repo *repos.Repo
var mc *minio.Client

func main() {
	ctx := context.Background()

	err := godotenv.Load(".env", ".env.local")
	if err != nil {
		slog.Info("no dotenv", err)
	}

	flag.Usage = func() {
		w := flag.CommandLine.Output()
		_, _ = fmt.Fprintln(w, "reconcile: Updates object store to match the database")
		flag.PrintDefaults()
	}
	flag.Parse()

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

	err = doMain(ctx)
	if err != nil {
		log.Fatal(err)
	}
}

func doMain(ctx context.Context) error {
	if *dryRun {
		slog.Warn("Dry run mode")
	} else {
		slog.Info("Updating object store")
	}

	var count int
	for obj := range mc.ListObjects(ctx, bucket, minio.ListObjectsOptions{Prefix: "flickr/"}) {
		if obj.Err != nil {
			log.Fatal(obj.Err)
		}

		keyParts := strings.Split(obj.Key, "/")
		if len(keyParts) != 3 {
			log.Println("Unexpected key:", obj.Key)
		}
		id := keyParts[1]

		_, err := repo.GetPhoto(ctx, id)

		if errors.Is(err, repos.ErrNotFound) {
			log.Printf("Deleting %s", obj.Key)
			if !*dryRun {
				err = mc.RemoveObject(ctx, bucket, obj.Key+"medium.jpg", minio.RemoveObjectOptions{})
				if err != nil {
					return err
				}
				err = mc.RemoveObject(ctx, bucket, obj.Key+"large.jpg", minio.RemoveObjectOptions{})
				if err != nil {
					return err
				}
			}
		} else if err != nil {
			return err
		}

		count++
		if count%1000 == 0 {
			log.Printf("Processed %d", count)
		}
	}
	return nil
}

func mustGetEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("%s not set", key)
	}
	return value
}
