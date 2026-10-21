# Linux 代理、监听恢复与恢复预检实施说明

本文保留 Linux 固定代理、监听重建和恢复预检的实现/部署说明，不维护场景状态。验证结论统一见[已验证场景记录](已验证场景记录.md)和[剩余未验证场景记录](剩余未验证场景记录.md)。

## 实现组成

- Linux 解析 GNOME/KDE 固定代理与绕过规则，不再使用独立助手；PAC/WPAD 降级直查系统路由出口，仅告警；详见 [Linux 固定代理说明](linux-proxy-support.md)。
- 网络监听在初始化或运行失败时释放资源并退避重建；网络版本变化使旧查询结果失效。
- 恢复预检检查权限、快照路径、快照格式和存储可写性；实际恢复逐项核对绑定与冲突。

## Linux 代理处理规则

Linux 已按新支持范围移除 GIO/libproxy 助手及 PAC/WPAD 执行能力。GNOME 使用有界 gsettings 快照，KDE 读取 XDG kioslaverc 固定代理配置。无代理、固定 HTTP/HTTPS/SOCKS 和受支持的绕过规则继续逐 URL 应用；已识别 PAC/WPAD 模式直查系统路由出口并持续提示降级，仅告警、暂停自动断网；其他不支持模式和配置错误明确报错。会话身份、配置来源和支持边界见 [Linux 固定代理说明](linux-proxy-support.md)。

## 监听恢复

- 共用 `netwatch.Watcher` 串行维护持久通知源，每次获取指纹同步排空队列。初始化失败、读取失败、截断或丢事件会关闭源，按 0.5 秒到 30 秒退避重建；失败和成功重建都推进版本。只有重建、排空与完整快照成功后才能取得有效指纹。
- Linux 订阅链路、IPv4/IPv6 地址、路由、策略规则。短报文、尾部不完整数据、NLMSG_OVERRUN/ERROR 都判失败；普通流量产生的 cloned 路由不计为配置变化。
- macOS 路由订阅移除 sync.Once，使用持久非阻塞 socket 和上述重建机制；ARP/NDP、连接克隆路由仍被过滤。
- SCDynamicStore 移除 pthread_once，启动失败/线程退出按 1–30 秒退避重试，释放 RunLoop/source/store。快照读取失败会停止旧 RunLoop，等待退出后重建；读取期间版本改变则拒绝快照。
- 不能承诺中断已经挂起的 macOS 同步框架调用。启动线程不返回时不会无限创建新线程，仍报告失败；这项限制未因本轮重建修复而消失。

## 恢复入口和预检

CLI 启用保护/恢复，GUI 启用或启动自动保护，以及手动路由操作前，检查平台能力、权限、快照和存储。快照格式损坏时拒绝覆盖；预检只用独立临时文件验证同步写入和重命名，不替换恢复数据。

```sh
netgap --recovery-check --recovery-dir /原快照目录
netgap --recover --recovery-dir /原快照目录
```

第一条不修改路由，但会检查/创建快照目录、锁文件和临时存储探针。第二条会执行恢复，只能在明确要恢复时运行。GUI 目录可作为 CLI 的 `--recovery-dir`；实际快照名称仍为 `iproute.cache.json`。失败信息显示路径，要求保留剩余数据并使用原身份/目录或有相应访问权的管理员/root。

存储预检成功不代表所有路由均可恢复：网络绑定、冲突和动态有效期仍由实际恢复逐条检查；继续保留原有部分恢复和失败备份机制。权限、网络、文件状态也可能在预检后改变，实际操作仍需再次检查。

## 回归命令

WSL 完整核心回归命令（桌面标记仅对该测试进程清空；各代理测试自行设置临时会话）：

```sh
env XDG_CURRENT_DESKTOP= DESKTOP_SESSION= DISPLAY= WAYLAND_DISPLAY= DBUS_SESSION_BUS_ADDRESS= \
  GOMODCACHE=/mnt/c/Program_Base/gopath/pkg/mod GOPROXY=off GOSUMDB=off \
  go test -race ./internal/netwatch ./internal/routenet ./internal/detector \
  ./internal/ipchecker ./internal/monitor ./internal/guiapp/monitoring \
  ./internal/iproute ./cmd/netgap -count=1 -timeout=180s
```

原 GIO/PAC 私有会话测试入口随助手移除。历史验收结果仅适用于旧实现；固定代理替换后的回归与桌面验收需重新执行。

## 验收状态

本文件不维护验证结果或剩余任务清单。当前场景分别见[已验证场景记录](已验证场景记录.md)和[剩余未验证场景记录](剩余未验证场景记录.md)。
