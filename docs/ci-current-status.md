# 当前 CI 修改与验证状态

核查日期：2026-10-21。

## 文件已修改

- `.github/workflows/native-build.yml`：移除 GIO 助手构建、PAC 依赖及私有桌面 GIO 测试；Go 竞态测试扩展至 `./...` 全部包，保留 Linux 路由通知测试、原生构建和产物检查；Linux 归档只包含主程序。
- `.github/workflows/release-build.yml`：移除助手构建和打包及专用 PAC 依赖。
- `scripts/verify-native-artifacts.py`、`scripts/test_native_artifacts.py`：调整 Linux 产物检查及现有测试样例，归档不再要求助手。
- GNOME 现有错误模式用例改为未知模式，因为自动模式现在采用降级直查。

## 是否通过

本地全量自动测试已通过；当前批次远端 CI 正在安排运行，结果将补充在本文。

此前 [Native Build](https://github.com/winezer0/netgap/actions/runs/39528784603) 已成功，对应提交 `09bc82125d96d5283c6b1e7caf00d8538ed1923a`，不证明本批改动通过。

## 2026-10-21 本地全量测试

| 环境 | 命令 | 结果 |
|---|---|---|
| Windows | `go test ./... -count=1 -timeout=180s` | 通过 |
| Windows | `go test -race ./... -count=1 -timeout=180s` | 通过 |
| Windows | `go test -tags acceptance -race ./internal/detector -run TestNativeWPAD -count=1 -timeout=60s` | 通过，仅验证不可用 WPAD 的回退分支 |
| WSL 无桌面测试子进程 | `go test -race ./... -count=1 -timeout=180s` | 通过 |
| 前端 | `npm run build` | ESLint、5 个测试文件共 20 项测试、Vite 构建全部通过 |
| Windows / WSL | `python -B -m unittest discover -s scripts -p test_native_artifacts.py -v` | 各 5 项测试通过，WSL 使用 `python3` |

WSL 首轮直接继承 WSLg 会话环境时失败：显示/会话变量存在，但未识别出 GNOME/KDE，触发预期的未知桌面来源错误，导致依赖真实配置的测试失败。重跑仅对测试子进程清除 `XDG_CURRENT_DESKTOP`、`DISPLAY`、`WAYLAND_DISPLAY`、`DBUS_SESSION_BUS_ADDRESS`、`DESKTOP_SESSION`，使用 `GOMODCACHE=/mnt/c/Program_Base/gopath/pkg/mod GOPROXY=off GOSUMDB=off`；全量竞态测试通过。此结论仅适用于无桌面夹具，不代表 WSLg 或 GNOME/KDE 桌面验收通过。

本批未修改 WSL 路由、DNS、接口或系统代理配置。真实断网/恢复仅由隔离环境执行。PAC/WPAD 降级直查、已有重试暂停等真实桌面运行回归仍待验收；自动测试通过不等同于所有新增分支均有专项覆盖。
