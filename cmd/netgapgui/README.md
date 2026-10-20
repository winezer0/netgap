# NetGap 图形界面版

Linux 发行包新增同目录助手 `netgap-gio-proxy`；部署时保留助手并安装 GIO/glib-networking 及相应 PAC 模块。自动保护启用/启动、手动删除/恢复前会检查权限和恢复存储；失败会显示恢复快照路径。CLI 可通过 `--recovery-dir <GUI 数据目录>` 使用同一份快照。详见 [本批次记录](../../docs/gio-watch-recovery-results.md)。

图形界面版以窗口方式运行代理出口 IP 监控。界面可配置目标区域、检测间隔、IP 查询 API 和自定义 API 地址，启动或停止监控，查看运行日志，也可查询代理配置和默认路由，或手动移除、恢复默认路由。

## 构建与运行

在仓库根目录构建：

```powershell
cd cmd/netgapgui
wails build
```

在仓库根目录启动构建好的程序：

```powershell
./cmd/netgapgui/build/bin/netgapgui.exe
```

构建后的程序会加载内嵌的前端资源。

macOS 主机需要 Apple Command Line Tools、Wails、Node.js 和 CGO：

```sh
cd cmd/netgapgui
CGO_ENABLED=1 wails build -platform darwin/universal
open build/bin/netgapgui.app
```

macOS 已实现系统固定代理/PAC/WPAD、网络变化检测及双栈全网卡默认路由后端，原生行为待验证，详见 [macOS 功能支持](../../docs/macos-support.md)。

## 界面功能

- 设置检查间隔和目标区域关键词；添加或移除关键词后会保存配置。
- 选择 IP 查询 API。可用项包括 `ipconfig`、`ipify`、`seeip`、`ipapi`、`pcjson`、`ip2location` 和自定义 API。
- 填写自定义 API URL。接口需返回 JSON，格式包含 `success`、`code` 和 `data.ip` 字段；配置 URL 后才可启用该 API。
- 启动或停止监控，并查看、清除运行日志。
- 状态栏显示最近查询尝试/成功时间、连续失败、错误原因、路由动作及重试时间；长时间没有有效结果会提示。停止后保留本次观测，重新启动建立新会话。
- 查询当前代理配置和默认路由；手动移除或恢复默认路由。
- 在中文和英文界面间切换。

监控时，程序从所选 API 获取代理出口 IP，再通过本地数据库查询 IP 归属地，并与目标区域关键词匹配。检测和查询结果会显示在运行日志中。

监控检测到出口 IP 落入目标区域时，程序会自动尝试保存并删除系统默认路由；成功后监控停止。此路由操作需要管理员权限，非管理员运行时会在日志中记录操作失败，监控仍可继续。

部分断网失败后独立续作，失败和退避期间仍查询出口；新查询不会扩大旧任务。查询成功只代表本轮取得了有效结果，路由保护是否完成会单独显示。超过 30 秒或实际监控间隔的三倍（取较大者）没有成功结果时，状态栏突出提示。

## 配置与数据

配置文件名为 `netgapgui.yaml`，Windows/Linux 保存在可执行文件所在目录；macOS 保存在当前执行用户的 `~/Library/Application Support/netgapgui/`，日志和路由恢复快照也在该目录，避免写入 `.app`。界面中的设置会自动保存。程序使用内置 IPv4/IPv6 归属地数据库；如需使用自定义数据库，将 `ip2region_v4.xdb` 和 `ip2region_v6.xdb` 放入可执行文件旁的 `assets` 目录。

## 权限与网络

查询 IP 需要网络连接，且启用的外部 API 必须可访问。查询代理配置和默认路由不需要修改系统设置；移除或恢复默认路由需要管理员权限。移除默认路由会立即中断网络连接，恢复操作需要此前保存的路由信息。

macOS 修改路由要求进程有效 UID 为 0，管理员组成员身份不足以修改路由；当前没有自动提权功能。不同执行身份使用不同数据目录，恢复时必须使用原来的身份和快照目录。
