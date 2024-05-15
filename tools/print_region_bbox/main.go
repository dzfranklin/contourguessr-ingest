package main

import (
	"context"
	"contourguessr-ingest/repos"
	"flag"
	"fmt"
	"github.com/joho/godotenv"
	"log"
	"log/slog"
	"os"
)

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
	repo, err := repos.Connect(databaseURL)
	if err != nil {
		log.Fatal(err)
	}

	regions, err := repo.ListRegions(ctx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Region Bounding Boxes (left, bottom, right, top):")

	for _, region := range regions {
		bbox := region.Geo.Bound()
		fmt.Printf("%s: %.6f,%.6f,%.6f,%.6f\n", region.Name, bbox.Left(), bbox.Bottom(), bbox.Right(), bbox.Top())
	}
}

func mustGetEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("%s not set", key)
	}
	return value
}
