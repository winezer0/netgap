# macOS 功能支持

本批次已修复监听初始化/退出失败后永久不可恢复的代码路径：路由 socket 和 SCDynamicStore 均可退避重建，故障与恢复推进版本；配置快照读取失败会停止旧监听并重试，恢复前不接受旧检测结果。macOS Intel / ARM 原生 CI 已完成核心竞态测试、Objective-C 后端和 CLI 编译、完整 GUI 构建及安装包上传；线程 / RunLoop 自愈的真实故障行为和交互式桌面验收仍待验证。详见 [原生构建与桌面验收记录](native-desktop-validation.md)。

macOS 已增加系统代理、网络变化检测、完整默认路由快照及删除/恢复后端，CLI 与 GUI 共用。当前状态为**原生构建和核心自动化测试已通过，交互式桌面及真实路由行为待验证**；CI 使用原生 Intel / ARM runner，未以交叉编译代替原生构建。

## 功能范围

| 功能 | macOS 实现与限制 |
| --- | --- |
| 固定 HTTP/HTTPS 代理 | 通过 CFNetwork 读取系统配置，按每个查询网址和重定向网址解析；保留系统候选顺序与绕过结果 |
| SOCKS 系统代理 | CFNetwork 的 SOCKS 类型按 SOCKS5 连接；不能从该类型推断 SOCKS4，仅支持 SOCKS4 的服务器不在此系统模式能力内 |
| PAC / 自动发现 WPAD | 使用系统返回的自动配置入口，由 CFNetwork 下载/执行 PAC；自动发现依赖系统配置和发现结果，真实 WPAD 部署仍待验证 |
| DIRECT 与候选回退 | 仅使用系统明确返回的 DIRECT；代理拒绝、PAC 失败不会自行增加直连；候选连接沿用公共预算与回退逻辑 |
| 取消解析 | 停止监控或查询超时后取消请求，并撤销 PAC 的 CFRunLoopSource；原生等待每 50ms 检查取消，PAC 内部等待上限 25 秒，仍受查询取消约束 |
| 同步配置/绑定读取 | 接收所属操作 context，在原生调用前后检查取消；调用一旦进入同步 Apple 框架仍不可中断，不能承诺 3/5 秒读取预算或 15/60 秒路由预算是硬上限 |
| 环境代理 | 没有启用系统代理时，逐网址应用 HTTP_PROXY、HTTPS_PROXY、ALL_PROXY、NO_PROXY；读取系统配置失败会报错 |
| 出口 IP / 归属地 / 告警 | 使用公共查询器、本地 IPv4/IPv6 数据库及区域匹配逻辑 |
| 网络变化 | 组合接口状态、系统网络配置、原生路由表及变化订阅生成指纹；查询前后变化则丢弃旧结果 |
| 全部默认路由 | 捕获所有接口 IPv4 `0.0.0.0/0` 与 IPv6 `::/0`，包含带接口作用域及无作用域的默认路由 |
| 保存、删除和恢复 | 保存原生路由消息及接口/网络绑定，以路由套接字操作并等待内核确认；恢复核对接口、启动会话、网络与相关子网，遇到冲突保留备份 |
| 部分断网续作 | 共用持久任务、代理指纹、限定目标和退避机制，无需再次取得公网 IP；网络或目标变化后重新检测 |
| 多进程互斥 | 使用快照文件锁；修改前持久保存恢复信息，失败记录保留 |
| GUI 配置与日志 | 写入当前执行用户的 `~/Library/Application Support/netgapgui/`，不修改 `.app` 包内容 |

macOS 的 HTTPS 系统代理类型表示供 HTTPS 目标使用的 HTTP CONNECT 代理，因此连接代理使用 `http://`。不能将该类型直接当作“以 TLS 连接代理服务器”的 `https://`。

凭据只使用 CFNetwork 实际提供的用户名/密码，不自动读取钥匙串、弹出认证窗口或实现 NTLM/Kerberos/Negotiate。应用自身的代理配置、浏览器扩展、VPN 和 Network Extension 不一定通过系统代理 API 暴露；最终查询还会受内核网络路径影响。

ARP/NDP 缓存及按连接生成的克隆主机路由不进入配置指纹，避免查询流量令自身结果持续失效。真实切网、VPN、接口复用及高频网络变化仍需集成验证。

## 构建与启动

原生后端需要 macOS、Apple SDK/Clang 和 `CGO_ENABLED=1`。构建机安装 Xcode Command Line Tools；GUI 另需 Wails 与 Node.js。运行已构建程序不需要安装 Go、Xcode 或 Node.js。

```sh
# macOS 主机上构建 CLI（当前机器架构）
CGO_ENABLED=1 go build -o netgap ./cmd/netgap
./netgap --help

# macOS 主机上构建 amd64 + arm64 通用 GUI
cd cmd/netgapgui
CGO_ENABLED=1 wails build -platform darwin/universal
open build/bin/netgapgui.app
```

发布工作流已包含 macOS 通用 GUI，并增加原生后端与 CLI 编译步骤。尚未执行本次工作流，签名、公证和分发安装体验不在本轮交付范围。

另有独立 [Native Build 工作流](../.github/workflows/native-build.yml)，通过 push、pull_request 或手动调度，在 Intel 和 Apple Silicon runner 分别以 CGO 编译原生后端、CLI 和对应架构 GUI。此工作流不运行测试或发布产物，目前尚无执行结果；不替代发布中的 universal 打包检查。

`CGO_ENABLED=0` 的 macOS 构建仅用于纯 Go 编译检查；系统代理读取和自动断网能力会明确拒绝，不是可部署的完整版本。Wails/托盘 GUI 需要原生构建。

## 权限与恢复数据

普通用户可以监控、查询 IP 和读取网络状态。修改默认路由需要进程有效 UID 为 0；属于管理员用户组不等于已获得该权限。当前未实现特权辅助进程或自动提权。

CLI 路由恢复文件为当前执行用户的 `~/.cache/netgap/iproute.cache.json`；GUI 为当前执行用户的 `~/Library/Application Support/netgapgui/iproute.cache.json`，日志与配置也在该 GUI 目录。格式版本为 3，不导入旧版本快照。

删除与恢复应使用同一程序入口、执行身份和数据目录。例如在隔离 macOS 环境中使用相同的 `sudo -H` 方式执行 CLI 的 `--shutdown` 与 `--recover`。普通用户与 root、GUI 与 CLI 的默认恢复目录不同，不能假设互相自动找到备份。

无法识别接口/网络绑定或遇到不支持的默认网关形式时，操作明确失败。切网、机器重启、链路重连后可能保守拒绝恢复旧路由，需要由系统重新获取网络配置；已有冲突路由不会被覆盖。带有效期的原生路由保留原绝对过期时间，不续期。

## 保护边界与待验证事项

- 仅操作运行时默认路由，不修改 macOS 网络服务的持久设置。系统可能重新生成路由；新目标必须重新检测。
- 删除默认路由不是防火墙隔离。直连网段、更具体路由、VPN `/1` 路由仍可能保持连接。
- 路由表不支持本项目所需的跨条目事务，逐条复核和恢复备份只能限制竞争影响。
- macOS Intel/ARM 的原生核心竞态测试、Apple 框架桥接/CLI 编译及完整 GUI 构建已在 CI 通过；桌面运行、PAC/WPAD 实际选路、监听故障自愈、双栈/多网卡/作用域路由及真实删除恢复仍待验收。
- 当前没有可用的 macOS 桌面测试环境；签名/公证和 Gatekeeper 验收也未完成。剩余项目及通过标准见 [主 README](../README.md#验收状态与已知限制)。后续真实断网验证须在可离线恢复的隔离 macOS 环境执行，不修改日常开发机网络来代替验收。

## 接口依据

- [Apple CFNetworkCopyProxiesForURL](https://developer.apple.com/documentation/cfnetwork/cfnetworkcopyproxiesforurl(_:_:))：按目标 URL 获取系统候选路径。
- [Apple PAC 异步执行接口](https://developer.apple.com/documentation/CFNetwork/CFNetworkExecuteProxyAutoConfigurationURL%28_%3A_%3A_%3A_%3A%29)：通过 RunLoop 执行自动配置，并允许撤销事件源。
- [Go 原生路由接口](https://pkg.go.dev/golang.org/x/net/route)：读取和编码 macOS 路由消息。
- [Apple XNU 路由套接字实现](https://github.com/apple-oss-distributions/xnu/blob/main/bsd/net/rtsock.c)：原生确认及路由有效期语义。
