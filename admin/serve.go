package admin

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"log"
	"log/slog"
	"net/http"
)

var db *pgxpool.Pool
var mc *minio.Client
var mtApiKey string

func Serve(
	ctx context.Context,
	pool *pgxpool.Pool,
	minioClient *minio.Client,
	maptilerAPIKey string,
	addr string,
) {
	// Setup globals
	db = pool
	mc = minioClient
	mtApiKey = maptilerAPIKey

	// Serve

	mux := Mux()

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		slog.Info("Shutting down admin server...")
		err := srv.Shutdown(context.Background())
		if err != nil {
			slog.Error(fmt.Sprintf("error shutting down admin server: %s", err))
		}
	}()

	log.Printf("Admin server listening on %s (APP_ENV=%s)\n", addr, appEnv)
	err := srv.ListenAndServe()
	if !errors.Is(err, http.ErrServerClosed) {
		slog.Error(fmt.Sprintf("error shutting down admin server: %s", err))
	}
}