package fetchers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"netgap/internal/ipchecker/types"
)

// NewCustomIPFetcher 创建自定义 IP 查询 API 查询器
func NewCustomIPFetcher() types.IPFetcher {
	return &CustomIPFetcher{endpoint: ""}
}

// CustomIPFetcher 自定义 IP 查询 API 查询器
type CustomIPFetcher struct {
	endpoint string
}

// SetEndpoint 设置查询端点地址
func (f *CustomIPFetcher) SetEndpoint(url string) {
	f.endpoint = url
}

// Name 获取查询器名称
func (f *CustomIPFetcher) Name() string {
	return "custom-ipapi"
}

// Key 获取查询器关键字
func (f *CustomIPFetcher) Key() string {
	return "customip"
}

// CustomIPResponse 自定义 IP 查询 API 响应结构
type CustomIPResponse struct {
	Success bool                 `json:"success"`
	Code    int                  `json:"code"`
	Data    CustomIPDataResponse `json:"data"`
}

// CustomIPDataResponse 自定义 IP 查询 API 数据部分
type CustomIPDataResponse struct {
	IP string `json:"ip"`
}

// Fetch 获取IP信息
func (f *CustomIPFetcher) Fetch(ctx context.Context, client *http.Client) (string, error) {
	endpoint := strings.TrimSpace(f.endpoint)
	if endpoint == "" {
		return "", fmt.Errorf("custom IP API endpoint is empty")
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

	var ipResp CustomIPResponse
	if err := json.Unmarshal(body, &ipResp); err != nil {
		return "", err
	}

	if !ipResp.Success {
		return "", fmt.Errorf("API returned error code: %d", ipResp.Code)
	}

	return strings.TrimSpace(ipResp.Data.IP), nil
}
