package repository

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/google/uuid"
)

func normalizeAdminAlertRuleInput(input AdminAlertRuleInput) (AdminAlertRuleInput, error) {
	name, err := normalizeAdminControlText(input.Name, 1, 120)
	if err != nil {
		return AdminAlertRuleInput{}, fmt.Errorf("%w: name is invalid", ErrInvalidAdminAlert)
	}
	kind := strings.ToLower(strings.TrimSpace(input.Kind))
	if !validAdminAlertKind(kind) {
		return AdminAlertRuleInput{}, fmt.Errorf("%w: alert kind is unsupported", ErrInvalidAdminAlert)
	}
	severity, err := normalizeAdminSeverity(input.Severity)
	if err != nil {
		return AdminAlertRuleInput{}, fmt.Errorf("%w: severity is invalid", ErrInvalidAdminAlert)
	}
	if input.ForSeconds < 0 || input.ForSeconds > 86400 {
		return AdminAlertRuleInput{}, fmt.Errorf("%w: for_seconds is invalid", ErrInvalidAdminAlert)
	}
	if err := validateAdminAlertCondition(kind, input.Condition); err != nil {
		return AdminAlertRuleInput{}, err
	}
	return AdminAlertRuleInput{Name: name, Kind: kind, Condition: append(json.RawMessage(nil), input.Condition...), Severity: severity, ForSeconds: input.ForSeconds, Enabled: input.Enabled}, nil
}

func normalizeAdminAlertRulePatch(patch AdminAlertRulePatch) (AdminAlertRulePatch, error) {
	if patch.Name == nil && patch.Kind == nil && patch.Condition == nil && patch.Severity == nil && patch.ForSeconds == nil && patch.Enabled == nil {
		return AdminAlertRulePatch{}, fmt.Errorf("%w: at least one field is required", ErrInvalidAdminAlert)
	}
	if patch.Name != nil {
		value, err := normalizeAdminControlText(*patch.Name, 1, 120)
		if err != nil {
			return AdminAlertRulePatch{}, ErrInvalidAdminAlert
		}
		patch.Name = &value
	}
	if patch.Kind != nil {
		value := strings.ToLower(strings.TrimSpace(*patch.Kind))
		if !validAdminAlertKind(value) {
			return AdminAlertRulePatch{}, ErrInvalidAdminAlert
		}
		patch.Kind = &value
	}
	if patch.Condition != nil && len(patch.Condition) == 0 {
		return AdminAlertRulePatch{}, ErrInvalidAdminAlert
	}
	if patch.Severity != nil {
		value, err := normalizeAdminSeverity(*patch.Severity)
		if err != nil {
			return AdminAlertRulePatch{}, ErrInvalidAdminAlert
		}
		patch.Severity = &value
	}
	if patch.ForSeconds != nil && (*patch.ForSeconds < 0 || *patch.ForSeconds > 86400) {
		return AdminAlertRulePatch{}, ErrInvalidAdminAlert
	}
	return patch, nil
}

func validateAdminAlertCondition(kind string, raw json.RawMessage) error {
	if len(raw) == 0 || len(raw) > adminAlertMaxCondition || !json.Valid(raw) {
		return fmt.Errorf("%w: condition must be a bounded JSON object", ErrInvalidAdminAlert)
	}
	var condition map[string]any
	if err := json.Unmarshal(raw, &condition); err != nil || condition == nil {
		return fmt.Errorf("%w: condition must be a JSON object", ErrInvalidAdminAlert)
	}
	if _, forbidden := condition["sql"]; forbidden {
		return fmt.Errorf("%w: arbitrary SQL is not accepted", ErrInvalidAdminAlert)
	}
	switch kind {
	case "metric_threshold", "error_rate", "latency", "log_match", "service_health", "disk_pressure":
		if !conditionHasNumber(condition, "threshold") || !conditionHasString(condition, "operator", "gt", "gte", "lt", "lte") {
			return fmt.Errorf("%w: telemetry alerts require operator and numeric threshold", ErrInvalidAdminAlert)
		}
		if err := validateTelemetryAlertCondition(kind, condition); err != nil {
			return err
		}
	case "monitor_failure", "heartbeat_failure":
		if !conditionHasUUID(condition, "monitor_id") {
			return fmt.Errorf("%w: monitor alerts require monitor_id", ErrInvalidAdminAlert)
		}
	case "certificate_expiry":
		if !conditionHasUUID(condition, "monitor_id") || !positiveCertificateThresholds(condition) {
			return fmt.Errorf("%w: certificate alerts require monitor_id and days", ErrInvalidAdminAlert)
		}
	default:
		return fmt.Errorf("%w: alert kind is unsupported", ErrInvalidAdminAlert)
	}
	return nil
}

func validAdminAlertKind(value string) bool {
	switch value {
	case "metric_threshold", "error_rate", "latency", "log_match", "service_health", "disk_pressure", "monitor_failure", "heartbeat_failure", "certificate_expiry":
		return true
	default:
		return false
	}
}

func validateTelemetryAlertCondition(kind string, condition map[string]any) error {
	if value, ok := condition["window_seconds"]; ok {
		window, valid := value.(float64)
		if !valid || window < 30 || window > 24*60*60 || math.Trunc(window) != window {
			return fmt.Errorf("%w: telemetry alert window_seconds is invalid", ErrInvalidAdminAlert)
		}
	}
	if value, ok := condition["service"]; ok && (!isString(value) || !validAdminControlText(strings.TrimSpace(value.(string)), 1, 128)) {
		return fmt.Errorf("%w: service filter is invalid", ErrInvalidAdminAlert)
	}
	switch kind {
	case "metric_threshold":
		if !conditionHasBoundedString(condition, "metric", 256) {
			return fmt.Errorf("%w: metric threshold alerts require metric", ErrInvalidAdminAlert)
		}
		if value, ok := condition["aggregation"]; ok && (!isString(value) || !conditionHasString(condition, "aggregation", "avg", "min", "max", "sum", "latest")) {
			return fmt.Errorf("%w: metric aggregation is invalid", ErrInvalidAdminAlert)
		}
	case "latency":
		if value, ok := condition["percentile"]; ok && (!isString(value) || !conditionHasString(condition, "percentile", "p50", "p95", "p99")) {
			return fmt.Errorf("%w: latency percentile is invalid", ErrInvalidAdminAlert)
		}
	case "log_match":
		if !conditionHasBoundedString(condition, "search", 256) && !conditionHasBoundedString(condition, "message", 256) {
			return fmt.Errorf("%w: log match alerts require search text", ErrInvalidAdminAlert)
		}
		if value, ok := condition["level"]; ok && (!isString(value) || !validAdminControlText(strings.TrimSpace(value.(string)), 1, 64)) {
			return fmt.Errorf("%w: log level is invalid", ErrInvalidAdminAlert)
		}
	case "service_health":
		if !conditionHasBoundedString(condition, "service", 128) {
			return fmt.Errorf("%w: service health alerts require service", ErrInvalidAdminAlert)
		}
	case "disk_pressure":
		if value, ok := condition["metric"]; ok && (!isString(value) || !validAdminControlText(strings.TrimSpace(value.(string)), 1, 256)) {
			return fmt.Errorf("%w: disk metric is invalid", ErrInvalidAdminAlert)
		}
	}
	return nil
}

func conditionHasNumber(condition map[string]any, key string) bool {
	value, ok := condition[key]
	if !ok {
		return false
	}
	number, ok := value.(float64)
	return ok && !math.IsNaN(number) && !math.IsInf(number, 0)
}

func conditionHasPositiveNumber(condition map[string]any, key string) bool {
	value, ok := condition[key]
	if !ok {
		return false
	}
	number, ok := value.(float64)
	return ok && number > 0 && !math.IsNaN(number) && !math.IsInf(number, 0)
}

func positiveCertificateThresholds(condition map[string]any) bool {
	hasThreshold := false
	for _, key := range []string{"days", "threshold"} {
		if _, exists := condition[key]; !exists {
			continue
		}
		hasThreshold = true
		if !conditionHasPositiveNumber(condition, key) {
			return false
		}
	}
	return hasThreshold
}

// monitorAlertRuleCompatible is the authoritative monitor/rule compatibility
// predicate used by alert writes and monitor kind changes. monitor_failure is
// intentionally compatible with every monitor kind supported by the current
// evaluator; the specialized rules retain their kind-specific contracts.
func monitorAlertRuleCompatible(ruleKind, monitorKind string) bool {
	switch ruleKind {
	case "monitor_failure":
		return monitorKind == "http" || monitorKind == "tcp" || monitorKind == "dns" || monitorKind == "tls" || monitorKind == "heartbeat"
	case "heartbeat_failure":
		return monitorKind == "heartbeat"
	case "certificate_expiry":
		return monitorKind == "tls"
	default:
		return false
	}
}

func validateAdminAlertMonitorCompatibility(ruleKind, monitorKind string) error {
	if monitorAlertRuleCompatible(ruleKind, monitorKind) {
		return nil
	}
	switch ruleKind {
	case "heartbeat_failure":
		return fmt.Errorf("%w: heartbeat_failure requires a heartbeat monitor", ErrInvalidAdminAlert)
	case "certificate_expiry":
		return fmt.Errorf("%w: certificate_expiry requires a TLS monitor", ErrInvalidAdminAlert)
	default:
		return fmt.Errorf("%w: monitor alert rule is incompatible with the monitor kind", ErrInvalidAdminAlert)
	}
}

func conditionHasString(condition map[string]any, key string, values ...string) bool {
	value, ok := condition[key].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return false
	}
	for _, allowed := range values {
		if value == allowed {
			return true
		}
	}
	return false
}

func conditionHasUUID(condition map[string]any, key string) bool {
	value, ok := condition[key].(string)
	if !ok {
		return false
	}
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	return err == nil && parsed != uuid.Nil
}

func conditionHasBoundedString(condition map[string]any, key string, maximum int) bool {
	value, ok := condition[key].(string)
	return ok && validAdminControlText(value, 1, maximum)
}

func isString(value any) bool {
	_, ok := value.(string)
	return ok
}

func normalizeAdminSeverity(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value != "info" && value != "warning" && value != "critical" {
		return "", errors.New("invalid severity")
	}
	return value, nil
}
