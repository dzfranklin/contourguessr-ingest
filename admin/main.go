package main

import (
	"context"
	"contourguessr-ingest/admin/routes"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"log"
	"net/http"
	"os"
)

var db *pgxpool.Pool

func main() {
	// Load environment variables

	err := godotenv.Load(".env", ".env.local")
	if err != nil {
		log.Println(err)
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL not set")
	}

	maptilerApiKey := os.Getenv("ADMIN_MAPTILER_API_KEY")
	if maptilerApiKey == "" {
		log.Fatal("ADMIN_MAPTILER_API_KEY not set")
	}

	host := os.Getenv("HOST")
	if host == "" {
		host = "0.0.0.0"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := host + ":" + port

	appEnv := os.Getenv("APP_ENV")

	// Setup globals

	routes.MaptilerAPIKey = maptilerApiKey

	db, err = pgxpool.Connect(context.Background(), databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	routes.Db = db

	rdb := redis.NewClient(&redis.Options{
		Addr: os.Getenv("REDIS_ADDR"),
	})
	routes.Rdb = rdb

	// Serve

	mux := routes.Mux()

	log.Printf("Listening on %s (APP_ENV=%s)\n", addr, appEnv)
	log.Fatal(http.ListenAndServe(addr, mux))
}
