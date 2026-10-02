package monitoring

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Stealth-deplover/stealth/internal/repository"
)

func checkHTTP(ctx context.Context, job repository.AdminMonitorJob, config monitorConfig, details *json.RawMessage) (*int, error) {
	method := strings.ToUpper(strings.TrimSpace(config.Method))
	if method == "" {
		method = http.MethodGet
	}
	request, err := http.NewRequestWithContext(ctx, method, job.Target, strings.NewReader(config.Body))
	if err != nil {
		return nil, errors.New("HTTP request could not be created")
	}
	for key, value := range config.Headers {
		request.Header.Set(key, value)
	}
	client := &http.Client{
		Timeout: time.Duration(job.TimeoutMS) * time.Millisecond,
		Transport: &http.Transport{
			Proxy:                 nil,
			DialContext:           safeDialContext,
			TLSHandshakeTimeout:   time.Duration(job.TimeoutMS) * time.Millisecond,
			ResponseHeaderTimeout: time.Duration(job.TimeoutMS) * time.Millisecond,
			DisableCompression:    false,
			MaxIdleConnsPerHost:   2,
		},
	}
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) > maxRedirects {
			return errors.New("too many redirects")
		}
		if err := validatePublicURL(next.Context(), next.URL); err != nil {
			return err
		}
		return nil
	}
	if err := validatePublicURL(ctx, request.URL); err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, classifyNetworkError(err)
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(response.Body, maxHTTPResponseBytes+1))
	if readErr != nil {
		return &response.StatusCode, errors.New("HTTP response could not be read")
	}
	if len(body) > maxHTTPResponseBytes {
		return &response.StatusCode, errors.New("HTTP response exceeded the monitor limit")
	}
	expectedStatus := config.ExpectedStatus
	if expectedStatus == 0 {
		expectedStatus = http.StatusOK
	}
	if response.StatusCode != expectedStatus {
		return &response.StatusCode, fmt.Errorf("HTTP endpoint returned status %d", response.StatusCode)
	}
	if config.BodyContains != "" && !strings.Contains(string(body), config.BodyContains) {
		return &response.StatusCode, errors.New("HTTP body assertion did not match")
	}
	encoded, _ := json.Marshal(map[string]any{"status_code": response.StatusCode, "content_length": len(body)})
	*details = encoded
	return &response.StatusCode, nil
}

func checkTCP(ctx context.Context, job repository.AdminMonitorJob, config monitorConfig, details *json.RawMessage) error {
	host, port, _ := hostPort(job.Target, config)
	connection, err := safeDialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return classifyNetworkError(err)
	}
	_ = connection.Close()
	encoded, _ := json.Marshal(map[string]any{"host": host, "port": port})
	*details = encoded
	return nil
}

func checkDNS(ctx context.Context, job repository.AdminMonitorJob, config monitorConfig, details *json.RawMessage) error {
	recordType := strings.ToUpper(strings.TrimSpace(config.RecordType))
	if recordType == "" {
		recordType = "A"
	}
	var values []string
	var err error
	resolver := net.DefaultResolver
	switch recordType {
	case "A", "AAAA":
		// net.Resolver.LookupIP only accepts "ip", "ip4" or "ip6"; passing
		// "a"/"aaaa" fails with UnknownNetworkError on every lookup.
		network := "ip4"
		if recordType == "AAAA" {
			network = "ip6"
		}
		ips, lookupErr := resolver.LookupIP(ctx, network, job.Target)
		err = lookupErr
		for _, ip := range ips {
			if (recordType == "A" && ip.To4() != nil) || (recordType == "AAAA" && ip.To4() == nil) {
				values = append(values, ip.String())
			}
		}
	case "CNAME":
		var value string
		value, err = resolver.LookupCNAME(ctx, job.Target)
		if value != "" {
			values = []string{strings.TrimSuffix(value, ".")}
		}
	case "TXT":
		values, err = resolver.LookupTXT(ctx, job.Target)
	}
	if err != nil {
		return errors.New("DNS lookup failed")
	}
	if len(config.ExpectedValues) > 0 && !expectedDNSValuesPresent(values, config.ExpectedValues) {
		return errors.New("DNS values did not match")
	}
	encoded, _ := json.Marshal(map[string]any{"record_type": recordType, "record_count": len(values)})
	*details = encoded
	return nil
}

func checkTLS(ctx context.Context, job repository.AdminMonitorJob, config monitorConfig, details *json.RawMessage) error {
	host, port, _ := hostPort(job.Target, config)
	connection, err := safeDialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return classifyNetworkError(err)
	}
	tlsConnection := tls.Client(connection, &tls.Config{MinVersion: tls.VersionTLS12, ServerName: host})
	defer tlsConnection.Close()
	if err := tlsConnection.HandshakeContext(ctx); err != nil {
		return classifyNetworkError(err)
	}
	state := tlsConnection.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return errors.New("TLS endpoint did not present a certificate")
	}
	certificate := state.PeerCertificates[0]
	now := time.Now()
	if now.Before(certificate.NotBefore) || now.After(certificate.NotAfter) {
		return errors.New("TLS certificate is not currently valid")
	}
	days := int(time.Until(certificate.NotAfter).Hours() / 24)
	encoded, _ := json.Marshal(map[string]any{"expires_at": certificate.NotAfter.UTC(), "issuer": certificate.Issuer.String(), "days_remaining": days})
	*details = encoded
	if config.CertificateExpiryDays > 0 && days < config.CertificateExpiryDays {
		return errors.New("TLS certificate is approaching expiry")
	}
	return nil
}

func checkHeartbeat(now time.Time, job repository.AdminMonitorJob, config monitorConfig, details *json.RawMessage) error {
	if job.LastHeartbeatAt == nil {
		return errors.New("heartbeat has not been received")
	}
	age := now.Sub(job.LastHeartbeatAt.UTC())
	grace := time.Duration(config.GraceSeconds) * time.Second
	if age > time.Duration(job.IntervalSeconds)*time.Second+grace {
		encoded, _ := json.Marshal(map[string]any{"last_heartbeat_at": job.LastHeartbeatAt.UTC(), "age_seconds": int64(age.Seconds())})
		*details = encoded
		return errors.New("heartbeat is late")
	}
	encoded, _ := json.Marshal(map[string]any{"last_heartbeat_at": job.LastHeartbeatAt.UTC(), "age_seconds": int64(age.Seconds())})
	*details = encoded
	return nil
}

func hostPort(target string, config monitorConfig) (string, int, error) {
	host := strings.TrimSpace(config.Host)
	port := config.Port
	if host == "" {
		var err error
		var portString string
		host, portString, err = net.SplitHostPort(target)
		if err != nil {
			return "", 0, err
		}
		port, err = strconv.Atoi(portString)
		if err != nil {
			return "", 0, err
		}
	}
	return strings.Trim(host, "[]"), port, nil
}
