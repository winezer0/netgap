# NetGap 命令行版

命令行版定时检测系统代理出口 IP，并使用本地 IP 归属地数据库判断出口是否落在配置的目标区域。查询 API 不可用时会尝试其他启用的 API。

## 构建与运行

在仓库根目录构建：

```powershell
go build -o netgap.exe ./cmd/netgap
```

构建后启动：

```powershell
./netgap.exe
```

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

API 查询 key 以 `--api-list` 输出为准，包含 `ipconfig`、`ipify`、`seeip`、`ipapi`、`pcjson`、`ip2location` 和 `customip` 等。

## 路由操作

`--shutdown`、`--shutdown-test` 和 `--recover` 涉及系统默认路由，必须以管理员权限运行。删除默认路由会导致网络中断；如需恢复，请运行 `--recover`。恢复依赖此前保存的网关信息。若恢复失败，请根据程序提示手动添加默认路由。

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
