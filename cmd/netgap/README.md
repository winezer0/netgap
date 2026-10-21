# NetGap 命令行版

Linux 无需独立代理助手：支持 GNOME/KDE 无代理、固定代理和绕过规则，不执行 PAC/WPAD，识别到自动模式后直接查询系统路由出口，仅告警并暂停自动断网。GNOME 需要系统 `gsettings` 与桌面代理 schema，KDE 读取 `kioslaverc`；支持边界见 [Linux 固定代理说明](../../docs/linux-proxy-support.md)。

恢复前可执行 `netgap --recovery-check --recovery-dir <原快照目录>`，检查权限、快照格式和存储，不修改路由；确认目录后使用 `--recover --recovery-dir <原快照目录>`。实际恢复仍逐条校验网络绑定、冲突和有效期；失败时保留恢复数据。需要原执行身份或具有该目录访问权的管理员/root。

命令行版定时检测系统代理出口 IP，并使用本地 IP 归属地数据库判断出口是否落在配置的目标区域。查询 API 不可用时会尝试其他启用的 API。

每轮日志包含 `Monitor health` 摘要：最近尝试/成功时间、连续查询失败、查询错误、路由保护状态和重试时间。部分断网续作失败或退避时仍查询出口，新结果不扩大旧任务；只有断网确认完成后才按 `--shutdown` 停止监控。

## 构建与运行

在仓库根目录构建：

```powershell
go build -o netgap.exe ./cmd/netgap
```

构建后启动：

```powershell
./netgap.exe
```

macOS 请在本机安装 Apple Command Line Tools 后原生构建，不能使用 `CGO_ENABLED=0` 部署完整功能：

```sh
CGO_ENABLED=1 go build -o netgap ./cmd/netgap
./netgap --help
```

macOS 已增加 CFNetwork 系统代理/PAC/WPAD 和原生双栈默认路由后端，原生行为待验证，详见 [macOS 功能支持](../../docs/macos-support.md)。

Linux 只需构建 CLI，不再构建或部署 GIO 助手：

```sh
go build -o netgap ./cmd/netgap
```

Linux 自动代理模式直接查询系统路由出口（可能不同于浏览器代理出口），持续提示降级，仅告警、不自动断网；已保存的断网重试暂停且保留恢复数据。切回固定或无代理模式后恢复正常处理。

查看帮助：

```powershell
./netgap.exe --help
```

示例：

```powershell
# 每 5 秒检查一次，匹配指定区域
./netgap.exe --regions 北京,上海 --interval 5

# 只使用指定 API；API key 可通过 --api-list 查看
./netgap.exe --api-use ipify,ipconfig

# 查看全部 API key 和名称
./netgap.exe --api-list

# 排除指定 API，其余 API 仍可用于轮询和失败回退
./netgap.exe --api-ban ipapi,customip

# 将日志写入文件并启用调试级别
./netgap.exe --lf ./netgap.log --ll debug
```

## 参数

| 参数 | 说明 |
| --- | --- |
| `-R, --regions` | 目标区域关键词，可用逗号分隔多个值；匹配归属地中的省份或城市。 |
| `--interval` | 检查间隔，单位为秒，默认值为 3。 |
| `--api-use` | 只使用指定的 IP 查询 API key，多个值用逗号分隔。 |
| `--api-ban` | 排除指定的 IP 查询 API key，多个值用逗号分隔。 |
| `--api-list` | 列出当前支持的 API key 和名称。 |
| `--lf` | 日志文件路径；不指定时只输出到控制台。 |
| `--ll` | 日志级别：`debug`、`info`、`warn` 或 `error`。 |
| `--lc` | 控制台日志格式，可组合 `T`（时间）、`L`（级别）、`C`（调用位置）、`F`（函数）、`M`（消息）；设为空或 `off` 可关闭控制台日志。 |
| `-s, --shutdown` | 启用自动断网：检测到出口 IP 位于目标区域时删除默认路由，并在监控前保存路由信息。 |
| `-S, --shutdown-test` | 立即保存并删除默认路由，用于测试。 |
| `-r, --recover` | 根据已保存的路由信息恢复默认路由。 |
| `--recovery-dir` | 指定恢复快照目录；可指定原 GUI 数据目录，须保持原快照及执行身份一致。 |
| `--recovery-check` | 检查权限、快照格式和存储后退出，不修改路由；不保证实际网络绑定、冲突和有效期校验必然通过。 |

API 查询 key 以 `--api-list` 输出为准，包含 `ipconfig`、`ipify`、`seeip`、`ipapi`、`pcjson`、`ip2location` 和 `customip` 等。

## 路由操作

`--shutdown`、`--shutdown-test` 和 `--recover` 涉及系统默认路由，必须以管理员权限运行。删除默认路由会导致网络中断；如需恢复，请运行 `--recover`。恢复依赖此前保存的网关信息。若恢复失败，请根据程序提示手动添加默认路由。

macOS 需要有效 root 权限。CLI 快照位于当前执行用户的 `~/.cache/netgap/iproute.cache.json`，GUI 使用单独目录；删除和恢复必须保持相同入口、身份及目录。例如隔离环境中均使用 `sudo -H ./netgap`，分别传入 `--shutdown` 和 `--recover`。

```powershell
# 管理员终端：命中目标区域时自动删除默认路由
./netgap.exe --shutdown --regions 北京,上海

# 管理员终端：立即测试删除默认路由（会中断网络）
./netgap.exe --shutdown-test

# 管理员终端：根据保存的网关信息恢复默认路由
./netgap.exe --recover
```

## IP 数据库

程序使用内置 IPv4/IPv6 归属地数据库。也可将 `ip2region_v4.xdb` 和 `ip2region_v6.xdb` 放入可执行文件旁的 `assets` 目录供程序使用。

## 尚未完成的验收

历史五平台原生构建和核心竞态测试已通过；移除 Linux GIO 后的固定代理实现需重新验收，历史 PAC 结果不适用于当前 Linux 代码。真实代理/切网端到端行为、Windows/macOS WPAD 发现、原生监听故障恢复，以及双栈、多网卡的完整断网/恢复仍有待验收场景。Linux sudo 原用户桌面会话和 KDE 配置差异也需要实际部署验证。

当前缺少相应隔离环境；真实路由操作不能在日常主机或当前 WSL 上补测。安装包独立下载/归档核验尚未完成；外部 API 可达性及配额需在部署网络检查。剩余项目、限制与通过标准见 [主 README 验收清单](../../README.md#验收状态与已知限制)，已有证据见 [原生验证记录](../../docs/native-desktop-validation.md)。
