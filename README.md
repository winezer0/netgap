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

1. **检测代理配置**：优先读取系统代理；未检测到可用系统代理时，再读取 `HTTP_PROXY`、`HTTPS_PROXY`、`ALL_PROXY` 环境变量。未检测到代理时，查询 API 会直接联网。
2. **轮询 IP 查询 API**：根据配置筛选 API，并按 API key 排序。每轮检查会轮换起始 API；从该 API 开始依次尝试，遇到请求失败、空响应或无效 IP 时回退到下一个，取得首个有效 IP 后结束本轮查询。因此不是每轮都会请求所有 API。连续失败超过 10 次的 API 会在当前进程中停用，重启程序后重新启用。
3. **查询 IP 归属地**：使用程序内置或 `assets` 目录中的本地 IPv4/IPv6 数据库解析出口 IP。
4. **匹配目标区域**：将解析出的省份和城市与配置的区域关键词做包含匹配。匹配到关键词时，判定出口落入目标区域，并记录为本地出口。
5. **处理本地出口**：命令行版默认只记录告警；启用 `--shutdown` 后，会保存当前默认路由并尝试删除。图形界面版命中目标区域时会自动尝试保存并删除默认路由，成功后停止监控。

API 选择规则：命令行指定 `--api-use` 时只使用指定 API；未指定时使用全部 API（可用 `--api-ban` 排除）。图形界面版使用配置中勾选的 API。

## 管理员与非管理员

IP 查询、区域匹配、日志查看等监控功能通常无需管理员权限。读取代理配置和查询默认路由也属于只读操作。修改系统默认路由需要管理员权限（Windows 管理员权限，Linux root 权限）。

| 操作 | 管理员运行 | 非管理员运行 |
| --- | --- | --- |
| 命令行普通监控 | 可监控；默认只告警，不修改路由 | 可监控；默认只告警，不修改路由 |
| 命令行 `--shutdown` | 保存默认路由；目标区域命中时删除默认路由 | 启动时提示需要管理员权限并退出 |
| 命令行 `--shutdown-test` / `--recover` | 可删除路由或恢复已保存的路由 | 提示需要管理员权限，不执行操作 |
| 图形界面监控 | 可监控；命中目标区域时自动尝试删除默认路由 | 可监控；命中目标区域时删除路由会失败并记录错误 |
| 图形界面手动删除/恢复默认路由 | 可执行 | 操作失败并提示权限不足 |

删除默认路由会中断网络连接。恢复路由依赖程序此前保存的网关信息；请在具备管理员权限的情况下使用 `--recover` 或图形界面的恢复操作。

## 支持的操作系统

当前代码针对 **Windows 和 Linux** 提供系统代理检测与路由操作实现：

| 系统 | 当前支持情况 | 平台相关行为 |
| --- | --- | --- |
| Windows | 支持命令行版和图形界面版 | 从 Windows Internet 设置读取系统代理；路由操作使用系统 `route` 命令。 |
| Linux | 支持命令行版和图形界面版 | 可从 GNOME 设置读取手动代理，也可使用代理环境变量；路由操作使用 `ip route` 命令。 |
| macOS 及其他系统 | 未列为当前支持平台 | 可尝试通过代理环境变量查询；系统代理检测和默认路由操作未实现或未经验证。 |

Linux 上图形界面的构建还需要满足 Wails 对应平台的构建依赖。其他系统即使能够编译或启动，也不代表代理检测和路由操作完整可用。

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
