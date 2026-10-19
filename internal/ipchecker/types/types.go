package types

import (
	"context"
	"net/http"
)

// IPInfo IP信息结构体
type IPInfo struct {
	IP      string
	Country string
	Region  string
	City    string
	Isp     string
}

// IPFetcher IP获取接口
type IPFetcher interface {
	// Fetch 获取IP信息
	Fetch(ctx context.Context, client *http.Client) (string, error)
	// Name 获取查询器名称
	Name() string
	// Key 获取查询器关键字，用于命令行匹配
	Key() string
}
