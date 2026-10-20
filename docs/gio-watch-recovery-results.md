# GIO、监听恢复与恢复预检实施记录

日期：2026-10-20。当前工作区未提交。Windows/WSL Go 工具链为 1.26.4；WSL 为 Ubuntu、普通用户 UID 1000。本文件的最新状态覆盖此前计划中的 B2/C/D 待实施描述。

## 交付状态

| 任务 | 本轮结果 | 验证边界 |
| --- | --- | --- |
| B2 Linux PAC 与来源识别 | 已接入独立 GIO 助手；GNOME 自动模式、KDE/libproxy 逐网址解析，保留候选顺序 | GNOME/KDE 固定代理、PAC、绕过已有 WSL 原生证据；WPAD 发现未验证 |
| C macOS 监听自愈 | 已修改路由 socket 和 SCDynamicStore 的失败重建路径 | 共用状态机测试通过；Objective-C 和 macOS 原生运行尚未验证 |
| D Linux 短暂切网检测 | 持久 Netlink socket 记录接口/地址/路由/策略规则事件，捕获 A→B→A | 合成报文、失败退避/重建与只读订阅测试通过；未实际切换 WSL 网络 |
| 调度故障回归 | CLI/GUI 续作失败及退避仍查询；健康查询错误与路由错误分离；旧动作不能覆盖新动作 | Windows、WSL 回归通过；真实路由并发行为仍需隔离环境 |
| 恢复预检 | 权限、实际快照路径、现有快照格式、可写性、同步写入和重命名检查 | 临时目录测试通过；实际恢复仍逐条验证网络、冲突和有效期 |
| 原生构建 | Windows 桌面 GUI、WSL CLI/核心/GIO 助手通过；CI 增加助手与私有会话测试 | 完整 Linux GUI 缺构建依赖；没有 macOS 主机，不能标为全部原生验证完成 |

## Linux GIO 处理规则

- 普通 GNOME 手动模式保留 B1 的有界配置快照和逐网址规则。自动模式使用同目录 `netgap-gio-proxy`，由 GIO 的 GNOME 模块调用系统 PACRunner。KDE 使用 GIO/libproxy，要求可读取当前用户的 `kioslaverc`；不假定旧 KDE 布局受支持。
- 不再以“安装了 gsettings”推断当前使用 GNOME。明确无桌面会话标记的进程才按环境代理处理；GNOME mode=none 保留环境回退。只有 DISPLAY/Wayland 等标记但无法确定桌面的环境会明确报错，WSLg 也可能落入此边界。
- 缺助手、缺代理模块、未知桌面、来源读取/规则解析失败不作为 DIRECT。拒绝空实现或不匹配的 GIO 后端；每个 API 和重定向目标单独解析，不追加未返回的直连候选。
- 指纹包含后端、配置快照、相关环境变量及 KDE 配置文件摘要；每次调用校验与检测时的状态一致，配置已变返回状态变化错误。SHA256 摘要不包含可逆的密码明文。
- 每次启动独立助手，共用父预算，并限制为最多 3 秒、64 KiB 输出、64 个系统候选。取消会终止进程组；下一轮重新尝试，不缓存永久失败。
- sudo 启动且能识别原用户时，父进程保留路由权限，助手降到原 UID/GID/附属组，使用其 HOME、配置目录及拥有者匹配的 `/run/user/<uid>/bus`。无法确认会话或配置身份就报错。**实际 sudo 降权和不同发行版会话布局尚未实机验证**；没有识别到桌面标记时不会猜测登录用户。
- 系统解析模块也有版本差异：本次 libproxy 0.5.4 在某些多候选结果中没有返回脚本末尾的 DIRECT。程序遵守实际返回值，不自行补齐；纯 DIRECT 和绕过结果已验证。

### 构建和部署

助手是可选的独立 CGO 程序，普通 CLI/核心构建不引入 GIO 链接依赖：

```sh
CGO_ENABLED=1 go build -tags gio -trimpath -o /目标目录/netgap-gio-proxy ./cmd/netgap-gio-proxy
go build -o /目标目录/netgap ./cmd/netgap
```

构建机需要 C 编译器、pkg-config、GIO 开发文件。运行机需要 GIO、glib-networking、相应代理/PAC 模块；GNOME 还需要桌面代理 schema、gsettings 和可用的用户会话服务。安装 GIO 接口库不等于已经安装了这些模块。

助手必须放在应用可执行文件旁，不从 PATH 搜索。Linux 发布归档和原生构建工作流已加入助手；工作流新增原生助手和私有桌面测试，尚未提交或触发 CI。

依据：[GIO 解析接口](https://docs.gtk.org/gio/iface.ProxyResolver.html)、[GNOME 解析实现](https://github.com/GNOME/glib-networking/blob/master/proxy/gnome/gproxyresolvergnome.c)、[libproxy KDE 配置来源](https://github.com/libproxy/libproxy/blob/main/src/backend/plugins/config-kde/config-kde.c)。

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

## 已执行验证

| 验证 | 结果 |
| --- | --- |
| Windows `go test ./... -count=1 -timeout=180s` | 通过 |
| Windows 核心调度/快照竞态回归 | 通过 |
| WSL 下方八个包全部测试，启用 `-race` | 通过，无竞态报告 |
| WSL GIO/监听/预检/健康/续作专项重复 3 次 | 通过；最终完整回归补齐续作即时失败分支 |
| 原生 GIO + 私有 D-Bus + 临时 keyfile + 本地 PAC 服务 | GNOME、KDE 固定代理与 PAC、HTTP/HTTPS、绕过、候选顺序、服务退出/恢复通过 |
| 应用 detector → 同目录原生助手 → 系统 PACRunner | 通过；配置改变后旧规则返回 ErrStateChanged |
| WSL `go build ./internal/...`、CLI 原生构建、`-tags gio` 助手原生构建 | 通过 |
| Windows `go build -tags desktop,production ... ./cmd/netgapgui` | 通过；未重新生成前端和安装包 |
| Darwin amd64/arm64 CLI 交叉构建，CGO=0 | 通过；只覆盖 Go 路由代码，不验证 Objective-C，不是完整功能版本 |
| WSL Linux GUI `-tags desktop,production` | 失败：缺 gtk+-3.0、gio-unix-2.0、webkit2gtk-4.0 开发依赖；未安装系统包来绕过该限制 |
| `git diff --check` | 通过 |

WSL 完整核心回归命令（桌面标记仅对该测试进程清空；各代理测试自行设置临时会话）：

```sh
env XDG_CURRENT_DESKTOP= DESKTOP_SESSION= DISPLAY= WAYLAND_DISPLAY= DBUS_SESSION_BUS_ADDRESS= \
  GOMODCACHE=/mnt/c/Program_Base/gopath/pkg/mod GOPROXY=off GOSUMDB=off \
  go test -race ./internal/netwatch ./internal/routenet ./internal/detector \
  ./internal/ipchecker ./internal/monitor ./internal/guiapp/monitoring \
  ./internal/iproute ./cmd/netgap -count=1 -timeout=180s
```

原生解析复现入口：`python3 scripts/test-gio-helper.py <助手> <glib-pacrunner>`。可设 `NETGAP_GIO_DETECTOR_TEST` 指向与助手同目录的 detector 测试二进制；`NETGAP_GIO_TEST_KDE=1` 增加 KDE/libproxy 测试。本次 WSL 使用下载并解包到 `/tmp/netgap-gio-deps` 的依赖，未运行 apt install、sudo 或修改系统安装状态；加载路径和离线 Go 缓存选项均仅作用于子进程。

测试没有执行真实路由删除/恢复、网卡切换、DNS 修改或系统代理修改。私有 D-Bus、GNOME keyfile 和 KDE kioslaverc 均在临时目录；PAC HTTP 服务仅监听 loopback。前后只读摘要完全一致：地址、DNS、GNOME 真实代理、IPv4/IPv6 路由、IPv4/IPv6 策略规则及 UID。既有 WSL localhost/NAT 代理提示未作调整。已有公网 API 测试允许网络失败，不能把整套测试通过理解为所有公网 API 均可达。

## 剩余验证任务

1. 在 macOS amd64/arm64 执行 CGO/Objective-C 和完整 GUI 原生构建，验证初始化失败、监听退出、恢复后的版本及资源释放；当前没有可用 macOS 主机。
2. 在具备 GTK/WebKit 开发环境的 Linux 完成桌面 GUI 构建；现有 CI 以 Ubuntu 22.04/WebKit 4.0 为目标，本次 Ubuntu 24.04 WSL 缺对应依赖。不同 WebKit 目标须分别验证。
3. 隔离桌面环境验证 sudo 原用户会话、KDE/libproxy 不同版本与缺模块行为、WPAD 发现、实际 A→B→A 切网和队列溢出后的完整恢复。
4. 仅在隔离 Windows/Linux/macOS 环境测试双栈、多网卡真实断网/恢复、Windows 双存储副作用、并发手动/自动动作及中途失败。当前用户 WSL 不承担这类测试。
