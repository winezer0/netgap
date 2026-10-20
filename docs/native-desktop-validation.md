# 原生构建与桌面验收记录

## 范围

原生 CI 验证原生工具链、核心竞态测试、Linux GIO/PAC 集成、完整 Wails GUI 编译和安装包归档。CI 成功不代表交互式桌面或真实路由操作验收成功。

工作流：`.github/workflows/native-build.yml`。本轮使用独立验证分支，未合并到 main，未发布 Release。

## 构建矩阵

| Runner | 平台 | GUI 依赖 / 标签 | 安装包 Artifact |
|---|---|---|---|
| windows-latest | windows/amd64 | WebView2 | native-windows-amd64 |
| ubuntu-22.04 | linux/amd64 | GTK 3、WebKit 4.0、GIO、libproxy1-plugin-webkit | native-ubuntu-22.04-amd64 |
| ubuntu-24.04 | linux/amd64 | GTK 3、WebKit 4.1、GIO；webkit2_41 | native-ubuntu-24.04-amd64 |
| macos-15-intel | darwin/amd64 | Xcode / macOS 原生框架 | native-macos-intel |
| macos-15 | darwin/arm64 | Xcode / macOS 原生框架 | native-macos-arm64 |

每个安装包 Artifact 包含独立的 CLI 和 GUI 压缩包。Linux 两种压缩包都包含相邻的 `netgap-gio-proxy`；Unix 先归档再上传，保留可执行权限及 `.app` 结构。产物保留 14 天，不自动发布。

每个平台另上传 `evidence-<平台标识>`：提交号、运行链接、工具链和系统版本、核心测试日志，以及执行到相应步骤时的 GIO 测试、GUI 构建和动态库信息。失败任务也尝试上传已生成的证据。

Ubuntu 24.04 同时设置 Go 构建标签和 Wails 显式构建标签，覆盖绑定生成及最终 GUI 构建。依据：[Wails 平台依赖说明](https://wails.io/docs/gettingstarted/installation/)。

## 最终结论

**原生 CI 五个平台全部通过；交互式桌面验收和真实断网 / 恢复未执行。**

- 成功运行：[39407712602](https://github.com/winezer0/netgap/actions/runs/39407712602)，验证提交 `1466ee78ee494d741944c9cd24da41e0e0b6b8eb`。
- 时间：2026-10-20 10:24:27–10:27:00 UTC（北京时间 18:24:27–18:27:00）。
- 分支：`validation/native-desktop-20261020`；没有合并 main 或发布 Release。
- 上传结果：5 份原生安装包 Artifact、5 份测试 / 构建证据 Artifact，均上传成功。
- 五个平台的日志已下载至 `build/native-validation/39407712602-evidence/`（Git 忽略），首两轮证据也保存在 `build/native-validation/`。提交号、各步骤结论、Artifact ID / SHA256 / 到期时间和环境信息永久记录在 [CI 证据清单](native-ci-evidence-2026-10-20.json)。
- 安装包本地下载及独立归档核验尚未完成：大文件下载等待数分钟未完成，单包重试亦未完成，已停止本轮下载进程；不能将 CI 上传成功写作本地归档检查通过。可在成功运行页面下载对应安装包进行后续验收。
- 用户确认暂时没有桌面测试环境，因此下方桌面验收矩阵仍全部为待执行；不以 CI 编译成功代替桌面行为验证。
- 本轮未修改 WSL 路由、网卡、DNS 或系统代理。

| 平台 | 原生核心竞态测试 | 完整 GUI 构建 | 安装包 / 证据上传 |
|---|---|---|---|
| Windows amd64 | 通过，61 个顶层用例 | 通过 | 通过 |
| Ubuntu 22.04 amd64 | 通过，66 个顶层用例；另含 GIO / PAC 私有会话测试 | 通过，WebKit 4.0 | 通过 |
| Ubuntu 24.04 amd64 | 通过，66 个顶层用例；另含 GIO / PAC 私有会话测试 | 通过，WebKit 4.1 | 通过 |
| macOS Intel | 通过，55 个顶层用例 | 通过，原生 x86_64 Mach-O | 通过 |
| macOS ARM | 通过，55 个顶层用例 | 通过，原生 arm64 Mach-O | 通过 |

核心测试每个平台覆盖 7 个包；Linux 的 opt-in GIO 原生用例在普通核心测试中跳过一次，随后在私有 D-Bus / 临时配置夹具中单独执行并通过。Linux 还单独通过只读 Netlink 监听测试。GUI 构建未包含真实桌面启动，macOS 未执行 Developer ID 签名 / 公证验收。

## 执行记录

- 本地工作流静态检查：`go run github.com/rhysd/actionlint/cmd/actionlint@latest .github/workflows/native-build.yml`，通过。
- 首轮原生 CI：[39406149335](https://github.com/winezer0/netgap/actions/runs/39406149335)，提交 `9218c4d`。Windows、macOS Intel / ARM 和 Ubuntu 24.04 通过；Ubuntu 22.04 的 PAC 集成测试失败，系统解析返回 DIRECT。未放宽断言或跳过测试。
- 修正 Ubuntu 22.04 的 CI 和发行构建依赖：安装 `libproxy1-plugin-webkit`。Ubuntu 24.04 的 libproxy 0.5 使用不同依赖布局。依据：[Ubuntu 22.04 PAC 插件](https://packages.ubuntu.com/jammy/libproxy1-plugin-webkit)、[Ubuntu 24.04 libproxy 依赖](https://packages.ubuntu.com/noble/libproxy1v5)。
- 第二轮：[39406841806](https://github.com/winezer0/netgap/actions/runs/39406841806)，提交 `b0d900b`。Ubuntu 22.04 / 24.04、macOS Intel / ARM 均通过全部构建和上传；Windows 的 `TestManagerStopShouldNotLogAfterStop` 失败（停止后额外输出两条日志）。
- Windows 失败已用新测试 `TestManagerStopDuringResultPublication` 在本地稳定复现：停止发生在健康状态发布与结果日志之间，旧结果和异步停止日志随后发布。修复为普通日志检查运行身份和取消状态，发布结果前复查；停止日志在 Stop / 自然结束之间去重，显式 Stop 在返回前请求发布，不等待路由操作退出。已开始调用的日志回调不强行中断。
- 修复后本地 `go test -race ./internal/guiapp/monitoring -count=100 -timeout=180s` 通过，含停止不阻塞慢路由及旧任务不覆盖新运行的测试；`go test -race ./internal/monitor ./internal/guiapp/monitoring ./internal/iproute -count=1 -timeout=180s` 通过。
- 第三轮原生 CI 五个平台全部通过，详见上方最终结论及证据清单。
- 交互式桌面验收：用户确认暂时没有桌面测试环境，全部保留为待执行。
- 真实断网 / 恢复：尚未执行，仅限具有快照和控制台的隔离虚拟机。

## 桌面验收矩阵

各项需记录系统版本、架构、桌面 / 会话类型、安装包来源提交、操作步骤、预期 / 实际结果及日志或截图路径。未执行项不能填写通过。

| 项目 | macOS Intel | macOS ARM | Ubuntu 22 GNOME | Ubuntu 24 GNOME | Linux KDE |
|---|---|---|---|---|---|
| GUI 启动、渲染、托盘、关闭 / 重开 | 待执行 | 待执行 | 待执行 | 待执行 | 待执行 |
| 设置保存、重启加载、日志展示 | 待执行 | 待执行 | 待执行 | 待执行 | 待执行 |
| 无代理、固定 HTTP/HTTPS/SOCKS、绕过规则 | 待执行 | 待执行 | 待执行 | 待执行 | 待执行 |
| PAC 按 URL 选择代理 / DIRECT、失败不擅自直连 | 待执行 | 待执行 | 待执行 | 待执行 | 待执行 |
| 代理退出 / 重启、切换配置后监控继续 | 待执行 | 待执行 | 待执行 | 待执行 | 待执行 |
| 切网和快速切换回原网络后重新检测 | 待执行 | 待执行 | 待执行 | 待执行 | 待执行 |
| 原生监听或 PAC 服务故障后的异常展示和自愈 | 待执行 | 待执行 | 待执行 | 待执行 | 待执行 |
| 停止 / 重启监控，旧任务不覆盖新状态 | 待执行 | 待执行 | 待执行 | 待执行 | 待执行 |
| 权限不足、损坏快照、不可写目录的恢复预检 | 待执行 | 待执行 | 待执行 | 待执行 | 待执行 |

### 执行约束

1. 从成功的同一次 CI 下载对应架构的安装包和证据，确认其提交号；Linux 保持助手与程序相邻。
2. 首轮使用普通用户、全新临时配置且关闭自动删除路由；先核对配置，再启动监控。不要复用可能启用了自动保护的现有配置。
3. 使用隔离桌面会话中的代理和本地 PAC 测试服务；每个查询 URL 核对系统解析结果与实际请求路径。不得以查询成功单独证明代理路径正确。
4. 网络变化和系统代理配置操作仅在测试机执行。监听故障须有可重复的注入方式；不能以共享状态机单元测试代替原生故障验收。
5. 真正删除 / 恢复路由前创建虚拟机快照并确认控制台可用，覆盖多网卡、IPv4/IPv6、部分失败、重启恢复及绑定冲突。证据保存在宿主机可取得的位置。
6. 当前 WSL 不执行路由删除 / 恢复，不修改其网卡、DNS 或系统代理设置。

## 单条验收证据模板

```text
用例：
环境（系统 / 架构 / 桌面 / 会话）：
CI URL / 提交号 / Artifact：
前置配置：
操作与时间点：
预期：
实际：
日志 / 截图：
结论（通过 / 失败 / 未执行）：
恢复与清理结果：
```
