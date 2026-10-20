# 测试结果（2026-10-20）

后续 B1 已实施并补充 WSL 专项、10 轮竞态及选定核心回归，Windows 全量复测通过；WSL 网络配置前后指纹一致。最新结果见 [B1 与 WSL 记录](wsl-gnome-test-results.md)，下文保留此前测试与前端修复记录。

环境：Windows amd64，实际执行工具链为 `go1.26.4 windows/amd64`。针对当前未提交工作区运行；首次仅执行测试，后续按用户要求修复前端测试模拟并复测，业务代码未修改。

## 修复后复测

- 补齐 `useAppState.spec.js` 的 `onHealth` 模拟，四类事件均返回可断言的取消订阅函数；每项用例结束后卸载组件，并重置共享健康状态，避免用例互相影响。
- 新增三个用例：过滤旧会话/旧修订健康事件；初始状态晚于实时事件返回时保留新健康状态；组件卸载取消全部事件订阅。
- 前端 `npm test -- --reporter=verbose`：**5 个文件、18 项全部通过**，退出码 0，无未处理异常。
- 前端 `npm run lint`：通过，退出码 0。
- 此次仅修改测试和文档，无需重新生成 Wails 绑定或前端内嵌产物；Go 代码未变，沿用下方已通过的全量及竞态检测记录。

## 首次执行结果（保留失败记录）

| 命令 | 结果 |
| --- | --- |
| `go test ./... -count=1 -timeout=180s` | 通过，退出码 0 |
| `go test -race ./internal/detector ./internal/guiapp/monitoring ./internal/ipchecker ./internal/iproute ./internal/monitor -count=1 -timeout=180s` | 五个包全部通过，退出码 0，未报告数据竞争 |
| 前端目录执行 `npm test -- --reporter=verbose` | 失败，退出码 1；5 个文件中 4 个通过，15 项中 13 项通过、2 项失败，另有 3 个未处理异常 |
| 前端目录执行 `npm run lint` | 通过，退出码 0 |
| `git diff --check` | 通过，退出码 0 |

## 前端失败原因

`src/composables/useAppState.spec.js` 的 backend 模拟对象缺少新增的 `onHealth` 方法。生产 backend 已提供该接口，但测试在挂载时抛出 `TypeError: backend.onHealth is not a function`，未继续调用初始状态读取。

因此两项断言失败：

1. `loads initial state and subscribes to live state and logs`：初始状态读取未被调用。
2. `saves close and route preferences and merges the backend result`：初始语言未加载，保存参数仍为默认 `zh`，与预期 `en` 不符。

另一个托盘/退出测试虽然断言通过，挂载也产生同样异常，不能将其视为无异常通过。

上述模拟缺失已修复，健康状态过滤及卸载清理覆盖已补充，复测结果见本页首节；未更改生产代码或削弱既有断言。

## 本次证据范围

- 执行了 Windows 原生 PAC、代理传输、停止/重启、失败恢复、路由快照等现有测试。路由变更逻辑使用模拟后端或模拟 PowerShell cmdlet；Windows 真实系统部分为只读枚举与绑定校验，变更脚本还进行了不执行的语法解析。
- 未修改系统代理、网卡或实际默认路由，未执行真实断网/恢复；现有公网查询测试可能访问外部 IP 服务。
- 部分旧公网测试在请求失败时仅记录日志后返回，因此 Go 测试通过不能证明公网服务均可访问，也不能证明所有实际出口路径正确。
- 未运行 Linux/macOS 原生测试。Linux gsettings 进程挂起、进程组取消和输出限制，以及 macOS 同步桥接与订阅恢复，仍缺相应原生故障测试证据。
- 新健康状态、续作调度和取消链路的所有新增分支尚无完整针对性回归覆盖；本次现有测试通过不能替代该覆盖。

结论：Go 全量、核心竞态检测及修复后的前端 18 项测试均通过，ESLint 通过。Linux/macOS 原生故障与真实路由操作尚未验证，当前仍不能标记全部平台回归完成。
