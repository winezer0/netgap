package iproute

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// RouteInfo 路由信息结构体
type RouteInfo struct {
	Gateway   string `json:"gateway" yaml:"gateway"`
	Interface string `json:"interface" yaml:"interface"`
}

// AppName 应用名称
var AppName = "netgap"

// GetCacheFilePath 获取缓存文件路径
func GetCacheFilePath() string {
	if cacheDirOverride != "" {
		return filepath.Join(cacheDirOverride, "iproute.cache")
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = ""
	}
	cacheDir := filepath.Join(homeDir, ".cache", AppName)
	os.MkdirAll(cacheDir, 0755)
	return filepath.Join(cacheDir, "iproute.cache")
}

// SaveRouteInfo 保存路由信息到缓存文件
func SaveRouteInfo(route *RouteInfo) error {
	cacheFile := GetCacheFilePath()
	content := fmt.Sprintf("Gateway: %s\nInterface: %s\n", route.Gateway, route.Interface)
	return os.WriteFile(cacheFile, []byte(content), 0644)
}

// LoadRouteInfo 从缓存文件加载路由信息
func LoadRouteInfo() (*RouteInfo, error) {
	cacheFile := GetCacheFilePath()
	content, err := os.ReadFile(cacheFile)
	if err != nil {
		return nil, err
	}

	route := &RouteInfo{}
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "Gateway: ") {
			route.Gateway = strings.TrimPrefix(line, "Gateway: ")
		} else if strings.HasPrefix(line, "Interface: ") {
			route.Interface = strings.TrimPrefix(line, "Interface: ")
		}
	}

	if route.Gateway == "" {
		return nil, fmt.Errorf("缓存文件中未找到网关信息")
	}

	return route, nil
}

// GetDefaultRoute 获取默认路由信息
func GetDefaultRoute() (*RouteInfo, error) {
	if runtime.GOOS == "windows" {
		return getWindowsDefaultRoute()
	} else {
		return getLinuxDefaultRoute()
	}
}

// getWindowsDefaultRoute 获取Windows默认路由
func getWindowsDefaultRoute() (*RouteInfo, error) {
	cmd := exec.Command("route", "print", "0.0.0.0")
	hideWindow(cmd)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, err
	}

	output := out.String()
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "0.0.0.0") {
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				return &RouteInfo{
					Gateway:   parts[2],
					Interface: parts[3],
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("未找到默认路由")
}

// getLinuxDefaultRoute 获取Linux默认路由
func getLinuxDefaultRoute() (*RouteInfo, error) {
	cmd := exec.Command("ip", "route", "show", "default")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, err
	}

	output := out.String()
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "default via") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				gateway := parts[2]
				interfaceName := ""
				if len(parts) >= 5 {
					interfaceName = parts[4]
				}
				return &RouteInfo{
					Gateway:   gateway,
					Interface: interfaceName,
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("未找到默认路由")
}

// DeleteDefaultRoute 删除默认路由
func DeleteDefaultRoute() error {
	if runtime.GOOS == "windows" {
		return deleteWindowsDefaultRoute()
	} else {
		return deleteLinuxDefaultRoute()
	}
}

// deleteWindowsDefaultRoute 删除Windows默认路由
func deleteWindowsDefaultRoute() error {
	cmd := exec.Command("route", "delete", "0.0.0.0")
	hideWindow(cmd)
	return cmd.Run()
}

// deleteLinuxDefaultRoute 删除Linux默认路由
func deleteLinuxDefaultRoute() error {
	cmd := exec.Command("ip", "route", "del", "default")
	return cmd.Run()
}

// AddDefaultRoute 添加默认路由
func AddDefaultRoute(gateway string) error {
	if runtime.GOOS == "windows" {
		return addWindowsDefaultRoute(gateway)
	} else {
		return addLinuxDefaultRoute(gateway)
	}
}

// addWindowsDefaultRoute 添加Windows默认路由
func addWindowsDefaultRoute(gateway string) error {
	cmd := exec.Command("route", "add", "0.0.0.0", "mask", "0.0.0.0", gateway)
	hideWindow(cmd)
	return cmd.Run()
}

// addLinuxDefaultRoute 添加Linux默认路由
func addLinuxDefaultRoute(gateway string) error {
	cmd := exec.Command("ip", "route", "add", "default", "via", gateway)
	return cmd.Run()
}

// CheckAdminPermission 检查是否具有管理员权限
func CheckAdminPermission() bool {
	if runtime.GOOS == "windows" {
		return checkWindowsAdminPermission()
	} else {
		return checkLinuxAdminPermission()
	}
}

// checkWindowsAdminPermission 检查Windows管理员权限
func checkWindowsAdminPermission() bool {
	cmd := exec.Command("net", "session")
	hideWindow(cmd)
	err := cmd.Run()
	return err == nil
}

// checkLinuxAdminPermission 检查Linux管理员权限
func checkLinuxAdminPermission() bool {
	return os.Geteuid() == 0
}
