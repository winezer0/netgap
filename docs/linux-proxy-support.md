# Linux 固定代理支持与独立助手移除

## 支持范围

Linux 已移除独立 `netgap-gio-proxy`、GIO/libproxy 解析路径及 PAC/WPAD 执行能力。CLI/GUI 的 Linux 归档只包含主程序，不再单独构建、复制或检查助手。Windows/macOS 的原生代理能力不受此改动影响。

| 来源 | 当前处理 |
|---|---|
| GNOME `mode=none` | 按现有优先级继续检查环境变量；无环境代理时直连 |
| GNOME `mode=manual` | 使用系统 `gsettings list-recursively` 的有界快照，解析 HTTP/HTTPS/SOCKS、共享代理、HTTP 认证和绕过规则 |
| KDE `ProxyType=0` 或未配置 | 按现有优先级继续检查环境变量；无环境代理时直连 |
| KDE `ProxyType=1` | 读取 XDG 配置层中的 `kioslaverc` 固定代理与绕过规则；HTTP/HTTPS 按目标协议选择，SOCKS 作为系统配置的后续候选 |
| 明确无桌面标记 | 使用 `HTTP_PROXY`、`HTTPS_PROXY`、`ALL_PROXY`、`NO_PROXY` 及现有小写变量规则 |
| GNOME `mode=auto`、KDE `ProxyType=2/3` | 不下载或执行 PAC，不使用环境代理；通过系统路由直接查询出口，持续提示降级，仅告警、暂停自动断网 |
| KDE 环境变量间接模式、未知模式/桌面、配置读取失败 | 明确报错，不猜测系统路径，不回退到其他代理来源 |

已选择的手动模式即使某个目标为 DIRECT，也不改用环境代理。PAC/WPAD 模式则明确降级为系统路由直查：检测结果可能不同于浏览器经过代理后的出口，不代表 PAC 规则已应用。命中监控地区仅告警，CLI/GUI 均继续监控、不自动删除默认路由；已保存的断网重试暂停，任务和恢复快照保留。自动动作入口会再次检查模式，避免查询前续作绕过限制。切回固定或无代理模式后恢复正常处理，旧任务仍需原有快照与状态校验。手动删除/恢复仍是用户显式操作。

界面通过 `observationOnly=linux_pac_wpad` 持续显示降级原因，健康阶段为 `observation_only`，自动保护状态为 `suspended`；请求失败仍显示查询错误并保留降级提示。日志标记 `SYSTEM_ROUTE (PAC/WPAD not applied; alerts only)`，不冒充浏览器出口。未知模式、身份/会话错误或配置读取失败仍报错，监控继续后续轮询。

## KDE 配置与边界

- 读取 `$XDG_CONFIG_HOME/kioslaverc`（默认 `$HOME/.config/kioslaverc`）及 `$XDG_CONFIG_DIRS`（默认 `/etc/xdg`）各层；低优先级先读取，用户配置最后覆盖。每层文件最多 1 MiB、最多 32 个配置根，非普通文件拒绝。
- 解析 `[Proxy Settings]` 的普通键值；支持 URL 和旧版“地址 空格 端口”的固定代理写法，支持常规 KConfig 字符串转义。泛型 `socks` 按 SOCKS5 处理。
- 支持域名后缀、显式端口、IP/CIDR、`<local>` 和 `ReversedException`；保留 KDE 后缀规则的前导点语义，不套用 Windows 的隐式 loopback 绕过。CIDR 遇到域名时使用有界 DNS 查询，取首个地址；DNS 失败报错，不能猜测代理路径。
- 不执行 PAC/WPAD（`ProxyType=2/3` 按上述降级策略处理）。不支持 `ProxyType=4` 环境变量间接配置、代理组/键的不可变/删除/本地化/环境展开标记、复杂通配符和旧非 XDG 配置布局。遇到这些配置明确拒绝，不能声称等同于所有版本的 KDE/KConfig 系统解析器。
- 每轮重读配置，指纹包含各层路径、存在状态和内容摘要；绕过、模式、地址或文件优先级发生变化会使旧配置失效，不输出代理密码明文。

实现依据：[KDE KIO 固定代理、候选与绕过处理](https://github.com/KDE/kio/blob/v5.116.0/src/core/kprotocolmanager.cpp)。当前实现范围小于完整 KIO，仍需在目标桌面验收。

## 会话与依赖

GNOME 仍需要系统 `gsettings`、桌面代理 schema 和可用的原用户会话。移除的是项目附带的独立助手和代理解析模块要求；GUI 的 GTK/WebKit 系统依赖仍保留，并可能间接依赖 GLib/GIO 等库，不能将此改动理解为 GUI 不再需要这些系统库。

sudo 且能够识别原用户时，GNOME 设置读取进程降到原 UID/GID/附属组，父进程保留路由权限；验证原用户运行目录和 D-Bus 拥有者，并使用原用户 HOME/配置目录。KDE 复用同一身份/会话验证后只读配置。身份或会话无法确认时明确报错，不改读 root 的桌面配置，不猜测其他登录用户。

## 本批检查与验收状态

2026-10-21 本批完成 Windows 下 detector、monitor、GUI 服务/调度与 CLI 编译，WSL 下 detector、monitor、GUI 调度与 CLI 编译，以及前端 `vite build`。未运行测试、未触发新原生 CI，未修改 WSL 路由、DNS、接口或代理配置；编译通过不代表运行行为已验收。

仍需验证 GNOME/KDE 自动模式的真实直查、环境代理隔离、查询失败时的持续提示、已有断网任务暂停、切回固定/无代理后的恢复，以及代理切换与自动动作并发。桌面和真实断网/恢复验收需隔离环境。历史 GIO/PAC 测试不证明这次替换已通过验收。当前验收状态统一记录在[已验证场景](已验证场景记录.md)及[剩余未验证场景](剩余未验证场景记录.md)。
