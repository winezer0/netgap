# 路由恢复修正后全量测试

日期：2026-10-21；环境：Windows amd64，本地工作区未提交版本。

本轮依据用户提供的 CIM 扩展错误修正有限路由生命周期：扣除经过时间后向下取整到秒，ValidLifetime/PreferredLifetime 不再携带微秒，且共用计算时间。实际恢复尚未经过隔离环境验收。

| 检查 | 结果 |
| --- | --- |
| `go test -count=1 ./...` | 通过 |
| `go test -race -count=1 -timeout=180s ./...` | 通过，无 race 报告 |
| `npm run build` | ESLint、26 项前端测试和 Vite 生产构建通过 |
| `python -B -m unittest discover -s scripts -p 'test_*.py' -v` | 5 项通过 |
| `go vet ./...` | 未通过：两处原生指针转换警告，见下文 |
| `go vet -unsafeptr=false ./...` | 其余检查通过；不能替代完整 vet 通过 |
| 路由包交叉编译 | Linux amd64、macOS amd64/arm64、Windows arm64 通过，`CGO_ENABLED=0` |
| Windows amd64 GUI | `node build.mjs -s -skipbindings -o netgapgui-lifetime-fix.exe` 构建通过 |

## 回归覆盖

- 日志中的有限生命周期：剩余 15:59:40.061603 转换为 15:59:40，微秒为 0。
- 有限首选期、不足一秒的有效期/首选期、过期、无限生命周期、保存时间晚于当前时间。
- 本机 CIM 输入构造与类型回读、与系统 CDXML 接口索引参数集核对，均不调用创建/删除方法。
- 无快照恢复不调用路由后端；GUI 将其显示为无需恢复，实际后端错误继续显示失败。

## 未解决的检查警告

完整 `go vet` 报告：

- `internal/detector/network_table_windows.go:88`：`possible misuse of unsafe.Pointer`。
- `internal/detector/system_proxy_windows.go:117`：`possible misuse of unsafe.Pointer`。

本轮未修改这两处原生 ABI 指针转换；race 测试通过不代表这些警告已消除。

## 验收边界

未执行当前宿主机或 WSL 的路由增删、DNS/代理修改，未触发远端 CI。macOS/Linux GUI 原生运行、Windows 真实 IPv4/IPv6 删除与恢复、持久存储副作用和立即恢复仍须在隔离环境验收；交叉编译与模拟测试不能代替这些验收。

产物：`cmd/netgapgui/build/bin/netgapgui-lifetime-fix.exe`。构建未清理原程序或 `iproute.cache.json`。
