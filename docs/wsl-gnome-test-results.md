# Linux 手动代理修复与 WSL 测试（B1）

## 实施结果

已修复 GNOME 手动模式下 HTTP/HTTPS 查询的系统选路：一次有界 `gsettings list-recursively` 读取配置，解析 GVariant 字符串、布尔值及列表；HTTP、HTTPS、SOCKS、共享、绕过和 HTTP 认证参与逐网址选择。重定向继续使用目标网址的规则。

- HTTPS 独立配置优先；未配置 HTTPS 时沿用 HTTP。共享 HTTP 作为默认配置，独立协议配置优先，存在 SOCKS 时 SOCKS 覆盖默认配置。端点 host 为空或 port 为 0 表示该端点禁用。[GNOME schema](https://github.com/GNOME/gsettings-desktop-schemas/blob/master/schemas/org.gnome.system.proxy.gschema.xml.in)、[GNOME resolver 实现](https://github.com/GNOME/glib-networking/blob/master/proxy/gnome/gproxyresolvergnome.c)
- SOCKS 按 `socks5 → socks4a → socks4` 保留候选顺序，不追加 DIRECT。绕过支持域名及子域、IP、CIDR、带显式端口的地址；不沿用 Windows 的隐式 loopback 绕过。端口限定只匹配 URL 显式指定的端口。[GLib resolver 实现](https://github.com/GNOME/glib/blob/main/gio/gsimpleproxyresolver.c)、[绕过规则说明](https://gnome.pages.gitlab.gnome.org/gtk/gio/property.SimpleProxyResolver.ignore-hosts.html)
- 手动模式已选定但所有适用端点禁用时，按该系统配置返回 DIRECT，不再落入环境代理。解析/读取失败明确报错。自动模式仍明确拒绝，留待 B2；FTP 查询不在 IP API 的 HTTP/HTTPS 请求范围内。
- 指纹使用完整快照的稳定哈希，避免新增规则指针地址导致每轮指纹变化；凭据、绕过或其他设置变化可使旧结果失效。凭据不写入错误信息或测试输出。
- WSL 重复测试发现输出超限可能被 helper 的 SIGPIPE 掩盖，现已显式保留输出超限原因。

## 测试环境与范围

Ubuntu / WSL，普通用户 UID 1000，`go1.26.4 linux/amd64`，gcc 可用。测试 helper 仅通过测试进程 PATH 及临时目录接入；本地代理、源站都监听回环地址，保留的示例域名由模拟代理处理，不解析或访问公网目标。

未执行提权、真实路由删除/恢复、网络重启、网卡/DNS 修改或真实 `gsettings set/reset`。已有路由用例使用模拟后端。实际 GNOME 配置及网络状态只读读取。

WSL 首次依赖下载等待后已中止；后续使用已有模块缓存，`GOPROXY=off`、`GOSUMDB=off` 仅用于该测试进程，没有修改 `go env -w`、WSL 配置或系统代理。WSL 启动时已有 localhost 代理/NAT 提示，本次未调整该设置。

## 测试结果

所有最终执行退出码均为 0：

| 范围 | 结果 |
| --- | --- |
| WSL 新增 GNOME/GVariant/Linux helper 专项 | 通过；覆盖协议优先级、绕过、认证、配置错误、稳定指纹、读取恢复、超时、输出限制、进程组取消 |
| WSL detector + ipchecker 专项 `-race -count=10` | 10 轮通过，未报告数据竞争；包含重定向绕过、HTTP→HTTPS 独立代理及代理拒绝不直连 |
| WSL detector 全包 | 通过，包含现有系统配置只读检测 |
| WSL 六个核心包选定回归 | detector、ipchecker、monitor、guiapp/monitoring、iproute、cmd/netgap 通过；只运行下列指定测试，不是 Linux 全项目测试 |
| Windows `go test ./... -count=1 -timeout=180s` | 全量通过 |
| `git diff --check` | 通过 |

WSL 命令前缀（从仓库根目录执行，路径按当前环境）：

```text
wsl -d Ubuntu --exec env GOMODCACHE=/mnt/c/Program_Base/gopath/pkg/mod GOPROXY=off GOSUMDB=off go test
```

专项参数：

```text
-race ./internal/detector ./internal/ipchecker -run 'Test(Gnome|GVariant|LinuxGnome)' -count=10 -timeout=120s
```

选定回归参数：

```text
./internal/detector ./internal/ipchecker ./internal/monitor ./internal/guiapp/monitoring ./internal/iproute ./cmd/netgap -run 'Test(Gnome|GVariant|LinuxGnome|Recovery|Checker|Snapshot|System|Environment|Parse|Failure|Transport|Direct|Manager)' -count=1 -timeout=120s
```

## WSL 网络前后对比

使用 [只读指纹脚本](../scripts/wsl-network-state.py) 在测试前后读取配置，忽略自然递减的生命周期和缓存字段，不输出代理凭据。

| 检查项 | 前后结果 |
| --- | --- |
| IPv4/IPv6 全表路由 | 一致 |
| IPv4/IPv6 策略规则 | 一致 |
| 网卡地址与状态 | 一致 |
| `/etc/resolv.conf` | 一致 |
| GNOME 系统代理配置 | 一致 |
| 执行身份 | 均为 UID 1000 |

这是前后配置一致的证据，不代表验证了真实切网、断网或恢复。B2 Linux PAC/其他配置来源、macOS 原生后端、Linux 完整 GUI 和真实路由操作仍待后续任务。
