package cloudflare

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type pageInfo struct {
	TotalPages int `json:"total_pages"`
	TotalCount int `json:"total_count"`
	PerPage    int `json:"per_page"`
}

func pageInfoDone(page, resultCount int, info pageInfo, requestedPerPage int) bool {
	if resultCount == 0 {
		return true
	}
	if info.TotalPages > 0 {
		return page >= info.TotalPages
	}
	perPage := info.PerPage
	if perPage <= 0 {
		perPage = requestedPerPage
	}
	if info.TotalCount > 0 {
		return page*perPage >= info.TotalCount
	}
	return resultCount < perPage
}

func (c *APIClient) ListTunnels(ctx context.Context, accountID, name string) ([]Tunnel, error) {
	accountID, err := safeID(accountID, "account")
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if len(name) > 120 || strings.ContainsAny(name, "\x00\r\n") {
		return nil, errors.New("tunnel name is invalid")
	}
	tunnels := make([]Tunnel, 0)
	for page := 1; page <= maxListPages; page++ {
		values := url.Values{}
		values.Set("is_deleted", "false")
		values.Set("per_page", "100")
		values.Set("page", strconv.Itoa(page))
		if name != "" {
			values.Set("name", name)
		}
		var result struct {
			Result     []Tunnel `json:"result"`
			ResultInfo pageInfo `json:"result_info"`
		}
		if err := c.do(ctx, http.MethodGet, "/accounts/"+accountID+"/cfd_tunnel?"+values.Encode(), nil, &result); err != nil {
			return nil, err
		}
		tunnels = append(tunnels, result.Result...)
		if pageInfoDone(page, len(result.Result), result.ResultInfo, 100) {
			return tunnels, nil
		}
	}
	return nil, errors.New("Cloudflare returned too many tunnel pages")
}

func (c *APIClient) CreateTunnel(ctx context.Context, accountID, name string) (Tunnel, error) {
	accountID, err := safeID(accountID, "account")
	if err != nil {
		return Tunnel{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 120 || strings.ContainsAny(name, "\x00\r\n") {
		return Tunnel{}, errors.New("tunnel name is invalid")
	}
	var result struct {
		Result Tunnel `json:"result"`
	}
	body := map[string]string{"name": name, "config_src": "cloudflare"}
	if err := c.do(ctx, http.MethodPost, "/accounts/"+accountID+"/cfd_tunnel", body, &result); err != nil {
		return Tunnel{}, err
	}
	if strings.TrimSpace(result.Result.ID) == "" {
		return Tunnel{}, errors.New("Cloudflare returned an invalid tunnel")
	}
	return result.Result, nil
}

func (c *APIClient) ConfigureTunnel(ctx context.Context, accountID, tunnelID string, ingress []IngressRule) error {
	accountID, err := safeID(accountID, "account")
	if err != nil {
		return err
	}
	tunnelID, err = safeID(tunnelID, "tunnel")
	if err != nil {
		return err
	}
	if len(ingress) == 0 || len(ingress) > 64 {
		return errors.New("tunnel ingress configuration is invalid")
	}
	for _, rule := range ingress {
		if strings.TrimSpace(rule.Service) == "" || len(rule.Service) > 2048 || strings.ContainsAny(rule.Service, "\x00\r\n") {
			return errors.New("tunnel ingress service is invalid")
		}
	}
	body := map[string]any{"config": map[string]any{"ingress": ingress}}
	return c.do(ctx, http.MethodPut, "/accounts/"+accountID+"/cfd_tunnel/"+tunnelID+"/configurations", body, &struct{}{})
}

func (c *APIClient) TunnelConfiguration(ctx context.Context, accountID, tunnelID string) ([]IngressRule, error) {
	accountID, err := safeID(accountID, "account")
	if err != nil {
		return nil, err
	}
	tunnelID, err = safeID(tunnelID, "tunnel")
	if err != nil {
		return nil, err
	}
	var result struct {
		Result struct {
			Config struct {
				Ingress []IngressRule `json:"ingress"`
			} `json:"config"`
		} `json:"result"`
	}
	if err := c.do(ctx, http.MethodGet, "/accounts/"+accountID+"/cfd_tunnel/"+tunnelID+"/configurations", nil, &result); err != nil {
		return nil, err
	}
	if len(result.Result.Config.Ingress) == 0 || len(result.Result.Config.Ingress) > 64 {
		return nil, errors.New("Cloudflare returned an invalid tunnel configuration")
	}
	for _, rule := range result.Result.Config.Ingress {
		if strings.TrimSpace(rule.Service) == "" || len(rule.Service) > 2048 || strings.ContainsAny(rule.Service, "\x00\r\n") {
			return nil, errors.New("Cloudflare returned an invalid tunnel configuration")
		}
	}
	return result.Result.Config.Ingress, nil
}

func (c *APIClient) TunnelStatus(ctx context.Context, accountID, tunnelID string) (TunnelStatus, error) {
	accountID, err := safeID(accountID, "account")
	if err != nil {
		return TunnelStatus{}, err
	}
	tunnelID, err = safeID(tunnelID, "tunnel")
	if err != nil {
		return TunnelStatus{}, err
	}
	var result struct {
		Result TunnelStatus `json:"result"`
	}
	if err := c.do(ctx, http.MethodGet, "/accounts/"+accountID+"/cfd_tunnel/"+tunnelID, nil, &result); err != nil {
		return TunnelStatus{}, err
	}
	return result.Result, nil
}

func (c *APIClient) TunnelToken(ctx context.Context, accountID, tunnelID string) (string, error) {
	accountID, err := safeID(accountID, "account")
	if err != nil {
		return "", err
	}
	tunnelID, err = safeID(tunnelID, "tunnel")
	if err != nil {
		return "", err
	}
	var result struct {
		Result string `json:"result"`
	}
	if err := c.do(ctx, http.MethodGet, "/accounts/"+accountID+"/cfd_tunnel/"+tunnelID+"/token", nil, &result); err != nil {
		return "", err
	}
	if strings.TrimSpace(result.Result) == "" {
		return "", errors.New("Cloudflare returned an empty tunnel token")
	}
	return result.Result, nil
}

func StatusIsHealthy(status TunnelStatus) bool {
	return strings.EqualFold(strings.TrimSpace(status.Status), "healthy")
}

func NumericStatus(value int) string { return strconv.Itoa(value) }
