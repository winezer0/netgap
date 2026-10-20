# Linux 系统代理专项测试指南

本文保留 WSL/Linux 专项测试的环境和执行方法，不记录运行结果。当前场景状态见[已验证场景记录](已验证场景记录.md)和[剩余未验证场景记录](剩余未验证场景记录.md)。

## 测试环境

建议使用 Ubuntu/WSL 普通用户运行专项测试。测试 helper 通过测试进程的 PATH 和临时目录接入；本地代理、源站与 PAC 服务监听 loopback。使用私有 D-Bus、临时 GNOME 配置和临时 KDE `kioslaverc`，不要写入真实桌面配置。

测试前后可使用[只读指纹脚本](../scripts/wsl-network-state.py)检查 IPv4/IPv6 路由、策略规则、网卡地址、DNS、GNOME 代理设置和执行 UID 是否一致。不要在本指南的专项测试中提权或更改 WSL 路由、网卡、DNS、系统代理。

## 场景覆盖

- GNOME 手动 HTTP/HTTPS/SOCKS 代理、共享设置、绕过规则及认证信息解析。
- HTTP 与 HTTPS 独立代理、重定向目标、系统指定 DIRECT、PAC 候选顺序及错误配置处理。
- 配置快照读取、稳定指纹、取消/超时、输出限制、助手进程退出和恢复。
- GIO/libproxy 的 GNOME/KDE 按 URL 解析、PAC 服务退出及恢复；恢复后需重新解析当前规则。
- Linux 监控停止/重启、失败退避、健康状态与恢复预检。

## 运行命令

在仓库根目录执行 Linux 核心回归：

```sh
go test -race ./internal/netwatch ./internal/routenet ./internal/detector \
  ./internal/ipchecker ./internal/monitor ./internal/guiapp/monitoring \
  ./internal/iproute ./cmd/netgap -count=1 -timeout=180s
```

GIO 助手专项入口：

```sh
python3 scripts/test-gio-helper.py <助手> <glib-pacrunner>
```

可将 `NETGAP_GIO_DETECTOR_TEST` 指向与助手同目录的 detector 测试二进制；设置 `NETGAP_GIO_TEST_KDE=1` 增加 KDE/libproxy 场景。依赖路径及缓存选项应限于测试进程，不修改系统级 Go 或桌面配置。
