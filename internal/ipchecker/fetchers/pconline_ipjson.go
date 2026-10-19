package fetchers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"

	"netgap/internal/ipchecker/types"
)

// NewPConlineIPJsonFetcher 创建 pconline ipJson.jsp 查询器
func NewPConlineIPJsonFetcher() types.IPFetcher {
	return &pconlineIPJsonFetcher{endpoint: "https://whois.pconline.com.cn/ipJson.jsp"}
}

// pconlineIPJsonFetcher pconline ipJson.jsp 查询器
type pconlineIPJsonFetcher struct {
	endpoint string
}

// Name 获取查询器名称
func (f *pconlineIPJsonFetcher) Name() string {
	return "pconline-ipjson"
}

// Key 获取查询器关键字
func (f *pconlineIPJsonFetcher) Key() string {
	return "pcjson"
}

// PConlineIPJsonResponse pconline ipJson.jsp 响应结构
type PConlineIPJsonResponse struct {
	IP string `json:"ip"`
}

// Fetch 获取IP信息
func (f *pconlineIPJsonFetcher) Fetch(ctx context.Context, client *http.Client) (string, error) {
	endpoint := strings.TrimSpace(f.endpoint)
	if endpoint == "" {
		endpoint = "https://whois.pconline.com.cn/ipJson.jsp"
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

	// 使用 GBK 解码器读取响应内容
	decoder := transform.NewReader(resp.Body, simplifiedchinese.GBK.NewDecoder())
	body, err := io.ReadAll(decoder)
	if err != nil {
		return "", err
	}

	// 解析响应内容
	content := string(body)

	// 提取 JSON 部分：查找包含 "ip" 键的 JSON 对象
	jsonStr := extractJSONObject(content)
	if jsonStr == "" {
		return "", nil
	}

	// 解析 JSON
	var ipResp PConlineIPJsonResponse
	if err := json.Unmarshal([]byte(jsonStr), &ipResp); err != nil {
		return "", err
	}

	return strings.TrimSpace(ipResp.IP), nil
}

// extractJSONObject 从内容中提取包含 "ip" 键的 JSON 对象
func extractJSONObject(content string) string {
	// 查找 {"ip" 模式，定位到真正的 JSON 对象
	marker := `{"ip"`
	idx := strings.Index(content, marker)
	if idx == -1 {
		// 尝试查找带空格的变体
		marker = `{ "ip"`
		idx = strings.Index(content, marker)
	}
	if idx == -1 {
		return ""
	}

	// 从找到的位置开始，匹配大括号对
	depth := 0
	start := idx
	for i := idx; i < len(content); i++ {
		if content[i] == '{' {
			depth++
		} else if content[i] == '}' {
			depth--
			if depth == 0 {
				return content[start : i+1]
			}
		}
	}

	return ""
}
