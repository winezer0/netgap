# 按系统规则检测实际出口

## Linux GIO 更新

GNOME 手动规则保留 B1 实现；GNOME 自动模式、KDE/libproxy 使用独立 `netgap-gio-proxy`，对每个 API/重定向地址调用系统解析器。缺助手、缺模块、未知桌面、配置/解析失败均报错，不回退环境代理或追加 DIRECT。明确无桌面和 GNOME mode=none 才进入原有环境规则。GNOME/KDE PAC 与服务失败恢复已有 WSL 原生证据；部署依赖和限制见 [本批次结果](gio-watch-recovery-results.md)。

本轮已按审查结果调整代码，尚未提交；Windows 全量与 WSL 选定回归已执行，未执行真实断网。macOS 双架构纯 Go 交叉编译不覆盖原生桥接，仍待原生构建。当前进展及限制见 [修复进展](proxy-route-repair-progress.md)。

## 已修复与剩余问题

| 范围 | 状态 |
| --- | --- |
| API 永久丢失、Windows 连接拒绝识别 | 已修复 |
| 停止/重启任务隔离、旧网络查询结果校验 | 已修复，真实切网仍待集成验证 |
| 协议/URL 凭据保留、实际路径日志、连接回收 | 已修复，企业集成认证不支持 |
| PAC/WPAD 与固定代理逐网址选路 | 已实现；候选连接预算已修复代码，待回归 |
| 环境代理按网址选路 | 验证顺序已修复代码，待回归 |
| 系统读取取消与超时 | Linux helper 超时、进程组取消、输出限制及恢复已在 WSL 专项验证；原生同步读取仍不可中断 |
| Linux GNOME 手动规则 | 已实现 HTTP/HTTPS/SOCKS、共享、绕过与 HTTP 认证，WSL 专项及本地传输回归通过；PAC/其他来源仍待补齐 |
| 删除失败后保留监控与备份 | 已补充 GUI/CLI 独立续作，待故障与并发回归 |
| 所有默认路由快照与网络绑定 | 持久恢复、身份冲突及相关子网校验已调整，待真实系统验证 |
| Linux 断网、命令载荷容量 | 已新增 Netlink 后端与标准输入通道，待运行验证；不支持的平台启用前明确拒绝 |
| macOS 平台后端 | 已增加 CFNetwork 逐网址解析、网络变化检测、原生完整路由操作和 GUI 独立数据目录，待原生构建与运行验证 |

## 本次实现

Windows 下，每轮监控重新读取当前用户的系统代理配置。每个 IP 查询请求及其重定向地址，分别应用对应规则，再通过选中的路径发出实际 HTTP 请求。

| 配置 | 处理方式 |
| --- | --- |
| 固定代理 | 按查询网址的 HTTP/HTTPS 协议选择对应代理；支持通用代理、SOCKS 及绕过列表 |
| PAC | 使用 Windows WinHTTP 异步接口对每个网址执行 PAC，保留系统返回的候选路径顺序 |
| 自动发现 WPAD | 使用系统 DHCP/DNS 发现；仅自动发现且明确报告未发现 PAC 时，使用固定配置或直连 |
| PAC/WPAD 与固定代理同时存在 | 优先自动规则；PAC 返回的 DIRECT 就是该网址实际检测的直连路径 |
| 环境变量代理 | 系统未配置代理时使用；逐网址应用 HTTP_PROXY、HTTPS_PROXY、ALL_PROXY、NO_PROXY 及小写形式 |
| 无代理 / DIRECT | 明确使用直连传输，避免 Go 默认传输层再次隐式读取其他代理 |

固定代理请求层支持 HTTP、HTTPS、SOCKS4/4a、SOCKS5/5h。Windows `socks=host:port` 按 SOCKS4 解释。URL 中提供的 HTTP Basic 或 SOCKS5 用户名和密码保留用于连接，不输出到路径日志。

macOS 每轮通过 CFNetwork 读取系统代理，对查询与重定向网址调用系统选路及 PAC 执行接口，保留候选顺序、绕过规则和显式 DIRECT。系统 HTTPS 类型按 HTTP CONNECT 连接，SOCKS 类型按 SOCKS5 连接；没有配置系统代理时使用环境规则。自动发现依赖系统返回入口，PAC/WPAD 失败报错，不追加直连。完整功能与验证边界见 [macOS 功能支持](macos-support.md)。

## 失败与恢复

- 按规则依次尝试候选代理，只有规则明确提供 DIRECT 时才允许回退直连；不会在代理失败后自行追加直连。
- 规则解析与候选连接分别分配时间，连接预算最多 1.5 秒并按剩余时间和候选数缩短，为后续路径及响应读取留出时间；请求已发送后不因响应超时切换路径。
- 只对无请求体的 GET 在连接失败时尝试后续候选；HTTP 状态错误不会触发路径切换。
- 一个查询网址失败后，继续使用其他查询网址各自的规则，不因它们可能选择不同代理而提前结束。
- PAC 下载、执行、配置解析失败会明确报错，监控继续后续轮询。取消或停止监控会关闭本次原生解析句柄并取消网络请求。
- 成功结果记录最后一次实际 HTTP 请求使用的路径，日志显示 `DIRECT` 或不含凭据的代理地址；地区未命中不再表述为“代理工作正常”。
- 断网续作失败或退避期间仍执行出口查询，不会用新结果扩大旧任务。GUI/CLI 分开记录查询健康与路由进度，展示最后成功时间、失败分类及重试状态；这些改动已编译，待故障回归。
- 代理配置读取共用 3 秒预算，指纹共用 5 秒预算，继承所属查询/路由操作的取消。Linux gsettings 可终止进程组并限制输出及管道等待；已识别系统配置读取失败时明确报错。CLI Ctrl+C/SIGTERM 和 GUI 生命周期取消沿调用链传递；同步系统 API 不能保证在预算内中断，只在返回后拒绝过期结果。

## 适用范围与限制

- Windows 使用 WinHTTP，macOS 使用 CFNetwork。Linux GNOME 手动模式的 HTTP/HTTPS/SOCKS、共享、绕过和认证语义见 [B1 记录](wsl-gnome-test-results.md)；自动模式和 KDE 现使用上方 GIO 助手。缺少 gsettings 不再被当成系统未配置代理。手动模式选定后的 DIRECT 不再使用环境代理。
- Windows PAC 能力遵循 WinHTTP，其 PAC 下载支持 HTTP/HTTPS，不支持 `file:` PAC；不能把浏览器扩展等同于系统能力。macOS 自动配置执行委托 CFNetwork，本轮未验证 `file:` PAC；不能宣称覆盖。不支持或解析失败时返回错误。
- 不自动提供 NTLM/Kerberos/Negotiate 企业认证；需要此类认证的 PAC 下载或代理请求可能失败。HTTP Basic 与 SOCKS5 认证须在代理 URL 中提供凭据。
- 系统代理配置在一轮查询开始时读取。查询前后核对网络与代理指纹，Windows/macOS 变化订阅参与版本检查；发生变化则丢弃旧结果。删除路由前再次核对，准备备份期间变化也会终止操作。

## 删除与恢复全部默认路由

按用户选择，自动断网、GUI 手动断网和 CLI `--shutdown-test` 统一执行以下流程：

1. 枚举所有网卡的 IPv4 `0.0.0.0/0` 与 IPv6 `::/0`；Windows 同时读取 ActiveStore 和 PersistentStore，Linux/macOS 捕获运行时原生路由，macOS 包含接口作用域路由。
2. 保存路由的原生属性及接口/网络绑定，Windows 包含下一跳、GUID、地址与前缀、网络配置标识、跃点、协议、发布、有效期和策略存储。完整快照使用原子写入并同步到磁盘，保存失败不开始删除。
3. 另存任务 ID、确认的目标和代理指纹，按精确记录删除并核对接口身份与属性，结束后确认没有默认路由残留。中途失败保留恢复记录和监控，GUI/CLI 后续优先续作原任务，无需重新取得公网 IP。
4. 同一路由身份再次出现时更新其恢复属性和有效期，其他网络已删除记录保留。续作遇到新路由、网络或代理变化时转回检测，不沿用旧结果扩大范围。
5. 恢复时核对稳定接口、网络和相关地址族/子网，处理已过去的有效期；macOS 保留原生绝对过期时间，过期后拒绝恢复。Windows 持久路由通过省略 PolicyStore 创建；附带活动记录在持久化副作用记录的约束下恢复或清理。已有无关冲突不会被覆盖。
6. 部分恢复后保留失败记录及其关联存储/副作用记录；全部成功后清除快照。进程内串行执行，Windows 独占句柄或 Linux/macOS 文件锁阻止多进程同时修改缓存；恢复前使旧断网任务失效。

当前缓存为 `iproute.cache.json`，格式版本为 3，按研发阶段要求不兼容旧缓存。Linux 已新增原生 Netlink 双栈默认路由捕获、属性保存、删除和恢复；未修改 NetworkManager 等持久配置。无法安全绑定接口的路由类型会在删除前明确拒绝；真实 Linux 运行仍待验证。

macOS 使用原生路由套接字捕获和修改，不修改网络服务持久配置；不能识别接口或网关类型时明确失败。启用完整保护要求 CGO 和有效 root 权限，数据目录与恢复方式见 [macOS 说明](macos-support.md#权限与恢复数据)。

删除默认路由不等同于防火墙断网：直连网段、其他更具体路由、VPN 的 `/1` 路由或之后由系统重新生成的路由仍可能提供连接。操作期间系统路由表也无法作为一个事务冻结，当前实现通过复核、逐条处理和保留恢复记录处理变化。

停止监控会立即取消当前任务，不等待路由操作锁；正在执行的系统命令收到取消后终止，已删除部分的恢复记录保留。

## 检测失败分类

CLI 与 GUI 日志区分 `network_changed`、`proxy_rules`、`proxy_unavailable`、`network_request`、`timeout`、`api_unavailable`、`api_response`、`local_resolution`、`canceled`，并为断网续作增加 `disconnect_pending`、`disconnect_obsolete` 分类。健康状态中的路由失败另展示续作错误的内层分类，权限预检失败标记为 `permission`。

## 本轮编译检查

Windows 相关包与 CLI/GUI 入口编译通过；Linux 相关包与 CLI 交叉编译通过；macOS amd64/arm64 核心包与 CLI 在 CGO 禁用下交叉编译通过，Apple 框架与原生 GUI 尚未编译。本轮未新增或运行测试，未执行系统路由变更。以下为前一实现版本的验证记录，不代表覆盖本轮改动。

## 历史验证记录

已通过检测器、查询器、路由、监控核心、GUI 服务和 CLI 的包测试及相关竞态回归，CLI/GUI 入口编译通过，`git diff --check` 通过。

已实际运行本地 PAC 下载/执行，验证逐网址 DIRECT/代理分流、候选顺序、错误脚本与下载中的取消；验证 HTTP 代理认证、SOCKS4/4a/5/5h 握手与 SOCKS5 认证、重定向重新选路、显式 DIRECT 回退及禁止隐式直连。路由枚举和接口核对进行了 Windows 只读运行验证，删除/恢复脚本通过 PowerShell 语法解析，恢复脚本也通过模拟 cmdlet 验证。

删除、恢复、旧网络结果、删除前再次切换及备份重试使用模拟后端测试；没有修改本机系统代理、网络或路由。真实 Wi-Fi/网线/VPN 切换、企业 WPAD 部署、动态路由恢复以及实际删除/恢复命令仍需隔离环境集成验证，不能将当前回归结果视为这些场景已全部通过。

## 原生接口依据

- [WinHttpGetIEProxyConfigForCurrentUser](https://learn.microsoft.com/en-us/windows/win32/api/winhttp/nf-winhttp-winhttpgetieproxyconfigforcurrentuser)：读取当前用户配置。
- [WinHttpGetProxyForUrlEx](https://learn.microsoft.com/en-us/windows/win32/api/winhttp/nf-winhttp-winhttpgetproxyforurlex)：逐网址异步解析、取消及完整候选列表。
- [WINHTTP_AUTOPROXY_OPTIONS](https://learn.microsoft.com/en-us/windows/win32/api/winhttp/ns-winhttp-winhttp_autoproxy_options)：自动发现与 PAC 下载认证行为。
- [Get-NetRoute](https://learn.microsoft.com/en-us/powershell/module/nettcpip/get-netroute)：读取活动及持久路由配置。
- [New-NetRoute](https://learn.microsoft.com/en-us/powershell/module/nettcpip/new-netroute)：路由协议、策略存储与有效期参数。
