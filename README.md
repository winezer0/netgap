# NetGap

NetGap 用于检查代理出口 IP 是否仍处于预期网络区域。

项目提供命令行版和图形界面版：命令行版适合脚本和终端操作，图形界面版提供可视化配置、状态和日志。


## 免责声明
继续阅读文章或使用工具视为您已同意 [NOVASEC免责声明](https://mp.weixin.qq.com/s/iRWRVxkYu7Fx5unxA34I7g)

<img width="600" height="800" alt="image" src="https://github.com/user-attachments/assets/dcb0e156-7d68-4c62-b634-7cdc7634df21" />

## 程序说明

- [命令行版（`cmd/netgap`）](cmd/netgap/README.md)：按设定间隔查询代理出口 IP，匹配目标区域，并可选择在检测到目标区域时移除默认路由。
- [图形界面版（`cmd/netgapgui`）](cmd/netgapgui/README.md)：配置检测区域、间隔和查询 API，查看监控日志，并手动查询或操作默认路由。

两个程序都会检测系统代理配置，并依次使用可用的 IP 查询 API 获取出口 IP，再通过本地 IPv4/IPv6 数据库查询归属地。可在可执行文件旁的 `assets` 目录提供自定义数据库；未提供时使用程序内置数据库。

## 支持的 IP 查询 API

| API 名称 | 地址 | curl 命令 | 响应示例（程序读取字段） | 频率 |
| --- | --- | --- | --- | --- |
| `ipconfig` | [ipconfig.io](https://ipconfig.io/json) | `curl.exe -s https://ipconfig.io/json` | `{"ip":"<出口IP>"}` | 未公布固定限额；[官方建议每 15–30 分钟查询](https://docs.ipconfig.io/guides/get-your-public-ip-in-powershell) |
| `ipify` | [api.ipify.org](https://api.ipify.org?format=json) | `curl.exe -s "https://api.ipify.org?format=json"` | `{"ip":"<出口IP>"}` | [官方称不限量](https://www.ipify.org/) |
| `seeip` | [api.seeip.org](https://api.seeip.org/jsonip) | `curl.exe -s https://api.seeip.org/jsonip` | `{"ip":"<出口IP>"}` | [官方称无实际次数限制](https://seeip.org/) |
| `ipapi` | [ip-api.com](http://ip-api.com/json?fields=query) | `curl.exe -s "http://ip-api.com/json?fields=query"` | `{"query":"<出口IP>"}` | [免费接口最多 45 次/分钟](https://ip-api.com/docs/legal)；超限可能临时封禁 |
| `pcjson` | [PConline](https://whois.pconline.com.cn/ipJson.jsp) | `curl.exe -s https://whois.pconline.com.cn/ipJson.jsp` | `IPCallBack({"ip":"<出口IP>", ...});` | 未查到官方公开的请求限额；实际限制以服务端为准 |
| `ip2location` | [api.ip2location.io](https://api.ip2location.io/) | `curl.exe -s https://api.ip2location.io/` | `{"ip":"<出口IP>", ...}` | 本项目当前使用免密请求：[最多 1,000 次/日](https://www.ip2location.io/ip2location-documentation) |
| `customip` | [winezer0/ipinfo（自部署参考）](https://github.com/winezer0/ipinfo) | `curl.exe -s "http://<主机>:<端口>/ip?token=<令牌>"` | `{"success":true,"code":0,"data":{"ip":"<出口IP>"}}` | 由自部署服务的配置和资源决定 |

响应示例仅展示程序读取的字段；`<出口IP>`、主机、端口和令牌需替换为实际值。自定义 API 的 URL 在图形界面中配置。

以上为服务商公开的限额或建议，未公布限额不代表没有限制。程序默认每 3 秒检查一次；选择有配额的 API 时，请按其额度调大监测间隔或停用该 API。服务商策略可能变更，请以其最新文档为准。

## 工作原理

1. **检测代理配置**：对每个查询网址及重定向目标应用系统固定代理和绕过规则；Windows/macOS 另支持 PAC/WPAD，Linux 自动代理模式仅查询系统路由出口，显示降级提示并暂停自动断网。明确未启用系统代理时，按平台规则读取 `HTTP_PROXY`、`HTTPS_PROXY`、`ALL_PROXY`、`NO_PROXY`。读取、解析失败或模式不支持会报告错误，不自行增加 DIRECT；无代理或规则明确选择 DIRECT 时直接联网。
2. **轮询 IP 查询 API**：根据配置筛选 API，并按 API key 排序。每轮检查会轮换起始 API；从该 API 开始依次尝试，取得首个有效 IP 后结束本轮查询。网络错误不处罚 API；API 自身连续失败超过 10 次后冷却 30 秒再试，不再永久移除查询器。
3. **查询 IP 归属地**：使用程序内置或 `assets` 目录中的本地 IPv4/IPv6 数据库解析出口 IP。
4. **匹配目标区域**：将解析出的省份和城市与配置的区域关键词做包含匹配。匹配到关键词时，判定出口落入目标区域，并记录为本地出口。
5. **处理本地出口**：命令行版默认只记录告警；启用 `--shutdown` 后，会保存当前默认路由并尝试删除。图形界面版命中目标区域时会自动尝试保存并删除默认路由，成功后停止监控。

API 选择规则：命令行指定 `--api-use` 时只使用指定 API；未指定时使用全部 API（可用 `--api-ban` 排除）。图形界面版使用配置中勾选的 API。

## 管理员与非管理员

IP 查询、区域匹配、日志查看等监控功能通常无需管理员权限。读取代理配置和查询默认路由也属于只读操作。修改系统默认路由需要管理员权限（Windows 管理员权限，Linux/macOS 有效 root 权限）。

| 操作 | 管理员运行 | 非管理员运行 |
| --- | --- | --- |
| 命令行普通监控 | 可监控；默认只告警，不修改路由 | 可监控；默认只告警，不修改路由 |
| 命令行 `--shutdown` | 保存默认路由；目标区域命中时删除默认路由 | 启动时提示需要管理员权限并退出 |
| 命令行 `--shutdown-test` / `--recover` | 可删除路由或恢复已保存的路由 | 提示需要管理员权限，不执行操作 |
| 图形界面监控 | 自动保护开启且预检通过后，命中目标区域时尝试删除默认路由 | 正常监控及告警；自动断网因权限不足暂停，明确提示且保留原设置，不续作已保存的断网任务 |
| 图形界面手动删除/恢复默认路由 | 可执行 | 操作失败并提示权限不足 |

断网操作保存全部网卡双栈默认路由快照后执行删除，部分失败时保存任务并在监控中续作；恢复会核对网络身份。请使用管理员权限执行 `--recover` 或图形界面的恢复操作。删除默认路由无法阻断直连网段、更具体路由或系统新生成的路由。实现说明见[系统代理与路由说明](docs/system-proxy-routing.md)，验证场景见[已验证场景记录](docs/已验证场景记录.md)和[剩余未验证场景记录](docs/剩余未验证场景记录.md)。

## 支持的操作系统

当前代码针对 **Windows、Linux 和 macOS** 提供平台后端，验证状态如下：

| 系统 | 当前支持情况 | 平台相关行为 |
| --- | --- | --- |
| Windows | 支持命令行版和图形界面版 | 从 Windows 系统配置逐网址应用固定代理/PAC/WPAD；完整路由操作使用本机原生 CIM/MI 接口，运行验收待完成。 |
| Linux | 支持 Ubuntu 22.04/24.04 amd64 CLI 和 GUI | GNOME/KDE 无代理、固定 HTTP/HTTPS/SOCKS 与绕过规则；不执行 PAC/WPAD，识别到自动模式后直接查询系统路由出口，仅告警并暂停自动断网。无桌面时使用环境代理，不再携带独立代理助手。完整路由操作使用原生 Netlink。 |
| macOS | 支持 Intel/Apple Silicon CLI 和 GUI | CFNetwork 按网址解析系统固定代理/PAC/WPAD；原生路由套接字处理双栈、全网卡及接口作用域默认路由。需 CGO，详见 [macOS 功能支持](docs/macos-support.md)。 |
| 其他系统 | 未列为当前支持平台 | 完整默认路由保护在启用前明确拒绝。 |

Linux/macOS 图形界面的构建需要满足 Wails 对应平台的构建依赖。macOS 原生后端需要 Apple SDK 和 `CGO_ENABLED=1`；纯 Go 交叉编译不能替代原生功能验证。

## GUI 登录启动

- 在“程序设置”中点击“添加系统启动项”，注册 Windows 当前用户登录启动，可在任务管理器“启动应用”中管理。移除启动项立即生效；程序移动后可在设置中更新路径。
- 设置中依次显示“网络匹配弹框”“自动开启检测”“启动时最小化”。后两项保存后在下次启动生效（包括手动启动），默认均关闭。勾选“启动时最小化”后窗口初始隐藏到托盘，Wails 和托盘就绪后再应用启动偏好；托盘超时未就绪或自动监测启动失败时显示主窗口。
- 启动项无需管理员权限，不自动提权；任务管理器中的禁用状态不会被覆盖。Linux/macOS 暂不支持系统启动项注册。
- 启动项通过当前用户 Run 注册表键实现，属于登录启动，详见 [Microsoft 启动应用说明](https://support.microsoft.com/en-us/windows/experience/startup-boot/configure-startup-applications-in-windows)。自动化测试使用独立注册表键；任务管理器显示和重新登录启动仍需实机验收。

## GUI 管理员重启

- Windows：点击窗口右上角、设置按钮左侧的重启图标，通过系统 UAC 申请管理员权限并重启。取消或启动失败时保留当前程序；已为管理员时按钮禁用。Linux/macOS 暂不支持此按钮。
- 授权后启动同一程序，新进程等待旧进程退出后再创建界面，避免单实例锁拦截。使用已保存的配置，恢复重启前的监测状态并显示主窗口；未保存的设置不会应用，登录启动项及启动偏好不变。
- 实现采用 [Windows Shell 的 runas 操作](https://learn.microsoft.com/en-us/windows/win32/shell/launch)。UAC 取消/批准及单实例交接仍需实机验收；构建检查不会自动触发权限申请或重启当前程序。

## 验证记录

- 已验证场景与证据：[已验证场景记录](docs/已验证场景记录.md)
- 尚未验证场景与验收要求：[剩余未验证场景记录](docs/剩余未验证场景记录.md)

### 部署与验收边界

- Windows 原生路由修复已接入本机 CIM/MI 接口，移除完整路由操作的 PowerShell 子进程，并调整变化判定及失败退避。当前通过编译，尚未完成原生只读与隔离断网/恢复验收；Windows 路由保护面向 amd64/arm64，旧恢复快照可能不兼容。详见 [实施计划与验收清单](docs/windows-native-route-repair.md)。

- Linux 代理检测不再要求 `netgap-gio-proxy`、libproxy 或 PAC 模块；GNOME 使用系统 `gsettings` 和桌面代理 schema，KDE 直接读取 XDG 配置目录中的 `kioslaverc`。GUI 仍需 GTK/WebKit 及其系统库；Ubuntu 24.04 使用 WebKit 4.1 和 `webkit2_41` 构建标签，两种产物不能假定可互换。
- Linux PAC/WPAD 不执行自动规则，改为系统路由直查（结果可能不同于浏览器代理出口），持续提示降级，仅告警；不使用环境代理，暂停新断网及已保存的断网重试并保留恢复数据。切回固定或无代理模式后恢复正常处理。KDE 环境变量间接模式及高级 KConfig 代理选项仍明确报错并继续后续复查。详细边界见 [Linux 固定代理说明](docs/linux-proxy-support.md)。历史 GIO/PAC 验收不能证明新实现已验收。
- GUI 默认开启自动删除路由。普通用户可直接启动监控，仅告警并明确提示自动断网因权限不足暂停，原设置保留。具备权限时自动保护仍需快照与存储预检；手动删除/恢复仍要求权限。Linux/macOS 修改路由要求有效 root 权限，当前没有自动提权功能。详见[普通用户监测修复说明](docs/gui-non-admin-monitoring.md)。
- `--recovery-check --recovery-dir <原快照目录>` 不修改路由，只检查权限、快照格式与恢复存储；实际恢复仍需校验网络绑定、冲突和有效期。此预检不能代替真实恢复验收。
- 删除默认路由不是完整网络隔离：直连网段、更具体的路由、VPN `/1` 路由和系统重新生成的路由仍可能保持连接。
- 真实断网/恢复及系统网络配置切换只在隔离测试机执行。验收步骤和证据模板见 [桌面验收执行指南](docs/native-desktop-validation.md)。

## 环境要求

- Go 版本：以 [go.mod](go.mod) 声明的版本为准（当前为 Go 1.25.5）。
- IP 查询需要网络连接，并依赖所启用的外部 API 可访问。
- 修改默认路由需要管理员权限；移除默认路由会中断网络连接。

## 构建

在仓库根目录执行：

```powershell
go build -o netgap.exe ./cmd/netgap
cd cmd/netgapgui && node build.mjs
```

具体运行方式和参数见各程序的独立 README。
