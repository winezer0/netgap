# NetGap

NetGap 用于检查代理出口 IP 是否仍处于预期网络区域。项目提供命令行版和图形界面版：命令行版适合脚本和终端操作，图形界面版提供可视化配置、状态和日志。

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

1. **检测代理配置**：对每个查询网址及重定向目标应用系统固定代理、PAC 和绕过规则；明确未启用系统代理时，按平台规则读取 `HTTP_PROXY`、`HTTPS_PROXY`、`ALL_PROXY`、`NO_PROXY`。读取或解析系统规则失败会报告错误，不自行增加 DIRECT；无代理或规则明确选择 DIRECT 时直接联网。
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
| 图形界面监控 | 自动保护开启且预检通过后，命中目标区域时尝试删除默认路由 | 关闭自动删除路由后可监控；启用或启动自动保护会被权限预检拒绝 |
| 图形界面手动删除/恢复默认路由 | 可执行 | 操作失败并提示权限不足 |

断网操作保存全部网卡双栈默认路由快照后执行删除，部分失败时保存任务并在监控中续作；恢复会核对网络身份。请使用管理员权限执行 `--recover` 或图形界面的恢复操作。删除默认路由无法阻断直连网段、更具体路由或系统新生成的路由。当前修复及验证边界见 [修复进展](docs/proxy-route-repair-progress.md)。

## 支持的操作系统

当前代码针对 **Windows、Linux 和 macOS** 提供平台后端，验证状态如下：

| 系统 | 当前支持情况 | 平台相关行为 |
| --- | --- | --- |
| Windows | 支持命令行版和图形界面版 | 从 Windows 系统配置逐网址应用固定代理/PAC/WPAD；完整路由操作使用 NetTCPIP PowerShell 接口。 |
| Linux | Ubuntu 22.04/24.04 amd64 原生核心测试、GIO/PAC 集成和完整 GUI 构建通过；桌面及真实路由行为待验收 | GNOME 手动规则及 GIO 系统解析助手，KDE/libproxy 支持有明确依赖边界；无桌面时使用环境代理。完整路由操作使用原生 Netlink。 |
| macOS | Intel/Apple Silicon 原生核心测试、CLI/Objective-C 后端及完整 GUI 构建通过；桌面及真实路由行为待验收 | CFNetwork 按网址解析系统固定代理/PAC/WPAD；原生路由套接字处理双栈、全网卡及接口作用域默认路由。需 CGO，详见 [macOS 功能支持](docs/macos-support.md)。 |
| 其他系统 | 未列为当前支持平台 | 完整默认路由保护在启用前明确拒绝。 |

Linux/macOS 图形界面的构建需要满足 Wails 对应平台的构建依赖。macOS 原生后端需要 Apple SDK 和 `CGO_ENABLED=1`；纯 Go 交叉编译不能替代原生功能验证。

## 验收状态与已知限制

### 已完成

Windows amd64、Ubuntu 22.04/24.04 amd64、macOS Intel/Apple Silicon 的原生核心竞态测试、CLI/GUI 构建及产物上传已通过。Linux 还通过私有 D-Bus、临时配置和本地 PAC 服务的 GIO 集成测试。这些结果不代表已完成真实桌面、切网或断网恢复验收。

验证提交为 `1466ee78ee494d741944c9cd24da41e0e0b6b8eb`，见 [成功 CI](https://github.com/winezer0/netgap/actions/runs/39407712602)、[完整验收记录](docs/native-desktop-validation.md) 和 [证据清单](docs/native-ci-evidence-2026-10-20.json)。

### 仍需验收（按优先级）

下表项目均**尚未完成验收**。当前没有 macOS 和完整 Linux GNOME/KDE 桌面测试环境，也没有可用于真实断网操作的隔离验证环境，因此保留为待执行，不标记通过。

| 优先级 | 待验收项目 | 当前限制 | 通过标准 |
| --- | --- | --- | --- |
| P0 | 真实路由删除与恢复 | 必须使用有快照和控制台的隔离 Windows/Linux/macOS 环境 | 全网卡 IPv4/IPv6 默认路由完整保存、删除、恢复；覆盖 Linux 策略表/多路径、macOS 作用域路由和 Windows 双存储副作用；部分失败、并发手动/自动操作及程序重启后仍可安全续作或恢复，冲突不覆盖、备份不丢失。 |
| P0 | 系统实际出口和中途切换 | 缺少跨平台隔离网络与桌面会话 | 无代理、固定 HTTP/HTTPS/SOCKS、绕过、PAC 下逐 URL 和重定向核对系统选路及真实请求路径；代理退出/重启、切网和 A→B→A 快速切换后继续检测，旧结果不触发路由动作。 |
| P0 | 原生监听故障自愈 | 需要平台原生故障注入，单元测试不能代替 | macOS SCDynamicStore/路由监听、Linux Netlink 监听初始化失败、退出或事件丢失后可退避重建；恢复后重新检测，无永久失效或重复监听。 |
| P1 | GUI 桌面交互及生命周期 | 缺 macOS Intel/ARM、Ubuntu GNOME/KDE 桌面；本轮 CI 也未执行 Windows GUI 交互 | 启动、渲染、托盘、关闭/重开、设置持久化、日志和健康状态正常；停止/重启不被旧任务覆盖；长时间运行及睡眠唤醒后监控仍工作。 |
| P1 | Linux 桌面来源、提权与依赖差异 | 缺真实桌面、原用户会话及多版本环境 | GNOME/KDE、X11/Wayland、sudo 原用户 D-Bus/配置来源正确；缺助手、缺模块或来源不明时明确报错；权限/目录/快照预检在实际部署中符合提示。 |
| P1 | WPAD 自动发现 | 缺受控 DNS/DHCP 和真实系统发现环境 | 验证系统发现、失效及恢复后的逐 URL 选路；已有 PAC 文件解析测试不等于 WPAD 发现已通过。 |
| P1 | 安装包部署与发行 | 大文件下载及单包重试未完成，尚未独立检查归档；没有签名/公证验收记录 | 下载对应 CI 产物，核对提交、架构、依赖和解压后可执行权限；Linux 助手须相邻且可运行，macOS `.app` 可启动。对外发行前另验证 Developer ID 签名、公证及 Gatekeeper 行为。 |

外部 API 的真实可达性、配额和限流恢复还需在目标部署网络检查；现有测试允许部分公网请求失败，不能据此认为每个 API 都可用。API 查询成功也不代表路由保护已经完成，应同时查看独立的保护状态。

### 部署与验收边界

- Linux CLI 和 GUI 均需将 `netgap-gio-proxy` 放在程序旁，并安装 GIO/glib-networking、桌面代理 schema 和对应 PAC 模块。Ubuntu 22.04 的 libproxy 0.4 需要 `libproxy1-plugin-webkit`；Ubuntu 24.04 GUI 使用 WebKit 4.1 和 `webkit2_41` 构建标签，两种产物不能假定可互换。
- KDE 的私有配置夹具测试不代表所有 KDE/libproxy 版本、旧配置布局或其他桌面均受支持；详细边界见 [GIO 实施记录](docs/gio-watch-recovery-results.md)。
- GUI 默认开启自动删除路由。普通用户应先关闭该选项再启动监控；自动保护和手动路由操作会检查权限、快照及存储。Linux/macOS 修改路由要求有效 root 权限，当前没有自动提权功能。
- `--recovery-check --recovery-dir <原快照目录>` 不修改路由，只检查权限、快照格式与恢复存储；实际恢复仍需校验网络绑定、冲突和有效期。此预检不能代替真实恢复验收。
- 删除默认路由不是完整网络隔离：直连网段、更具体的路由、VPN `/1` 路由和系统重新生成的路由仍可能保持连接。
- 真实断网/恢复及系统网络配置切换只在隔离测试机执行；当前 WSL 不执行路由删除/恢复，也不修改其网卡、DNS 或系统代理。验收步骤和证据模板见 [桌面验收矩阵](docs/native-desktop-validation.md#桌面验收矩阵)。

## 环境要求

- Go 版本：以 [go.mod](go.mod) 声明的版本为准（当前为 Go 1.25.5）。
- IP 查询需要网络连接，并依赖所启用的外部 API 可访问。
- 修改默认路由需要管理员权限；移除默认路由会中断网络连接。

## 构建

在仓库根目录执行：

```powershell
go build -o netgap.exe ./cmd/netgap
cd cmd/netgapgui && wails build
```

具体运行方式和参数见各程序的独立 README。
