package metricstracer

import (
	"context"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"
)

type (
	queryStartKey    struct{}
	queryContextData struct {
		operation string
		table     string
		startTime time.Time
	}

	metrics interface {
		DatabaseQueryDuration() *prometheus.HistogramVec
		DatabaseQueryTotal() *prometheus.CounterVec
	}

	// MetricsTracer is a pgx.Trace
	// https://pkg.go.dev/github.com/jackc/pgx/v5#Trace

	MetricsTracer struct {
		metrics metrics
	}
)

func New(metrics metrics) *MetricsTracer {
	return &MetricsTracer{
		metrics: metrics,
	}
}

func (m *MetricsTracer) TraceQueryStart(
	ctx context.Context,
	conn *pgx.Conn,
	data pgx.TraceQueryStartData,
) context.Context {
	operation, table := parseSQL(data.SQL)

	queryData := &queryContextData{
		operation: operation,
		table:     table,
		startTime: time.Now(),
	}

	return context.WithValue(ctx, queryStartKey{}, queryData)
}

func (m *MetricsTracer) TraceQueryEnd(
	ctx context.Context,
	conn *pgx.Conn,
	data pgx.TraceQueryEndData,
) {
	queryData, ok := ctx.Value(queryStartKey{}).(*queryContextData)
	if !ok {
		return
	}

	duration := time.Since(queryData.startTime).Milliseconds()

	status := "success"
	if data.Err != nil {
		status = "error"
	}

	m.metrics.DatabaseQueryDuration().WithLabelValues(queryData.operation, status, queryData.table).Observe(float64(duration))
	m.metrics.DatabaseQueryTotal().WithLabelValues(queryData.operation, status, queryData.table).Inc()
}

func parseSQL(sql string) (string, string) {
	var (
		operation = "unknown"
		table     = "unknown"
	)

	switch {
	case strings.HasPrefix(strings.ToUpper(sql), "SELECT"):
		operation = "select"
		table = extractTableNameFormSelect(sql)
	case strings.HasPrefix(strings.ToUpper(sql), "INSERT"):
		operation = "insert"
		table = extractTableNameFromInsert(sql)
	case strings.HasPrefix(strings.ToUpper(sql), "UPDATE"):
		operation = "update"
		table = extractTableNameFromUpdate(sql)
	case strings.HasPrefix(strings.ToUpper(sql), "DELETE"):
		operation = "delete"
		table = extractTableNameFromDelete(sql)
	case strings.HasPrefix(sql, "BEGIN"):
		operation = "begin"
		table = "transaction"
	case strings.HasPrefix(sql, "COMMIT"):
		operation = "commit"
		table = "transaction"
	case strings.HasPrefix(sql, "ROLLBACK"):
		operation = "rollback"
		table = "transaction"
	}

	return operation, table
}

// extractTableNameFromDelete extract table name from delete sql
// example: "DELETE FROM table_name"
func extractTableNameFromDelete(sql string) string {
	parts := strings.Fields(sql)
	for i, part := range parts {
		if strings.ToLower(part) == "from" && i+1 < len(parts) {
			return cleanTableName(parts[i+1])
		}
	}

	return "unknown"
}

// extractTableNameFromInsert extract table name from insert sql
// example: "INSERT INTO table_name"
func extractTableNameFromInsert(sql string) string {
	parts := strings.Fields(sql)
	for i, part := range parts {
		if strings.ToLower(part) == "into" && i+1 < len(parts) {
			return cleanTableName(parts[i+1])
		}
	}

	return "unknown"
}

// extractTableNameFromUpdate extract table name from update sql
// example: "UPDATE table_name"
func extractTableNameFromUpdate(sql string) string {
	parts := strings.Fields(sql)

	if len(parts) >= 2 {
		return cleanTableName(parts[1])
	}

	return "unknown"
}

// extractTableNameFormSelect extract table name from select sql
// example: "SELECT * FROM table_name"
func extractTableNameFormSelect(sql string) string {
	parts := strings.Fields(sql)
	for i, part := range parts {
		if strings.ToLower(part) == "from" && i+1 < len(parts) {
			return cleanTableName(parts[i+1])
		}
	}

	return "unknown"
}

// cleanTableName clean table name
func cleanTableName(name string) string {
	name = strings.TrimFunc(name, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r) && r != '_'
	})

	return strings.ToLower(name)
}
