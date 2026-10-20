# 按系统规则检测实际出口

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

## 失败与恢复

- 按规则依次尝试候选代理，只有规则明确提供 DIRECT 时才允许回退直连；不会在代理失败后自行追加直连。
- 只对无请求体的 GET 在连接失败时尝试后续候选；HTTP 状态错误不会触发路径切换。
- 一个查询网址失败后，继续使用其他查询网址各自的规则，不因它们可能选择不同代理而提前结束。
- PAC 下载、执行、配置解析失败会明确报错，监控继续后续轮询。取消或停止监控会关闭本次原生解析句柄并取消网络请求。
- 成功结果记录最后一次实际 HTTP 请求使用的路径，日志显示 `DIRECT` 或不含凭据的代理地址；地区未命中不再表述为“代理工作正常”。

## 适用范围与限制

- 本次完整系统配置读取与 PAC/WPAD 接入针对 Windows；其他平台沿用原有系统检测入口，不宣称提供相同覆盖。
- PAC 执行能力和可识别的代理类型遵循 Windows WinHTTP。其 PAC 下载支持 HTTP/HTTPS，不支持 `file:` PAC；不能把浏览器自身对 SOCKS5 等扩展的支持等同于 Windows 系统解析能力。不支持或解析失败时返回错误。
- 不自动提供 NTLM/Kerberos/Negotiate 企业认证；需要此类认证的 PAC 下载或代理请求可能失败。HTTP Basic 与 SOCKS5 认证须在代理 URL 中提供凭据。
- 系统代理配置在一轮查询开始时读取。查询前后核对网络与代理指纹，Windows 路由和接口变化通知也参与版本检查；发生变化则丢弃旧结果。删除路由前再次核对，准备备份期间变化也会终止操作。

## 删除与恢复全部默认路由

按用户选择，自动断网、GUI 手动断网和 CLI `--shutdown-test` 统一执行以下流程：

1. 枚举所有网卡的 IPv4 `0.0.0.0/0` 与 IPv6 `::/0`，同时读取 ActiveStore 和 PersistentStore。
2. 保存每条路由的下一跳、接口索引/GUID、地址与前缀、网络配置实例标识与名称、跃点、协议、发布属性、有效期和策略存储。完整快照使用原子写入并同步到磁盘，保存失败不开始删除。
3. 按精确路由记录删除，核对接口身份与关键属性，结束后确认没有默认路由残留。中途失败保留恢复记录，监控继续重试，不报告成功。
4. 重试或切换网络后的新操作合并恢复记录，不覆盖丢失此前记录；同一路由重新出现时刷新该条记录的有效期。
5. 恢复时逐条核对接口 GUID、地址和网络，恢复原有路由参数并扣除动态路由已过去的有效期。接口变化、有效期过期或已有冲突路由时明确报错，不覆盖当前路由。
6. 部分恢复后，仅保留失败记录供后续重试；全部成功后清除快照。进程内串行执行，并通过 Windows 独占文件句柄阻止两个进程同时修改恢复缓存。

缓存文件为现有 `iproute.cache` 路径后追加 `.v2.json`。旧的单网关缓存不足以恢复全部路由，不会自动套用。Windows 之外的完整快照断网暂不支持，会明确返回错误。

删除默认路由不等同于防火墙断网：直连网段、其他更具体路由、VPN 的 `/1` 路由或之后由系统重新生成的路由仍可能提供连接。操作期间系统路由表也无法作为一个事务冻结，当前实现通过复核、逐条处理和保留恢复记录处理变化。

停止监控会立即取消当前任务，不等待路由操作锁；正在执行的系统命令收到取消后终止，已删除部分的恢复记录保留。

## 检测失败分类

CLI 与 GUI 日志区分 `network_changed`、`proxy_rules`、`proxy_unavailable`、`network_request`、`timeout`、`api_unavailable`、`api_response`、`local_resolution`、`canceled`，避免把所有失败归为代理失效。

## 已执行检查

已通过检测器、查询器、路由、监控核心、GUI 服务和 CLI 的包测试及相关竞态回归，CLI/GUI 入口编译通过，`git diff --check` 通过。

已实际运行本地 PAC 下载/执行，验证逐网址 DIRECT/代理分流、候选顺序、错误脚本与下载中的取消；验证 HTTP 代理认证、SOCKS4/4a/5/5h 握手与 SOCKS5 认证、重定向重新选路、显式 DIRECT 回退及禁止隐式直连。路由枚举和接口核对进行了 Windows 只读运行验证，删除/恢复脚本通过 PowerShell 语法解析，恢复脚本也通过模拟 cmdlet 验证。

删除、恢复、旧网络结果、删除前再次切换及备份重试使用模拟后端测试；没有修改本机系统代理、网络或路由。真实 Wi-Fi/网线/VPN 切换、企业 WPAD 部署、动态路由恢复以及实际删除/恢复命令仍需隔离环境集成验证，不能将当前回归结果视为这些场景已全部通过。

## 原生接口依据

- [WinHttpGetIEProxyConfigForCurrentUser](https://learn.microsoft.com/en-us/windows/win32/api/winhttp/nf-winhttp-winhttpgetieproxyconfigforcurrentuser)：读取当前用户配置。
- [WinHttpGetProxyForUrlEx](https://learn.microsoft.com/en-us/windows/win32/api/winhttp/nf-winhttp-winhttpgetproxyforurlex)：逐网址异步解析、取消及完整候选列表。
- [WINHTTP_AUTOPROXY_OPTIONS](https://learn.microsoft.com/en-us/windows/win32/api/winhttp/ns-winhttp-winhttp_autoproxy_options)：自动发现与 PAC 下载认证行为。
- [Get-NetRoute](https://learn.microsoft.com/en-us/powershell/module/nettcpip/get-netroute)：读取活动及持久路由配置。
- [New-NetRoute](https://learn.microsoft.com/en-us/powershell/module/nettcpip/new-netroute)：路由协议、策略存储与有效期参数。
