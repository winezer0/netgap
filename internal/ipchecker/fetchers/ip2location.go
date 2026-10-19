package fetchers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"netgap/internal/ipchecker/types"
)

// NewIP2LocationFetcher 创建 ip2location.io 查询器
func NewIP2LocationFetcher() types.IPFetcher {
	return &ip2LocationFetcher{endpoint: "https://api.ip2location.io/"}
}

// ip2LocationFetcher ip2location.io 查询器
type ip2LocationFetcher struct {
	endpoint string
}

// Name 获取查询器名称
func (f *ip2LocationFetcher) Name() string {
	return "ip2location"
}

// Key 获取查询器关键字
func (f *ip2LocationFetcher) Key() string {
	return "ip2location"
}

// IP2LocationResponse ip2location.io 响应结构
type IP2LocationResponse struct {
	IP string `json:"ip"`
}

// Fetch 获取IP信息
func (f *ip2LocationFetcher) Fetch(ctx context.Context, client *http.Client) (string, error) {
	endpoint := strings.TrimSpace(f.endpoint)
	if endpoint == "" {
		endpoint = "https://api.ip2location.io/"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var ipResp IP2LocationResponse
	if err := json.Unmarshal(body, &ipResp); err != nil {
		return "", err
	}

	return strings.TrimSpace(ipResp.IP), nil
}
