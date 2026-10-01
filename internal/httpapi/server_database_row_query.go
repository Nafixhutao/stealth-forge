package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	dbcore "github.com/Stealth-deplover/stealth/internal/database"
	"github.com/Stealth-deplover/stealth/internal/repository"
)

func decodeRowObject(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, errors.New("data must be a JSON object")
	}
	var data map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&data); err != nil || data == nil {
		return nil, errors.New("data must be a JSON object")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("data must contain one JSON value")
	}
	return data, nil
}

func parseRowQuery(r *http.Request, schema repository.DatabaseTableSchema) (repository.RowQuery, error) {
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 100 {
			return repository.RowQuery{}, fmt.Errorf("%w: limit must be between 1 and 100", repository.ErrInvalidQuery)
		}
		limit = value
	}
	var cursor *repository.RowCursor
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		value, err := repository.DecodeRowCursor(raw)
		if err != nil {
			return repository.RowQuery{}, err
		}
		cursor = &value
	}
	byKey := make(map[string]repository.DatabaseColumnSchema, len(schema.Columns))
	for _, column := range schema.Columns {
		byKey[column.Key] = column
	}
	filters := make([]repository.RowFilter, 0)
	query := r.URL.Query()
	for key, values := range query {
		if !strings.HasPrefix(key, "filter") || key == "filter" {
			continue
		}
		if len(values) != 1 {
			return repository.RowQuery{}, fmt.Errorf("%w: filter must occur once", repository.ErrInvalidQuery)
		}
		columnKey, ok := filterColumnKey(key)
		if !ok {
			return repository.RowQuery{}, fmt.Errorf("%w: filter name is invalid", repository.ErrInvalidQuery)
		}
		column, ok := byKey[columnKey]
		if !ok {
			return repository.RowQuery{}, fmt.Errorf("%w: filter column is not declared", repository.ErrInvalidQuery)
		}
		value, err := dbcore.ParseQueryValue(dbcore.ColumnDefinition{Key: column.Key, Type: column.Type, VarcharSize: column.VarcharSize}, values[0])
		if err != nil {
			return repository.RowQuery{}, err
		}
		filters = append(filters, repository.RowFilter{Column: column, Value: value})
	}
	if raw := query.Get("filter"); raw != "" {
		var object map[string]string
		decoder := json.NewDecoder(strings.NewReader(raw))
		if err := decoder.Decode(&object); err != nil {
			return repository.RowQuery{}, fmt.Errorf("%w: filter must be an object", repository.ErrInvalidQuery)
		}
		for key, value := range object {
			column, ok := byKey[key]
			if !ok {
				return repository.RowQuery{}, fmt.Errorf("%w: filter column is not declared", repository.ErrInvalidQuery)
			}
			parsed, err := dbcore.ParseQueryValue(dbcore.ColumnDefinition{Key: column.Key, Type: column.Type, VarcharSize: column.VarcharSize}, value)
			if err != nil {
				return repository.RowQuery{}, err
			}
			filters = append(filters, repository.RowFilter{Column: column, Value: parsed})
		}
	}
	var orderBy *repository.DatabaseColumnSchema
	if raw := query.Get("order_by"); raw != "" && raw != "id" {
		column, ok := byKey[raw]
		if !ok {
			return repository.RowQuery{}, fmt.Errorf("%w: order column is not declared", repository.ErrInvalidQuery)
		}
		if !column.Required {
			return repository.RowQuery{}, fmt.Errorf("%w: order_by must target a required column so cursor ordering is stable", repository.ErrInvalidQuery)
		}
		orderBy = &column
	}
	descending := false
	if raw := query.Get("order_direction"); raw != "" {
		switch strings.ToLower(raw) {
		case "asc":
		case "desc":
			descending = true
		default:
			return repository.RowQuery{}, fmt.Errorf("%w: order_direction must be asc or desc", repository.ErrInvalidQuery)
		}
	}
	searchRaw := query.Get("search")
	search := strings.TrimSpace(searchRaw)
	searchColumnKey := strings.TrimSpace(query.Get("search_column"))
	var searchColumn *repository.DatabaseColumnSchema
	if searchRaw != "" && search == "" {
		return repository.RowQuery{}, fmt.Errorf("%w: search must not be empty", repository.ErrInvalidQuery)
	}
	if search != "" && searchColumnKey == "" {
		return repository.RowQuery{}, fmt.Errorf("%w: search_column is required with search", repository.ErrInvalidQuery)
	}
	if searchColumnKey != "" {
		column, ok := byKey[searchColumnKey]
		if !ok {
			return repository.RowQuery{}, fmt.Errorf("%w: search column is not declared", repository.ErrInvalidQuery)
		}
		if column.Type != dbcore.TypeVarchar && column.Type != dbcore.TypeText {
			return repository.RowQuery{}, fmt.Errorf("%w: full-text search requires a varchar or text column", repository.ErrInvalidQuery)
		}
		if search == "" {
			return repository.RowQuery{}, fmt.Errorf("%w: search is required with search_column", repository.ErrInvalidQuery)
		}
		if len(search) > 256 {
			return repository.RowQuery{}, fmt.Errorf("%w: search must be at most 256 bytes", repository.ErrInvalidQuery)
		}
		searchColumn = &column
	}
	if cursor != nil && orderBy != nil {
		value, err := canonicalCursorValue(*orderBy, cursor.Value)
		if err != nil {
			return repository.RowQuery{}, err
		}
		cursor.Value = value
	}
	return repository.RowQuery{Limit: limit, Cursor: cursor, Filters: filters, OrderBy: orderBy, Descending: descending, Search: search, SearchColumn: searchColumn}, nil
}

func canonicalCursorValue(column repository.DatabaseColumnSchema, value any) (any, error) {
	if value == nil {
		return nil, fmt.Errorf("%w: ordered cursor cannot contain a null value", repository.ErrInvalidQuery)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("%w: cursor value is invalid", repository.ErrInvalidQuery)
	}
	var text string
	switch column.Type {
	case dbcore.TypeJSON:
		var decoded any
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&decoded); err != nil {
			return nil, fmt.Errorf("%w: cursor value is invalid", repository.ErrInvalidQuery)
		}
		return decoded, nil
	case dbcore.TypeBoolean, dbcore.TypeInteger, dbcore.TypeDouble:
		var ok bool
		text, ok = cursorScalarText(value)
		if !ok {
			return nil, fmt.Errorf("%w: cursor value is invalid", repository.ErrInvalidQuery)
		}
	case dbcore.TypeVarchar, dbcore.TypeText, dbcore.TypeDatetime:
		var ok bool
		text, ok = value.(string)
		if !ok {
			return nil, fmt.Errorf("%w: cursor value is invalid", repository.ErrInvalidQuery)
		}
	default:
		return nil, fmt.Errorf("%w: cursor value is invalid", repository.ErrInvalidQuery)
	}
	value, err = dbcore.ParseQueryValue(dbcore.ColumnDefinition{Key: column.Key, Type: column.Type, VarcharSize: column.VarcharSize}, text)
	if err != nil {
		return nil, fmt.Errorf("%w: cursor value is invalid", repository.ErrInvalidQuery)
	}
	return value, nil
}

func cursorScalarText(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, true
	case json.Number:
		return string(typed), true
	case bool:
		return strconv.FormatBool(typed), true
	case int:
		return strconv.Itoa(typed), true
	case int8:
		return strconv.FormatInt(int64(typed), 10), true
	case int16:
		return strconv.FormatInt(int64(typed), 10), true
	case int32:
		return strconv.FormatInt(int64(typed), 10), true
	case int64:
		return strconv.FormatInt(typed, 10), true
	case uint:
		return strconv.FormatUint(uint64(typed), 10), true
	case uint8:
		return strconv.FormatUint(uint64(typed), 10), true
	case uint16:
		return strconv.FormatUint(uint64(typed), 10), true
	case uint32:
		return strconv.FormatUint(uint64(typed), 10), true
	case uint64:
		return strconv.FormatUint(typed, 10), true
	case float32:
		return strconv.FormatFloat(float64(typed), 'g', -1, 32), true
	case float64:
		return strconv.FormatFloat(typed, 'g', -1, 64), true
	default:
		return "", false
	}
}

func filterColumnKey(value string) (string, bool) {
	if strings.HasPrefix(value, "filter.") {
		key := strings.TrimPrefix(value, "filter.")
		return key, key != ""
	}
	if strings.HasPrefix(value, "filter_") {
		key := strings.TrimPrefix(value, "filter_")
		return key, key != ""
	}
	if strings.HasPrefix(value, "filter[") && strings.HasSuffix(value, "]") {
		key := strings.TrimSuffix(strings.TrimPrefix(value, "filter["), "]")
		return key, key != ""
	}
	return "", false
}

func writeDatabaseQueryError(w http.ResponseWriter, err error) {
	if errors.Is(err, repository.ErrUnindexedQuery) {
		writeError(w, 422, "unindexed_query", "filters and ordering require a real key index on the declared column")
		return
	}
	if errors.Is(err, repository.ErrInvalidQuery) || errors.Is(err, dbcore.ErrInvalidValue) {
		writeError(w, 422, "validation_error", err.Error())
		return
	}
	writeError(w, 422, "validation_error", "invalid database query")
}
