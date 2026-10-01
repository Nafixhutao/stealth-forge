package telemetry

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

func (s *ClickHouseStore) validate(queryRange TimeRange, limit int) error {
	if s == nil || s.conn == nil {
		return ErrDisabled
	}
	if err := queryRange.validate(s.maxQueryRange); err != nil {
		return err
	}
	if limit < 1 || limit > s.maxQueryRows {
		return fmt.Errorf("%w: limit must be between 1 and %d", ErrInvalidQuery, s.maxQueryRows)
	}
	return nil
}

func clickHouseLimit(value int) (uint32, error) {
	if value < 0 {
		return 0, fmt.Errorf("%w: result limit cannot fit ClickHouse UInt32", ErrInvalidQuery)
	}
	parsed, err := strconv.ParseUint(strconv.FormatInt(int64(value), 10), 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%w: result limit cannot fit ClickHouse UInt32", ErrInvalidQuery)
	}
	return uint32(parsed), nil
}

func (s *ClickHouseStore) query(ctx context.Context, query string, args ...any) (driver.Rows, error) {
	queryContext, cancel := context.WithTimeout(ctx, s.maxQueryDuration)
	queryContext = clickhouse.Context(queryContext, clickhouse.WithSettings(clickhouse.Settings{
		"max_execution_time":   uint64(s.maxQueryDuration / time.Second),
		"max_result_rows":      uint64(s.maxQueryRows),
		"max_result_bytes":     uint64(16 << 20),
		"result_overflow_mode": "throw",
	}))
	rows, err := s.conn.Query(queryContext, query, args...)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("query telemetry backend: %w", err)
	}
	return &cancellableRows{Rows: rows, cancel: cancel}, nil
}

// cancellableRows keeps the query deadline alive while ClickHouse streams
// rows. Cancelling the context in query() immediately after Query returned
// would make real ClickHouse result sets fail with context.Canceled before
// their first row was read. Every domain query defers Rows.Close, which
// releases the timer and context once scanning is complete.
type cancellableRows struct {
	driver.Rows
	cancel context.CancelFunc
}

func (r *cancellableRows) Close() error {
	err := r.Rows.Close()
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
	return err
}

func boundedFilter(value string, maximum int) string {
	value = strings.TrimSpace(value)
	if len(value) > maximum {
		return value[:maximum]
	}
	return value
}

var sensitiveKeyPattern = regexp.MustCompile(`(?i)(^|[._-])(password|passwd|pwd|secret|token|api[_-]?key|apikey|authorization|cookie|set-cookie|private[_-]?key|client[_-]?secret|access[_-]?token|refresh[_-]?token)([._-]|$)`)
var sensitiveTextPattern = regexp.MustCompile(`(?i)((?:bearer|basic)\s+|(?:password|passwd|pwd|secret|token|api[_-]?key|apikey|authorization|cookie|set-cookie|private[_-]?key|client[_-]?secret|access[_-]?token|refresh[_-]?token)['"]?\s*[:=]\s*(?:(?:bearer|basic)\s+)?)(["']?)[^\s,;{}"']+`)
var sensitiveURLPattern = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://[^:/\s]+:)[^@/\s]+`)

func redactAttributes(attributes map[string]string) map[string]string {
	if len(attributes) == 0 {
		return nil
	}
	redacted := make(map[string]string, len(attributes))
	for key, value := range attributes {
		if sensitiveKeyPattern.MatchString(key) {
			redacted[key] = "[REDACTED]"
			continue
		}
		redacted[key] = redactText(value)
	}
	return redacted
}

func redactText(value string) string {
	if value == "" {
		return value
	}
	value = sensitiveTextPattern.ReplaceAllString(value, `$1$2[REDACTED]`)
	return sensitiveURLPattern.ReplaceAllString(value, `$1[REDACTED]`)
}
