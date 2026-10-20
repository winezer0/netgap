package config

// Config 运行时配置，仅包含传递给其他模块的必要字段
type Config struct {
	Regions            []string
	Interval           int
	APIUse             []string
	APIBan             []string
	Shutdown           bool
	CustomAPIURL       string
	RemoveRouteOnMatch bool
}
