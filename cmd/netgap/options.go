package main

import (
	"strings"

	"github.com/jessevdk/go-flags"
)

// Options 命令行参数结构体
type Options struct {
	Regions       []string `short:"R" long:"regions" description:"指定本地网络所在区域" default:"湖南,北京,上海"`
	Interval      int      `long:"interval" description:"检查间隔时间（秒）" default:"3"`
	LogFile       string   `long:"lf" description:"日志文件路径(默认仅输出到控制台)" default:""`
	LogLevel      string   `long:"ll" description:"日志级别: debug/info/warn/error" default:"info" choice:"debug" choice:"info" choice:"warn" choice:"error"`
	ConsoleFormat string   `long:"lc" description:"控制台日志格式, 支持T(time),L(level),C(caller),F(func),M(消息), 为空或off时关闭" default:"T L C M"`
	APIUse        []string `long:"api-use" description:"指定使用的IP查询接口列表"`
	APIBan        []string `long:"api-ban" description:"禁用的IP查询接口列表"`
	APIList       bool     `long:"api-list" description:"查询当前支持的IP查询接口"`
	Shutdown      bool     `short:"s" long:"shutdown" description:"启用网络代理失效时的自动记录并删除默认路由功能"`
	ShutdownTest  bool     `short:"S" long:"shutdown-test" description:"主动删除默认路由（测试用）"`
	Recover       bool     `short:"r" long:"recover" description:"恢复被删除的默认路由"`
}

// ParseOptions 解析命令行参数
func ParseOptions() (*Options, error) {
	var opts Options
	_, err := flags.Parse(&opts)
	if err != nil {
		return nil, err
	}

	opts.Regions = splitCommaSeparatedSlice(opts.Regions)
	opts.APIUse = splitCommaSeparatedSlice(opts.APIUse)
	opts.APIBan = splitCommaSeparatedSlice(opts.APIBan)

	return &opts, nil
}

// splitCommaSeparatedSlice 将切片中包含逗号的元素拆分为多个元素
func splitCommaSeparatedSlice(items []string) []string {
	if len(items) == 0 {
		return items
	}

	var result []string
	for _, item := range items {
		parts := strings.Split(item, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				result = append(result, part)
			}
		}
	}
	return result
}
