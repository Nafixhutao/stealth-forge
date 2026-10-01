package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type apiError struct {
	Code    json.Number `json:"code"`
	Message string      `json:"message"`
}

type apiEnvelope struct {
	Success bool       `json:"success"`
	Errors  []apiError `json:"errors"`
}

func (c *APIClient) do(ctx context.Context, method, path string, body any, result any) error {
	if c == nil || c.httpClient == nil {
		return errors.New("Cloudflare client is not configured")
	}
	var requestBody io.Reader
	if body != nil {
		contents, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode Cloudflare request: %w", err)
		}
		requestBody = bytes.NewReader(contents)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, requestBody)
	if err != nil {
		return fmt.Errorf("create Cloudflare request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+c.apiToken)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("Cloudflare request failed: %w", err)
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read Cloudflare response: %w", err)
	}
	var envelope apiEnvelope
	if err := json.Unmarshal(contents, &envelope); err != nil {
		return fmt.Errorf("Cloudflare returned invalid JSON (HTTP %d)", response.StatusCode)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || !envelope.Success {
		message := publicAPIError(envelope.Errors)
		if c.apiToken != "" {
			message = strings.ReplaceAll(message, c.apiToken, "[redacted]")
		}
		providerErr := fmt.Errorf("Cloudflare request was rejected (HTTP %d): %s", response.StatusCode, message)
		switch response.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return fmt.Errorf("%w: %v", ErrUnauthorized, providerErr)
		case http.StatusNotFound:
			return fmt.Errorf("%w: %v", ErrResourceNotFound, providerErr)
		default:
			return providerErr
		}
	}
	if result == nil {
		return nil
	}
	if err := json.Unmarshal(contents, result); err != nil {
		return fmt.Errorf("decode Cloudflare response: %w", err)
	}
	return nil
}

func safeID(value, label string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 || strings.ContainsAny(value, "\x00\r\n/\\?#[ ]") {
		return "", fmt.Errorf("%s ID is invalid", label)
	}
	return value, nil
}

func publicAPIError(errors []apiError) string {
	if len(errors) == 0 {
		return "the provider did not return an explanation"
	}
	message := strings.TrimSpace(errors[0].Message)
	if message == "" {
		message = "the provider did not return an explanation"
	}
	if len(message) > 180 {
		message = message[:180]
	}
	if errors[0].Code != "" {
		return "error " + errors[0].Code.String() + ": " + message
	}
	return message
}
