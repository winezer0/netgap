package fetchers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"netgap/internal/ipchecker/types"
)

type jsonIPFetcher struct {
	name     string
	key      string
	endpoint string
	ipField  string
}

// NewIPConfigIOFetcher creates a fetcher for ipconfig.io's JSON endpoint.
func NewIPConfigIOFetcher() types.IPFetcher {
	return &jsonIPFetcher{
		name:     "ipconfig.io",
		key:      "ipconfig",
		endpoint: "https://ipconfig.io/json",
		ipField:  "ip",
	}
}

// NewIPifyFetcher creates a fetcher for api.ipify.org.
func NewIPifyFetcher() types.IPFetcher {
	return &jsonIPFetcher{
		name:     "api.ipify.org",
		key:      "ipify",
		endpoint: "https://api.ipify.org?format=json",
		ipField:  "ip",
	}
}

// NewSeeIPFetcher creates a fetcher for api.seeip.org.
func NewSeeIPFetcher() types.IPFetcher {
	return &jsonIPFetcher{
		name:     "api.seeip.org",
		key:      "seeip",
		endpoint: "https://api.seeip.org/jsonip",
		ipField:  "ip",
	}
}

// NewIPAPIQueryFetcher creates a fetcher for ip-api.com's query-only endpoint.
func NewIPAPIQueryFetcher() types.IPFetcher {
	return &jsonIPFetcher{
		name:     "ip-api.com",
		key:      "ipapi",
		endpoint: "http://ip-api.com/json?fields=query",
		ipField:  "query",
	}
}

func (f *jsonIPFetcher) Name() string { return f.name }

func (f *jsonIPFetcher) Key() string { return f.key }

func (f *jsonIPFetcher) Fetch(ctx context.Context, client *http.Client) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, f.endpoint, nil)
	if err != nil {
		return "", err
	}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("API returned HTTP status %s", response.Status)
	}

	var payload map[string]json.RawMessage
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return "", err
	}
	value, ok := payload[f.ipField]
	if !ok {
		return "", fmt.Errorf("API response does not contain %q", f.ipField)
	}

	var ip string
	if err := json.Unmarshal(value, &ip); err != nil {
		return "", fmt.Errorf("decode IP field %q: %w", f.ipField, err)
	}
	return strings.TrimSpace(ip), nil
}
