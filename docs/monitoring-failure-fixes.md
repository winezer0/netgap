# 导致监控失效的问题与修复顺序

本文保留早期监控故障链、设计要求和修复背景，不维护当前状态。当前验收结论统一见[已验证场景记录](已验证场景记录.md)和[剩余未验证场景记录](剩余未验证场景记录.md)；实施设计见[修复进展](proxy-route-repair-progress.md)。

同时将自定义 API 配置改为每轮查询独立持有，避免旧任务结束期间与新任务共享可变端点；显式代理 Transport 在查询结束后关闭空闲连接。直连查询恢复不再输出“代理连接恢复”。

早期测试方法和当时运行结果不作为当前状态记录；当前结果见上述集中记录。

```text
rtk proxy go test ./internal/ipchecker ./internal/monitor ./internal/guiapp/monitoring -run TestRecovery -count=1 -v
rtk proxy go test -race ./internal/ipchecker ./internal/monitor ./internal/guiapp/monitoring -run 'TestRecovery|TestManager' -count=1
rtk proxy go test ./internal/ipchecker ./internal/monitor -count=1
rtk proxy go test ./internal/guiapp/... ./internal/detector ./cmd/netgap ./cmd/netgapgui -count=1
```

本文保留首轮模拟测试的范围说明；真实系统场景的状态集中见[剩余未验证场景记录](剩余未验证场景记录.md)。

以下保留修复前故障链与验收依据。这里的“监控失效”包含任务意外停止、任务运行但没有查询能力，以及查询正常但保护不再执行三种情况。

## 第一优先级：网络恢复后监控无法恢复

**故障链：** 切网或代理中断 → 查询失败计入 API → 同一 API 连续失败 11 次后被永久移除 → 所有已启用 API 都可能被移除 → 网络恢复后仍查不到出口 IP。

初始化由 `sync.Once` 控制，停止再启动监控不会重建查询器。此时循环可能一直运行，但已经没有查询能力。这是当前最直接、已经复现的监控失效问题。

Windows 的代理连接拒绝分类错误会加重该问题：实际 Socket 错误码为 `10061`，当前用于比较的 `syscall.ECONNREFUSED` 为 `536870934`，因此代理退出仍会损耗 API 可用列表。即使修好拒绝连接判断，断网超时、连接重置等仍可能删除 API，必须一并处理。

**位置：** `internal/ipchecker/checker.go` 的 `initFetchers`、`GetExternalIP`、`increaseErrorCount`。

**修复要求：**

- 网络故障与代理故障不计入 API 自身失败惩罚。
- API 自身失败使用冷却和重新探测，不能永久删除。
- 没有可查询 API 时返回明确错误，不能返回空 IP 加 nil 错误。
- 正确识别 Windows 代理连接错误，同时保留其他平台的处理。

**验收：** 单 API 与多 API 配置分别经历长时间断网、代理退出、代理上游超时后，恢复网络或代理即可重新检测，不需要重启应用或重启监控。

## 第二优先级：快速停止重启导致新监控被旧任务破坏

**故障链：** 旧查询尚未结束 → Stop 立即将 running 置为 false → Start 启动新任务 → 旧任务返回或退出 → 清空共享 running 和 cancel，或者通过共享 cancel 取消新任务。

结果可能是新任务实际被停止，或后台仍在查询但界面显示停止、Stop 无法管理该任务。旧任务也可能继续执行路由操作。停止后仍执行模拟删除操作已经复现；新旧任务交叉风险由当前调用顺序确认。

**位置：** `internal/guiapp/monitoring/manager.go` 的 `Stop`、`Start`、`requestStop`、`finishRun`；`internal/ipchecker/checker.go` 的查询 context。

**修复要求：** 每次监控拥有独立任务标识和取消函数；停止信号传递到查询；只有当前任务可以更新运行状态或执行保护操作；旧任务退出不得修改新任务。

**验收：** 查询阻塞时连续停止、启动，再让旧查询返回成功或错误，新任务均继续正常检测，运行状态准确，且始终能够被停止。

## 第三优先级：GUI 删除路由失败后保护不再重试

**故障链：** 命中区域 → 删除前设置 localHandled=true → 删除失败 → 后续仍命中同一区域 → 因已处理标记直接跳过保护。

查询和日志仍可正常运行，但自动断网保护已经失效。已复现两次连续区域匹配只调用一次删除函数。只有后续出现未匹配结果或重新启动监控等使该标记复位，才可能再次尝试，不能依赖这种偶然恢复。

**位置：** `internal/guiapp/monitoring/manager.go:178`。

**修复要求：** 保护成功后才标记完成；失败保留明确状态；对可恢复错误进行受控重试，对权限等持续错误明确提示处理，不将失败视为已完成。

**验收：** 第一次模拟删除失败，第二次允许成功；持续命中区域时，保护仍能再次执行并完成。

## 第四优先级：CLI 删除路由失败却退出监控

**故障链：** 命中区域且开启 Shutdown → 删除路由失败只记日志 → 检测仍返回非健康且无错误 → 外层按 Shutdown 配置记录“网络已中断”并结束循环。

结果是网络没有成功中断，监控也已经停止，后续无法继续保护。该项由代码路径确认。

**位置：** `internal/monitor/monitor.go` 的 `CheckProxyStatus` 与 `StartMonitoring`。

**修复要求：** 将路由操作结果传递给外层；失败后保留监控和失败状态；只有确认保护成功后才按既有规则停止监控。

**验收：** 模拟路由删除失败，CLI 不得显示断网成功，不得因此退出检测；故障解除后仍能够完成保护。

## 行为设计参考

代理选路与路由边界见[系统代理与路由说明](system-proxy-routing.md)；当前场景结论统一见[已验证场景记录](已验证场景记录.md)和[剩余未验证场景记录](剩余未验证场景记录.md)。修复前复现背景见[切换检查报告](network-proxy-switch-review.md)。
