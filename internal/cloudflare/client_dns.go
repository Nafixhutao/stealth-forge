package cloudflare

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func (c *APIClient) CreateDNSRecord(ctx context.Context, zoneID string, record DNSRecord) (DNSRecord, error) {
	zoneID, err := safeID(zoneID, "zone")
	if err != nil {
		return DNSRecord{}, err
	}
	record, err = normalizeDNSRecord(record)
	if err != nil {
		return DNSRecord{}, err
	}
	var result struct {
		Result DNSRecord `json:"result"`
	}
	if err := c.do(ctx, http.MethodPost, "/zones/"+zoneID+"/dns_records", record, &result); err != nil {
		return DNSRecord{}, err
	}
	if strings.TrimSpace(result.Result.ID) == "" {
		return DNSRecord{}, errors.New("Cloudflare returned an invalid DNS record")
	}
	return result.Result, nil
}

func (c *APIClient) ListDNSRecords(ctx context.Context, zoneID, name string) ([]DNSRecord, error) {
	zoneID, err := safeID(zoneID, "zone")
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 253 || strings.ContainsAny(name, "\x00\r\n") {
		return nil, errors.New("DNS record name is invalid")
	}
	records := make([]DNSRecord, 0)
	for page := 1; page <= maxListPages; page++ {
		values := url.Values{}
		values.Set("name", name)
		values.Set("per_page", "100")
		values.Set("page", strconv.Itoa(page))
		var result struct {
			Result     []DNSRecord `json:"result"`
			ResultInfo pageInfo    `json:"result_info"`
		}
		if err := c.do(ctx, http.MethodGet, "/zones/"+zoneID+"/dns_records?"+values.Encode(), nil, &result); err != nil {
			return nil, err
		}
		records = append(records, result.Result...)
		if pageInfoDone(page, len(result.Result), result.ResultInfo, 100) {
			return records, nil
		}
	}
	return nil, errors.New("Cloudflare returned too many DNS record pages")
}

func (c *APIClient) GetDNSRecord(ctx context.Context, zoneID, recordID string) (DNSRecord, error) {
	zoneID, err := safeID(zoneID, "zone")
	if err != nil {
		return DNSRecord{}, err
	}
	recordID, err = safeID(recordID, "DNS record")
	if err != nil {
		return DNSRecord{}, err
	}
	var result struct {
		Result DNSRecord `json:"result"`
	}
	if err := c.do(ctx, http.MethodGet, "/zones/"+zoneID+"/dns_records/"+recordID, nil, &result); err != nil {
		return DNSRecord{}, err
	}
	if strings.TrimSpace(result.Result.ID) == "" {
		return DNSRecord{}, errors.New("Cloudflare returned an invalid DNS record")
	}
	return result.Result, nil
}

func (c *APIClient) UpdateDNSRecord(ctx context.Context, zoneID, recordID string, record DNSRecord) (DNSRecord, error) {
	zoneID, err := safeID(zoneID, "zone")
	if err != nil {
		return DNSRecord{}, err
	}
	recordID, err = safeID(recordID, "DNS record")
	if err != nil {
		return DNSRecord{}, err
	}
	record, err = normalizeDNSRecord(record)
	if err != nil {
		return DNSRecord{}, err
	}
	var result struct {
		Result DNSRecord `json:"result"`
	}
	if err := c.do(ctx, http.MethodPut, "/zones/"+zoneID+"/dns_records/"+recordID, record, &result); err != nil {
		return DNSRecord{}, err
	}
	if strings.TrimSpace(result.Result.ID) == "" {
		result.Result.ID = recordID
	}
	return result.Result, nil
}

func (c *APIClient) DeleteDNSRecord(ctx context.Context, zoneID, recordID string) error {
	zoneID, err := safeID(zoneID, "zone")
	if err != nil {
		return err
	}
	recordID, err = safeID(recordID, "DNS record")
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodDelete, "/zones/"+zoneID+"/dns_records/"+recordID, nil, nil)
}

func normalizeDNSRecord(record DNSRecord) (DNSRecord, error) {
	record.Type = strings.ToUpper(strings.TrimSpace(record.Type))
	record.Name = strings.TrimSpace(record.Name)
	record.Content = strings.TrimSpace(record.Content)
	if record.Type != "CNAME" || record.Name == "" || record.Content == "" || len(record.Name) > 253 || len(record.Content) > 253 || strings.ContainsAny(record.Name+record.Content, "\x00\r\n") {
		return DNSRecord{}, errors.New("DNS record is invalid")
	}
	if record.TTL == 0 {
		record.TTL = 1
	}
	return record, nil
}
