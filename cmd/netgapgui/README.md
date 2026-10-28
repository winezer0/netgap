# NetGap 图形界面版

Linux 发行包不再携带独立代理助手：支持 GNOME/KDE 无代理、固定代理和绕过规则，PAC/WPAD 模式降级为系统路由直查、仅告警，详见 [Linux 固定代理说明](../../docs/linux-proxy-support.md)。自动保护启用/启动、手动删除/恢复前会检查权限和恢复存储；失败会显示恢复快照路径。CLI 可通过 `--recovery-dir <GUI 数据目录>` 使用同一份快照。

图形界面版以窗口方式运行代理出口 IP 监控。界面可配置目标区域、检测间隔、IP 查询 API 和自定义 API 地址，启动或停止监控，查看运行日志，也可查询代理配置和默认路由，或手动移除、恢复默认路由。

## 构建与运行

在仓库根目录构建：

```powershell
cd cmd/netgapgui
node build.mjs
```

构建脚本会优先使用 `VITE_APP_VERSION`，未设置时读取 Git 标签及提交信息；没有 Git 信息时显示 `dev`。该值会编入标题栏，并同步更新构建时的 Wails 产品版本元数据。

所有 GUI 打包入口（`node build.mjs`、`wails-build.bat`、Native CI 和 Release CI）都会先执行 `npm ci` 和 `npm run check`。检查包括全目录 ESLint（不允许警告）及前端测试，任一失败立即停止打包；传入 `-s` 也不能跳过检查。直接执行普通 `wails build` 时，`frontend:build` 同样通过 `npm run build` 执行检查。CI 的检查输出保存在 GUI 构建日志中。

在仓库根目录启动构建好的程序：

```powershell
./cmd/netgapgui/build/bin/netgapgui.exe
```

构建后的程序会加载内嵌的前端资源。

macOS 主机需要 Apple Command Line Tools、Wails、Node.js 和 CGO：

```sh
cd cmd/netgapgui
CGO_ENABLED=1 node build.mjs -platform darwin/universal
open build/bin/netgapgui.app
```

macOS 已实现系统固定代理/PAC/WPAD、网络变化检测及双栈全网卡默认路由后端，原生行为待验证，详见 [macOS 功能支持](../../docs/macos-support.md)。

Linux Ubuntu 22.04/24.04 的历史完整 GUI 原生构建已通过；本次移除代理助手后的发行包需重新验收。Ubuntu 24.04 使用 WebKit 4.1 开发依赖并执行 `node build.mjs -tags webkit2_41`；Ubuntu 22.04 使用 WebKit 4.0，不再需要 PAC 模块。以下在 `cmd/netgapgui` 目录执行：

```sh
```

部署时保留助手与 GUI 相邻，使用匹配目标系统的构建产物。

## 界面功能

- 设置检查间隔和目标区域关键词；添加或移除关键词后会保存配置。
- 选择 IP 查询 API。可用项包括 `ipconfig`、`ipify`、`seeip`、`ipapi`、`pcjson`、`ip2location` 和自定义 API。
- 填写自定义 API URL。接口需返回 JSON，格式包含 `success`、`code` 和 `data.ip` 字段；配置 URL 后才可启用该 API。
- 启动或停止监控，并查看、清除运行日志。
- 状态栏显示最近查询尝试/成功时间、连续失败、错误原因、路由动作及重试时间；长时间没有有效结果会提示。停止后保留本次观测，重新启动建立新会话。
- 查询当前代理配置和默认路由；手动移除或恢复默认路由。
- 在中文和英文界面间切换。
- Windows 设置支持添加或移除当前用户登录启动项，以及配置启动后自动监测、最小化到托盘。
- Windows 窗口右上角、设置按钮左侧的重启图标提供“以管理员身份重启”：通过 UAC 授权后交接进程，保留已保存配置和监测状态；取消授权则保留当前程序。已为管理员时禁用按钮，其他平台暂不支持。实际 UAC 交接仍待实机验收。

监控时，程序从所选 API 获取代理出口 IP，再通过本地数据库查询 IP 归属地，并与目标区域关键词匹配。检测和查询结果会显示在运行日志中。

自动删除路由开启且具备权限时，监控检测到出口 IP 落入目标区域会尝试保存并删除系统默认路由；成功后监控停止。该选项默认开启，具备权限时仍需恢复存储预检。普通用户可直接启动监控并正常告警，界面明确显示自动断网因权限不足暂停；原设置与已有恢复记录保留，不自动续作断网任务。普通用户也可保存其他偏好，无需先关闭自动断网。详见[修复说明](../../docs/gui-non-admin-monitoring.md)。

部分断网失败后独立续作，失败和退避期间仍查询出口；新查询不会扩大旧任务。查询成功只代表本轮取得了有效结果，路由保护是否完成会单独显示。超过 30 秒或实际监控间隔的三倍（取较大者）没有成功结果时，状态栏突出提示。

## 配置与数据

配置文件名为 `netgapgui.yaml`，Windows/Linux 保存在可执行文件所在目录；macOS 保存在当前执行用户的 `~/Library/Application Support/netgapgui/`，日志和路由恢复快照也在该目录，避免写入 `.app`。界面中的设置会自动保存。程序使用内置 IPv4/IPv6 归属地数据库；如需使用自定义数据库，将 `ip2region_v4.xdb` 和 `ip2region_v6.xdb` 放入可执行文件旁的 `assets` 目录。

## 权限与网络

查询 IP 需要网络连接，且启用的外部 API 必须可访问。查询代理配置和默认路由不需要修改系统设置；移除或恢复默认路由需要管理员权限。移除默认路由会立即中断网络连接，恢复操作需要此前保存的路由信息。

macOS 修改路由要求进程有效 UID 为 0，管理员组成员身份不足以修改路由；当前没有自动提权功能。不同执行身份使用不同数据目录，恢复时必须使用原来的身份和快照目录。

## 尚未完成的验收

完整 GUI 编译和产物上传已通过，不代表已完成启动、渲染、托盘、关闭/重开、设置持久化或健康状态展示的桌面验收。当前没有 macOS Intel/ARM 和完整 Linux GNOME/KDE 桌面测试环境；本轮 CI 也未执行 Windows GUI 交互。

仍需验收代理退出/重启、绕过规则的实际选路、Windows/macOS PAC/WPAD、Linux 自动代理模式降级提示及自动断网暂停、切网和快速切回、原生监听故障自愈、睡眠唤醒，以及停止/重启后的旧任务隔离。真实默认路由删除/恢复、部分失败续作、并发手动/自动动作及重启恢复仅在带快照和控制台的隔离环境执行，不在当前 WSL 测试。

安装包独立下载/归档核验、macOS 签名/公证及 Gatekeeper 行为尚未验收。完整优先级、依赖限制与通过标准见 [主 README 验收清单](../../README.md#验收状态与已知限制)，执行矩阵及证据模板见 [原生验证记录](../../docs/native-desktop-validation.md#桌面验收矩阵)。
