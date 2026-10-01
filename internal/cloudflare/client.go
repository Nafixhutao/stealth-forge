// Package cloudflare contains the narrow Cloudflare control-plane adapter used
// by browser setup. It deliberately exposes only account, zone, tunnel, DNS,
// and status operations needed by Stealth.
package cloudflare

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	ErrUnauthorized     = errors.New("Cloudflare API token is unauthorized")
	ErrResourceNotFound = errors.New("Cloudflare resource was not found")
	ErrRoutingConflict  = errors.New("Cloudflare workload routing conflicts with provider state")
)

const defaultAPIBaseURL = "https://api.cloudflare.com/client/v4"

const maxListPages = 1000

type Account struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_on,omitempty"`
}

type Zone struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status,omitempty"`
	Type   string `json:"type,omitempty"`
}

// CertificatePack and Certificate are the read-only production certificate
// inventory returned by Cloudflare's certificate-packs API. The reconciler
// keeps only the coverage decision; these provider objects are never stored.
type CertificatePack struct {
	ID           string        `json:"id"`
	Status       string        `json:"status"`
	Type         string        `json:"type"`
	Hosts        []string      `json:"hosts"`
	Certificates []Certificate `json:"certificates"`
}

type Certificate struct {
	ID        string   `json:"id"`
	Status    string   `json:"status"`
	Hosts     []string `json:"hosts"`
	ExpiresOn string   `json:"expires_on,omitempty"`
}

// TotalTLSSettings is informational only. Cloudflare documents that Total
// TLS does not issue certificates for hostnames used with Cloudflare Tunnel,
// so Enabled is never accepted as proof of workload edge coverage.
type TotalTLSSettings struct {
	Enabled *bool `json:"enabled,omitempty"`
}

type Tunnel struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status,omitempty"`
	Token     string `json:"token,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

type DNSRecord struct {
	ID      string `json:"id,omitempty"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Proxied bool   `json:"proxied"`
	TTL     int    `json:"ttl"`
}

type IngressRule struct {
	Hostname string                     `json:"hostname,omitempty"`
	Service  string                     `json:"service"`
	Origin   json.RawMessage            `json:"originRequest,omitempty"`
	Extra    map[string]json.RawMessage `json:"-"`
}

func (rule *IngressRule) UnmarshalJSON(data []byte) error {
	type knownFields struct {
		Hostname string          `json:"hostname,omitempty"`
		Service  string          `json:"service"`
		Origin   json.RawMessage `json:"originRequest,omitempty"`
	}
	var known knownFields
	if err := json.Unmarshal(data, &known); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	delete(fields, "hostname")
	delete(fields, "service")
	delete(fields, "originRequest")
	*rule = IngressRule{Hostname: known.Hostname, Service: known.Service, Origin: known.Origin, Extra: fields}
	return nil
}

func (rule IngressRule) MarshalJSON() ([]byte, error) {
	fields := make(map[string]json.RawMessage, len(rule.Extra)+3)
	if rule.Hostname != "" {
		hostname, err := json.Marshal(rule.Hostname)
		if err != nil {
			return nil, err
		}
		fields["hostname"] = hostname
	}
	service, err := json.Marshal(rule.Service)
	if err != nil {
		return nil, err
	}
	fields["service"] = service
	if len(rule.Origin) != 0 {
		fields["originRequest"] = rule.Origin
	}
	for key, value := range rule.Extra {
		if key == "hostname" || key == "service" || key == "originRequest" {
			continue
		}
		fields[key] = value
	}
	return json.Marshal(fields)
}

type TunnelStatus struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	Status      string `json:"status"`
	Connections int    `json:"connections,omitempty"`
}

// UnmarshalJSON accepts both the legacy integer shape used by older mocks and
// Cloudflare's current tunnel response, where connections is an array that is
// being phased out in favor of the dedicated connections endpoint.
func (s *TunnelStatus) UnmarshalJSON(contents []byte) error {
	var raw struct {
		ID          string          `json:"id"`
		Name        string          `json:"name"`
		Status      string          `json:"status"`
		Connections json.RawMessage `json:"connections"`
	}
	if err := json.Unmarshal(contents, &raw); err != nil {
		return err
	}
	s.ID, s.Name, s.Status, s.Connections = raw.ID, raw.Name, raw.Status, 0
	if len(raw.Connections) == 0 || string(raw.Connections) == "null" {
		return nil
	}
	var count int
	if err := json.Unmarshal(raw.Connections, &count); err == nil {
		s.Connections = count
		return nil
	}
	var connections []json.RawMessage
	if err := json.Unmarshal(raw.Connections, &connections); err != nil {
		return err
	}
	s.Connections = len(connections)
	return nil
}

type Client interface {
	ListAccounts(context.Context) ([]Account, error)
	ListZones(context.Context, string) ([]Zone, error)
	ListCertificatePacks(context.Context, string) ([]CertificatePack, error)
	TotalTLSSettings(context.Context, string) (TotalTLSSettings, error)
	ListTunnels(context.Context, string, string) ([]Tunnel, error)
	CreateTunnel(context.Context, string, string) (Tunnel, error)
	ConfigureTunnel(context.Context, string, string, []IngressRule) error
	TunnelConfiguration(context.Context, string, string) ([]IngressRule, error)
	ListDNSRecords(context.Context, string, string) ([]DNSRecord, error)
	GetDNSRecord(context.Context, string, string) (DNSRecord, error)
	CreateDNSRecord(context.Context, string, DNSRecord) (DNSRecord, error)
	UpdateDNSRecord(context.Context, string, string, DNSRecord) (DNSRecord, error)
	DeleteDNSRecord(context.Context, string, string) error
	TunnelStatus(context.Context, string, string) (TunnelStatus, error)
	TunnelToken(context.Context, string, string) (string, error)
}

type APIClient struct {
	baseURL    string
	apiToken   string
	httpClient *http.Client
}

func NewClient(apiToken, baseURL string, httpClient *http.Client) (*APIClient, error) {
	apiToken = strings.TrimSpace(apiToken)
	if apiToken == "" {
		return nil, errors.New("Cloudflare API token is required")
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = defaultAPIBaseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("Cloudflare API base URL is invalid")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	return &APIClient{baseURL: baseURL, apiToken: apiToken, httpClient: httpClient}, nil
}

func (c *APIClient) ListAccounts(ctx context.Context) ([]Account, error) {
	accounts := make([]Account, 0)
	for page := 1; page <= maxListPages; page++ {
		var result struct {
			Result     []Account `json:"result"`
			ResultInfo pageInfo  `json:"result_info"`
		}
		path := "/accounts?per_page=50&page=" + strconv.Itoa(page)
		if err := c.do(ctx, http.MethodGet, path, nil, &result); err != nil {
			return nil, err
		}
		accounts = append(accounts, result.Result...)
		if pageInfoDone(page, len(result.Result), result.ResultInfo, 50) {
			return accounts, nil
		}
	}
	return nil, errors.New("Cloudflare returned too many account pages")
}

func (c *APIClient) ListZones(ctx context.Context, accountID string) ([]Zone, error) {
	accountID, err := safeID(accountID, "account")
	if err != nil {
		return nil, err
	}
	zones := make([]Zone, 0)
	for page := 1; page <= maxListPages; page++ {
		var result struct {
			Result     []Zone   `json:"result"`
			ResultInfo pageInfo `json:"result_info"`
		}
		path := "/zones?per_page=50&page=" + strconv.Itoa(page) + "&account.id=" + url.QueryEscape(accountID)
		if err := c.do(ctx, http.MethodGet, path, nil, &result); err != nil {
			return nil, err
		}
		zones = append(zones, result.Result...)
		if pageInfoDone(page, len(result.Result), result.ResultInfo, 50) {
			return zones, nil
		}
	}
	return nil, errors.New("Cloudflare returned too many zone pages")
}

// ListCertificatePacks reads production certificate packs including
// non-active states so readiness can distinguish active coverage from a
// matching certificate that Cloudflare is still provisioning.
func (c *APIClient) ListCertificatePacks(ctx context.Context, zoneID string) ([]CertificatePack, error) {
	zoneID, err := safeID(zoneID, "zone")
	if err != nil {
		return nil, err
	}
	packs := make([]CertificatePack, 0)
	for page := 1; page <= maxListPages; page++ {
		query := url.Values{}
		query.Set("deploy", "production")
		query.Set("page", strconv.Itoa(page))
		query.Set("per_page", "50")
		query.Set("status", "all")
		var result struct {
			Result     []CertificatePack `json:"result"`
			ResultInfo pageInfo          `json:"result_info"`
		}
		if err := c.do(ctx, http.MethodGet, "/zones/"+zoneID+"/ssl/certificate_packs?"+query.Encode(), nil, &result); err != nil {
			return nil, err
		}
		for _, pack := range result.Result {
			if strings.TrimSpace(pack.ID) == "" || strings.TrimSpace(pack.Status) == "" || strings.TrimSpace(pack.Type) == "" || len(pack.Hosts) > 50 || len(pack.Certificates) > 50 {
				return nil, errors.New("Cloudflare returned an invalid certificate pack")
			}
			for _, certificate := range pack.Certificates {
				if strings.TrimSpace(certificate.Status) == "" || len(certificate.Hosts) > 50 {
					return nil, errors.New("Cloudflare returned an invalid edge certificate")
				}
			}
		}
		packs = append(packs, result.Result...)
		if pageInfoDone(page, len(result.Result), result.ResultInfo, 50) {
			return packs, nil
		}
	}
	return nil, errors.New("Cloudflare returned too many certificate pack pages")
}

// TotalTLSSettings reads provider capability state without changing it. The
// result is supplemental only; an active certificate matching the required
// wildcard is still required for ready status.
func (c *APIClient) TotalTLSSettings(ctx context.Context, zoneID string) (TotalTLSSettings, error) {
	zoneID, err := safeID(zoneID, "zone")
	if err != nil {
		return TotalTLSSettings{}, err
	}
	var result struct {
		Result TotalTLSSettings `json:"result"`
	}
	if err := c.do(ctx, http.MethodGet, "/zones/"+zoneID+"/acm/total_tls", nil, &result); err != nil {
		return TotalTLSSettings{}, err
	}
	return result.Result, nil
}
