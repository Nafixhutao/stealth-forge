// Package monitoring owns the trusted execution side of instance monitors.
// Definitions are persisted by the API, but network probes run only in the
// worker process and return bounded protocol facts to the control plane.
package monitoring

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Stealth-deplover/stealth/internal/functionsecret"
	"github.com/Stealth-deplover/stealth/internal/repository"
)

const (
	maxHTTPResponseBytes = 1 << 20
	maxRedirects         = 5
	maxMonitorError      = 240
)

type monitorConfig struct {
	Method                string            `json:"method,omitempty"`
	Headers               map[string]string `json:"headers,omitempty"`
	Body                  string            `json:"body,omitempty"`
	ExpectedStatus        int               `json:"expected_status,omitempty"`
	BodyContains          string            `json:"body_contains,omitempty"`
	LatencyThresholdMS    int               `json:"latency_threshold_ms,omitempty"`
	Host                  string            `json:"host,omitempty"`
	Port                  int               `json:"port,omitempty"`
	RecordType            string            `json:"record_type,omitempty"`
	ExpectedValues        []string          `json:"expected_values,omitempty"`
	GraceSeconds          int               `json:"grace_seconds,omitempty"`
	CertificateExpiryDays int               `json:"certificate_expiry_days,omitempty"`
	TokenHash             string            `json:"token_hash,omitempty"`
}

// ValidateConfig validates the non-secret monitor definition before it is
// encrypted and stored. The worker repeats the checks after decrypting the
// config so a corrupt or old row cannot become an unrestricted network probe.
func ValidateConfig(kind, target string, config []byte) error {
	var value monitorConfig
	if len(config) == 0 || !json.Valid(config) || json.Unmarshal(config, &value) != nil {
		return errors.New("monitor configuration is invalid")
	}
	return validate(kind, target, value)
}

func Check(ctx context.Context, job repository.AdminMonitorJob, cipher *functionsecret.Cipher) repository.AdminMonitorCheckInput {
	started := time.Now()
	result := repository.AdminMonitorCheckInput{Details: json.RawMessage(`{}`)}
	if cipher == nil {
		result.Error = "monitor secret decryption is unavailable"
		result.LatencyMS = time.Since(started).Milliseconds()
		return result
	}
	plaintext, err := cipher.Decrypt(job.EncryptedConfig)
	if err != nil {
		result.Error = "monitor configuration could not be decrypted"
		result.LatencyMS = time.Since(started).Milliseconds()
		return result
	}
	var config monitorConfig
	if err := json.Unmarshal(plaintext, &config); err != nil || validate(job.Kind, job.Target, config) != nil {
		result.Error = "monitor configuration is invalid"
		result.LatencyMS = time.Since(started).Milliseconds()
		return result
	}

	probeContext, cancel := context.WithTimeout(ctx, time.Duration(job.TimeoutMS)*time.Millisecond)
	defer cancel()
	var checkErr error
	switch job.Kind {
	case "http":
		result.StatusCode, checkErr = checkHTTP(probeContext, job, config, &result.Details)
	case "tcp":
		checkErr = checkTCP(probeContext, job, config, &result.Details)
	case "dns":
		checkErr = checkDNS(probeContext, job, config, &result.Details)
	case "tls":
		checkErr = checkTLS(probeContext, job, config, &result.Details)
	case "heartbeat":
		checkErr = checkHeartbeat(time.Now().UTC(), job, config, &result.Details)
	case "synthetic":
		checkErr = errors.New("synthetic runner is not enabled")
	default:
		checkErr = errors.New("monitor type is unsupported")
	}
	result.LatencyMS = time.Since(started).Milliseconds()
	if result.LatencyMS < 0 {
		result.LatencyMS = 0
	}
	if checkErr == nil {
		result.Success = true
		if config.LatencyThresholdMS > 0 && result.LatencyMS > int64(config.LatencyThresholdMS) {
			result.Success = false
			result.Error = "probe exceeded the latency threshold"
		}
	} else {
		result.Error = safeCheckError(checkErr)
	}
	return result
}

func validate(kind, target string, config monitorConfig) error {
	target = strings.TrimSpace(target)
	if target == "" || strings.ContainsAny(target, "\x00\r\n") {
		return errors.New("target is invalid")
	}
	if config.LatencyThresholdMS < 0 || config.LatencyThresholdMS > 120000 || config.GraceSeconds < 0 || config.GraceSeconds > 7*86400 || config.CertificateExpiryDays < 0 || config.CertificateExpiryDays > 3650 {
		return errors.New("threshold is invalid")
	}
	switch kind {
	case "http":
		parsed, err := url.Parse(target)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
			return errors.New("HTTP target must be an absolute HTTP(S) URL")
		}
		method := strings.ToUpper(strings.TrimSpace(config.Method))
		if method == "" {
			method = http.MethodGet
		}
		if !validHTTPMethod(method) {
			return errors.New("HTTP method is invalid")
		}
		if config.ExpectedStatus < 0 || config.ExpectedStatus > 599 {
			return errors.New("expected HTTP status is invalid")
		}
		if len(config.Headers) > 32 || len(config.Body) > 1<<20 || len(config.BodyContains) > 512 {
			return errors.New("HTTP assertion or request is too large")
		}
		for key, value := range config.Headers {
			if !validHeader(key, value) {
				return errors.New("HTTP header is invalid")
			}
		}
	case "tcp", "tls":
		host, port, err := hostPort(target, config)
		if err != nil || host == "" || port < 1 || port > 65535 {
			return errors.New("network target is invalid")
		}
	case "dns":
		if !validDNSName(target) {
			return errors.New("DNS target is invalid")
		}
		recordType := strings.ToUpper(strings.TrimSpace(config.RecordType))
		if recordType == "" {
			recordType = "A"
		}
		if recordType != "A" && recordType != "AAAA" && recordType != "CNAME" && recordType != "TXT" {
			return errors.New("DNS record type is unsupported")
		}
		if len(config.ExpectedValues) > 32 {
			return errors.New("too many expected DNS values")
		}
	case "heartbeat":
		if len(config.TokenHash) != sha256.Size*2 || !isHex(config.TokenHash) {
			return errors.New("heartbeat token hash is invalid")
		}
	case "synthetic":
		return errors.New("synthetic runner is not enabled")
	default:
		return errors.New("monitor type is unsupported")
	}
	return nil
}
