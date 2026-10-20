# GIO、监听恢复与恢复预检实施说明

本文保留 GIO 助手、监听重建和恢复预检的实现/部署说明，不维护场景状态。验证结论统一见[已验证场景记录](已验证场景记录.md)和[剩余未验证场景记录](剩余未验证场景记录.md)。

## 实现组成

- Linux 代理解析通过 GIO/libproxy 助手按网址查询；运行行为、依赖和会话身份说明见下文。
- 网络监听在初始化或运行失败时释放资源并退避重建；网络版本变化使旧查询结果失效。
- 恢复预检检查权限、快照路径、快照格式和存储可写性；实际恢复逐项核对绑定与冲突。

## Linux GIO 处理规则

- 普通 GNOME 手动模式保留 B1 的有界配置快照和逐网址规则。自动模式使用同目录 `netgap-gio-proxy`，由 GIO 的 GNOME 模块调用系统 PACRunner。KDE 使用 GIO/libproxy，要求可读取当前用户的 `kioslaverc`；不假定旧 KDE 布局受支持。
- 不再以“安装了 gsettings”推断当前使用 GNOME。明确无桌面会话标记的进程才按环境代理处理；GNOME mode=none 保留环境回退。只有 DISPLAY/Wayland 等标记但无法确定桌面的环境会明确报错，WSLg 也可能落入此边界。
- 缺助手、缺代理模块、未知桌面、来源读取/规则解析失败不作为 DIRECT。拒绝空实现或不匹配的 GIO 后端；每个 API 和重定向目标单独解析，不追加未返回的直连候选。
- 指纹包含后端、配置快照、相关环境变量及 KDE 配置文件摘要；每次调用校验与检测时的状态一致，配置已变返回状态变化错误。SHA256 摘要不包含可逆的密码明文。
- 每次启动独立助手，共用父预算，并限制为最多 3 秒、64 KiB 输出、64 个系统候选。取消会终止进程组；下一轮重新尝试，不缓存永久失败。
- sudo 启动且能识别原用户时，父进程保留路由权限，助手降到原 UID/GID/附属组，使用其 HOME、配置目录及拥有者匹配的 `/run/user/<uid>/bus`。无法确认会话或配置身份就报错；没有识别到桌面标记时不会猜测登录用户。
- 系统解析模块可能因版本差异返回不同候选；程序遵守实际返回值，不自行补齐 DIRECT。

### 构建和部署

助手是可选的独立 CGO 程序，普通 CLI/核心构建不引入 GIO 链接依赖：

```sh
CGO_ENABLED=1 go build -tags gio -trimpath -o /目标目录/netgap-gio-proxy ./cmd/netgap-gio-proxy
go build -o /目标目录/netgap ./cmd/netgap
```

构建机需要 C 编译器、pkg-config、GIO 开发文件。运行机需要 GIO、glib-networking、相应代理/PAC 模块；GNOME 还需要桌面代理 schema、gsettings 和可用的用户会话服务。安装 GIO 接口库不等于已经安装了这些模块。

Ubuntu 22.04 / libproxy 0.4 还需安装 `libproxy1-plugin-webkit` 提供 PAC JavaScript 解析；仅安装 WebKit 库并不会安装此插件。Ubuntu 24.04 / libproxy 0.5 的依赖布局不同。部署后应运行本地 PAC 集成检查确认功能，不能仅凭进程启动判断 PAC 可用。

助手必须放在应用可执行文件旁，不从 PATH 搜索。Linux 发布归档和原生构建工作流会将助手与程序一起打包。

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

## 回归命令

WSL 完整核心回归命令（桌面标记仅对该测试进程清空；各代理测试自行设置临时会话）：

```sh
env XDG_CURRENT_DESKTOP= DESKTOP_SESSION= DISPLAY= WAYLAND_DISPLAY= DBUS_SESSION_BUS_ADDRESS= \
  GOMODCACHE=/mnt/c/Program_Base/gopath/pkg/mod GOPROXY=off GOSUMDB=off \
  go test -race ./internal/netwatch ./internal/routenet ./internal/detector \
  ./internal/ipchecker ./internal/monitor ./internal/guiapp/monitoring \
  ./internal/iproute ./cmd/netgap -count=1 -timeout=180s
```

原生解析测试入口：`python3 scripts/test-gio-helper.py <助手> <glib-pacrunner>`。可设 `NETGAP_GIO_DETECTOR_TEST` 指向与助手同目录的 detector 测试二进制；`NETGAP_GIO_TEST_KDE=1` 增加 KDE/libproxy 测试。依赖、加载路径和离线 Go 缓存选项应限制在测试进程；私有 D-Bus、GNOME keyfile、KDE kioslaverc 与 PAC 服务均使用隔离测试资源。

## 验收状态

本文件不维护验证结果或剩余任务清单。当前场景分别见[已验证场景记录](已验证场景记录.md)和[剩余未验证场景记录](剩余未验证场景记录.md)。
