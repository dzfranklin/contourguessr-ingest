package repos

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/tracelog"
	"log"
	"os"
)

type Repo struct {
	db *pgxpool.Pool
}

func Connect(databaseURL string) (*Repo, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}

	if os.Getenv("TRACE_SQL") != "" {
		l := &traceLogger{log.New(log.Writer(), "pgx: ", log.LstdFlags)}
		config.ConnConfig.Tracer = &tracelog.TraceLog{
			Logger:   l,
			LogLevel: tracelog.LogLevelInfo,
		}
	}

	db, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		log.Fatal(err)
	}

	return &Repo{db: db}, nil
}

func (r *Repo) Pool() *pgxpool.Pool {
	return r.db
}

type traceLogger struct {
	*log.Logger
}

func (l *traceLogger) Log(_ context.Context, level tracelog.LogLevel, msg string, data map[string]any) {
	l.Printf("%s: %s %+v", level, msg, data)
}
