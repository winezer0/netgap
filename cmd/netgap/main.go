package main

import (
	"fmt"
	"os"

	logging "github.com/winezer0/zaplogs"

	"netgap/internal/config"
	"netgap/internal/ipchecker"
	"netgap/internal/ipchecker/localdb"
	"netgap/internal/iproute"
	"netgap/internal/monitor"
)

func main() {
	// 解析命令行参数
	opts, err := ParseOptions()
	if err != nil {
		fmt.Printf("加载配置失败: %v\n", err)
		return
	}

	// 初始化日志记录器
	logCfg := logging.NewConfig(opts.LogLevel, opts.LogFile, opts.ConsoleFormat)
	logCfg.FileLevel = opts.LogLevel
	logCfg.MaxBackups = 10
	if err := logging.InitDefaultLogger(logCfg); err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logging.CloseAll()

	// 处理 --api-list 选项
	if opts.APIList {
		fetchers := ipchecker.GetAllFetchers()
		fmt.Println("当前支持的IP查询接口:")
		for _, fetcher := range fetchers {
			fmt.Printf("  - 关键字: %s, 名称: %s\n", fetcher.Key(), fetcher.Name())
		}
		return
	}

	// 处理 --recover 选项
	if opts.Recover {
		if !iproute.CheckAdminPermission() {
			fmt.Println("error: admin permission required to restore route")
			return
		}

		fmt.Println("尝试恢复默认路由...")
		route, err := iproute.LoadRouteInfo()
		if err != nil {
			fmt.Printf("从缓存文件加载路由信息失败: %v\n", err)
			fmt.Println("请手动修复默认路由，参考命令:")
			fmt.Println("  Windows: route add 0.0.0.0 mask 0.0.0.0 网关IP")
			fmt.Println("  Linux: ip route add default via 网关IP")
			return
		}

		err = iproute.AddDefaultRoute(route.Gateway)
		if err != nil {
			fmt.Printf("添加默认路由失败: %v\n", err)
			return
		}

		fmt.Printf("默认路由已成功恢复，网关: %s\n", route.Gateway)
		return
	}

	// 处理 --shutdown-test 选项
	if opts.ShutdownTest {
		if !iproute.CheckAdminPermission() {
			fmt.Println("error: admin permission required to delete route")
			return
		}

		// 先保存当前默认路由信息
		route, err := iproute.GetDefaultRoute()
		if err != nil {
			fmt.Printf("get default route failed: %v\n", err)
		} else {
			err = iproute.SaveRouteInfo(route)
			if err != nil {
				fmt.Printf("save route info failed: %v\n", err)
			} else {
				fmt.Printf("saved default route info, gateway: %s\n", route.Gateway)
			}
		}

		// 删除默认路由
		err = iproute.DeleteDefaultRoute()
		if err != nil {
			fmt.Printf("delete default route failed: %v\n", err)
			return
		}

		fmt.Println("default route deleted successfully, network disconnected")
		return
	}

	// 处理 --shutdown 选项
	if opts.Shutdown {
		if !iproute.CheckAdminPermission() {
			fmt.Println("错误: 需要管理员权限才能执行网络操作")
			return
		}

		// 保存当前默认路由信息
		route, err := iproute.GetDefaultRoute()
		if err != nil {
			fmt.Printf("获取默认路由失败: %v\n", err)
			return
		}

		err = iproute.SaveRouteInfo(route)
		if err != nil {
			fmt.Printf("保存路由信息失败: %v\n", err)
			return
		}

		fmt.Printf("已保存默认路由信息，网关: %s\n", route.Gateway)
	}

	// 检查必要参数 - 现在 Regions 有默认值，所以不需要必检
	if len(opts.Regions) == 0 {
		fmt.Println("警告: 未指定本地区域，使用默认值: 湖南, 北京, 上海")
	}

	// 构建运行时配置，仅传递必要字段
	cfg := &config.Config{
		Regions:  opts.Regions,
		Interval: opts.Interval,
		APIUse:   opts.APIUse,
		APIBan:   opts.APIBan,
		Shutdown: opts.Shutdown,
	}

	// 启动监控
	resolver, err := localdb.NewResolver()
	if err != nil {
		logging.Errorf("failed to initialize local IP databases: %v", err)
		return
	}
	defer func() {
		if err := resolver.Close(); err != nil {
			logging.Errorf("failed to close local IP databases: %v", err)
		}
	}()

	// Start monitoring.
	monitor.StartMonitoring(cfg, monitor.NewChecker(resolver))
}
