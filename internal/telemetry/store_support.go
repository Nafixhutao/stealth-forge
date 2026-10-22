package telemetry

import "strings"

// boundedFilter trims and caps a caller-supplied filter value before it is
// bound as a query parameter. It bounds work, not authorization: the value is
// still passed as a parameter, never interpolated into SQL.
func boundedFilter(value string, maximum int) string {
	value = strings.TrimSpace(value)
	if len(value) > maximum {
		return value[:maximum]
	}
	return value
}
