package repos

import (
	"context"
	"github.com/DataDog/go-sqllexer"
	"github.com/jackc/pgx/v5"
	"log"
	"log/slog"
	"time"
)

type tracer struct{}

var normalizer = sqllexer.NewNormalizer()

type ctxKey int

const (
	_ ctxKey = iota
	traceQueryCtxKey
	traceBatchCtxKey
	traceCopyFromCtxKey
	traceConnectCtxKey
)

type traceQueryData struct {
	startTime time.Time
	sql       string
}

const slowQueryThreshold = 200 * time.Millisecond

func (tl *tracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	sql := data.SQL
	sql, _, err := normalizer.Normalize(sql)
	if err != nil {
		slog.Error("error normalizing SQL: %s", "err", err)
	}
	return context.WithValue(ctx, traceQueryCtxKey, &traceQueryData{
		startTime: time.Now(),
		sql:       sql,
	})
}

func (tl *tracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	queryData := ctx.Value(traceQueryCtxKey).(*traceQueryData)

	endTime := time.Now()
	interval := endTime.Sub(queryData.startTime)

	if data.Err != nil {
		slog.Error("trace error", "err", data.Err, "sql", queryData.sql, "interval", interval)
		return
	}

	if interval > slowQueryThreshold {
		log.Printf("slow query: %s took %s (commandTag %s)", queryData.sql, interval, data.CommandTag.String())
	}
}

type traceBatchData struct {
	startTime time.Time
	sql       map[string]int
}

func (tl *tracer) TraceBatchStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceBatchStartData) context.Context {
	sql := make(map[string]int)
	for _, q := range data.Batch.QueuedQueries {
		s := q.SQL
		s, _, err := normalizer.Normalize(s)
		if err != nil {
			slog.Error("error normalizing SQL", "err", err)
		}

		sql[s] += 1
	}

	return context.WithValue(ctx, traceBatchCtxKey, &traceBatchData{
		startTime: time.Now(),
		sql:       sql,
	})
}

func (tl *tracer) TraceBatchQuery(_ context.Context, _ *pgx.Conn, data pgx.TraceBatchQueryData) {
	if data.Err != nil {
		log.Printf("trace error: BatchQuery: %s: %s", data.Err, data.SQL)
		return
	}
}

func (tl *tracer) TraceBatchEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceBatchEndData) {
	queryData := ctx.Value(traceBatchCtxKey).(*traceBatchData)

	endTime := time.Now()
	interval := endTime.Sub(queryData.startTime)

	if data.Err != nil {
		slog.Error("trace error: BatchClose", "err", data.Err, "interval", interval)
		return
	}

	if interval > slowQueryThreshold {
		slog.Error("slow batch", "sql", queryData.sql, "interval", interval)
	}
}

type traceCopyFromData struct {
	startTime   time.Time
	TableName   pgx.Identifier
	ColumnNames []string
}

func (tl *tracer) TraceCopyFromStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceCopyFromStartData) context.Context {
	return context.WithValue(ctx, traceCopyFromCtxKey, &traceCopyFromData{
		startTime:   time.Now(),
		TableName:   data.TableName,
		ColumnNames: data.ColumnNames,
	})
}

func (tl *tracer) TraceCopyFromEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceCopyFromEndData) {
	copyFromData := ctx.Value(traceCopyFromCtxKey).(*traceCopyFromData)

	endTime := time.Now()
	interval := endTime.Sub(copyFromData.startTime)

	if data.Err != nil {
		log.Printf("trace error: CopyFrom: %s: tableName %s, columnNames %v, time %s", data.Err, copyFromData.TableName, copyFromData.ColumnNames, interval)
		return
	}
}

type traceConnectData struct {
	startTime  time.Time
	connConfig *pgx.ConnConfig
}

func (tl *tracer) TraceConnectStart(ctx context.Context, data pgx.TraceConnectStartData) context.Context {
	return context.WithValue(ctx, traceConnectCtxKey, &traceConnectData{
		startTime:  time.Now(),
		connConfig: data.ConnConfig,
	})
}

func (tl *tracer) TraceConnectEnd(ctx context.Context, data pgx.TraceConnectEndData) {
	connectData := ctx.Value(traceConnectCtxKey).(*traceConnectData)

	endTime := time.Now()
	interval := endTime.Sub(connectData.startTime)

	if data.Err != nil {
		slog.Error("trace error: Connect", "err", data.Err, "host", connectData.connConfig.Host, "port", connectData.connConfig.Port, "database", connectData.connConfig.Database, "interval", interval)
		return
	}
}

func (tl *tracer) TracePrepareStart(ctx context.Context, _ *pgx.Conn, _ pgx.TracePrepareStartData) context.Context {
	return ctx
}

func (tl *tracer) TracePrepareEnd(_ context.Context, _ *pgx.Conn, _ pgx.TracePrepareEndData) {
	return
}
