# 代理与路由代码审查

**状态：以下 5 项 P1、3 项 P2 已完成对应代码调整，待回归验证。** 审查基线为提交 `763ce58`；下文保留当时的触发条件与建议。当前变更、平台限制及尚未执行的验证见 [本轮修复进展](proxy-route-repair-progress.md)，不能将编译通过视为真实故障已全部验证解决。

## 结论

本轮发现 **5 项 P1、3 项 P2**。优先处理：部分断网后无法继续重试、持久路由恢复参数不符合接口约束、备份合并冲突，以及 PAC 回退超时。

本报告基于当前工作区静态阅读及官方接口文档。未执行测试，未修改业务代码、系统代理、网络或路由。下列触发过程属于代码路径分析，不表示已在真实网络复现；上轮测试通过也不能覆盖这些遗漏场景。

## P1：部分路由删除成功后，重试可能再也无法执行

**位置：** [manager.go:143](/C:/ISEC/Deving/NetGAP/internal/guiapp/monitoring/manager.go:143)、[snapshot.go:198](/C:/ISEC/Deving/NetGAP/internal/iproute/snapshot.go:198)、[monitor.go:123](/C:/ISEC/Deving/NetGAP/internal/monitor/monitor.go:123)。

**触发：** 主用网卡默认路由删除成功，随后另一条路由删除失败；剩余路由无法访问查询 API，例如备用网卡不可达或仅有不可用的 IPv6 路径。

**问题：** 下一轮必须先重新取得出口 IP，并再次命中地区，才会调用路由删除。出口查询失败会直接返回，因此“保留监控并重试”仅重试了 IP 查询，未必能继续尚未完成的断网操作。CLI 存在相同依赖。

**影响：** 断网处于半完成状态，界面仍显示监控运行，但剩余路由处理没有进展；之后备用路径恢复还可能重新提供网络连接。

**建议：** 引入待完成断网状态，以快照中的具体路由和网络身份执行有边界的重试，不再依赖获得新的外部 IP；网络身份改变时重新判定，不能无条件扩大删除范围。

## P1：持久路由恢复使用了不受支持的创建参数

**位置：** [snapshot_windows.go:145](/C:/ISEC/Deving/NetGAP/internal/iproute/snapshot_windows.go:145)。

**触发：** 快照包含 PersistentStore 记录。恢复代码将保存的 `store` 原样传给 `New-NetRoute -PolicyStore`。

**问题：** 官方文档明确说明，创建路由时不能指定 `PersistentStore`；默认创建到两个存储，仅创建临时路由时指定 `ActiveStore`。因此当前持久恢复分支与接口约束不符，存在恢复失败的明确风险。[New-NetRoute 官方说明](https://learn.microsoft.com/en-us/powershell/module/nettcpip/new-netroute#-policystore)

**影响：** 可能只恢复活动路由，持久配置仍缺失，或持续返回部分恢复失败；重启后尤其明显。

**测试遗漏：** 现有恢复脚本测试模拟了 `New-NetRoute`，且只断言 ActiveStore，不会发现真实 cmdlet 对 PersistentStore 的限制。

**建议：** 按真实创建语义分别处理临时、持久配置，并核对两个存储的最终状态；不能假设读取与创建支持相同的参数值。

## P1：同一路由属性改变后，备份会生成互相冲突的恢复记录

**位置：** [snapshot.go:166](/C:/ISEC/Deving/NetGAP/internal/iproute/snapshot.go:166)、[snapshot.go:262](/C:/ISEC/Deving/NetGAP/internal/iproute/snapshot.go:262)、[snapshot_windows.go:134](/C:/ISEC/Deving/NetGAP/internal/iproute/snapshot_windows.go:134)。

**触发过程：**

1. 保存并删除某条路由，跃点为 10，尚未执行应用内恢复。
2. 系统或用户重新建立相同接口、目的前缀、下一跳的路由，跃点变成 20。
3. 再次触发断网，合并恢复记录。

**问题：** `sameRoute` 将跃点、协议等可变属性也作为去重条件，因而保留跃点 10 和 20 两条记录；恢复时 `Get-Matching` 却只按接口、前缀、下一跳匹配。旧记录先恢复成跃点 10，新记录随后被判断为冲突。

**影响：** 新状态无法完整恢复，甚至先恢复了过期的跃点配置；后续重试仍可能被该冲突阻挡。

**建议：** 分开定义路由身份与恢复属性；同一身份保存明确的最新恢复目标。确需保留历史版本时，不能将多个历史版本同时作为当前恢复目标。

## P1：首个代理超时会耗尽 PAC 后续候选的时间

**位置：** [checker.go:38](/C:/ISEC/Deving/NetGAP/internal/ipchecker/checker.go:38)、[checker.go:124](/C:/ISEC/Deving/NetGAP/internal/ipchecker/checker.go:124)、[proxy_transport.go:44](/C:/ISEC/Deving/NetGAP/internal/ipchecker/proxy_transport.go:44)。

**触发：** PAC 返回 `PROXY A; PROXY B; DIRECT`，A 不拒绝连接，而是持续无响应。

**问题：** PAC 解析与全部候选共用 5 秒请求上下文，没有为单个候选设置更短的连接预算。A 用完上下文后，即使错误被判断为允许回退，下一次循环也会因 `ctx.Err()` 直接退出，无法尝试 B 或 DIRECT。

**影响：** 候选回退对立即拒绝连接有效，但面对无响应代理可能持续检测失败，尽管后续合法路径可用。

**建议：** 分开规则解析、单候选连接和整轮查询预算，为后续候选预留时间；同时覆盖“连接被拒绝”和“连接无响应”两种回归场景。

## P1：无关地址变化也会阻止恢复同一网络的路由

**位置：** [snapshot_windows.go:70](/C:/ISEC/Deving/NetGAP/internal/iproute/snapshot_windows.go:70)、[snapshot_windows.go:76](/C:/ISEC/Deving/NetGAP/internal/iproute/snapshot_windows.go:76)。

**触发：** 保存的是 IPv4 默认路由，但同一网卡的 IPv6 地址、次要地址或地址前缀随后发生变化，原 IPv4 网络与网关仍有效。

**问题：** 每条路由保存整张网卡的所有地址，恢复时要求全部字符串完全一致。地址变化没有按路由所属地址族、相关子网或稳定网络身份区分。

**影响：** 未切换实际相关网络，也可能被判定为旧网络而拒绝恢复；重试只会重复失败。

**建议：** 校验稳定接口与网络标识，以及该条路由相关的地址族、子网和下一跳适用性。保留跨网络保护，但不以不相关地址的变化否定所有路由。

## P2：Linux 原有断网与恢复功能回退为始终不支持

**位置：** [route.go:141](/C:/ISEC/Deving/NetGAP/internal/iproute/route.go:141)、[snapshot_other.go:14](/C:/ISEC/Deving/NetGAP/internal/iproute/snapshot_other.go:14)、[actions.go:67](/C:/ISEC/Deving/NetGAP/internal/guiapp/actions.go:67)。

**问题：** 原先按平台分派的 `DeleteDefaultRoute` 现在统一进入快照流程，但非 Windows 的锁和后端均直接返回“不支持”。GUI 和 CLI 恢复入口也已经切换到该流程。

**影响：** 若 Linux 仍属于支持范围，自动断网、手动断网和恢复均发生功能回退。文档声明不支持不能替代兼容性处理。

**建议：** 补齐 Linux 完整快照实现；在此之前明确平台能力，并在启动保护前拒绝不支持的配置，避免看似开启了保护却无法执行。

## P2：无关或被绕过的环境代理配置会阻止有效查询

**位置：** [proxy.go:37](/C:/ISEC/Deving/NetGAP/internal/detector/proxy.go:37)、[proxy_routes.go:57](/C:/ISEC/Deving/NetGAP/internal/detector/proxy_routes.go:57)。

**触发：** 系统未配置代理，HTTP_PROXY 格式错误但 HTTPS_PROXY 有效；或目标应由 NO_PROXY 绕过，而对应代理值无效。

**问题：** 检测阶段先全局解析首个环境变量，可能在按网址选路前就失败；请求阶段也先验证代理，再执行 `ProxyFunc` 的绕过判断。

**影响：** 本应使用有效 HTTPS 代理或直连的查询，也会被无关配置拦截，违背逐网址应用规则的目标。

**建议：** 检测阶段保留环境规则集合；针对实际网址完成协议选择和绕过判断后，再验证需要使用的代理。

## P2：路由数据放入命令行，存在长度上限

**位置：** [snapshot_windows.go:42](/C:/ISEC/Deving/NetGAP/internal/iproute/snapshot_windows.go:42)、[snapshot_windows.go:54](/C:/ISEC/Deving/NetGAP/internal/iproute/snapshot_windows.go:54)、[snapshot_windows.go:107](/C:/ISEC/Deving/NetGAP/internal/iproute/snapshot_windows.go:107)。

**问题：** JSON 先做 Base64，再嵌入脚本整体按 UTF-16/Base64 编码，所有路由验证记录最终都进入 `-EncodedCommand`。路由、网卡地址较多时会放大命令行长度；代码没有长度检查，也没有标准输入或文件通道。

**影响：** 达到 Windows 创建进程的 32,767 字符上限后，验证命令无法启动，保护操作失败。该项为条件性容量风险，未测量当前机器达到上限所需的记录数。[CreateProcessW 官方限制](https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-createprocessw)

**建议：** 脚本保持固定，结构化数据通过标准输入或受控临时文件传递。

## 建议修复顺序

1. 部分断网的独立重试状态、持久路由创建语义。
2. 路由身份去重、相关网络身份校验。
3. PAC 候选时间预算。
4. 平台兼容、环境变量逐网址解析、命令行数据传输。

现有模拟测试应补充真实 cmdlet 参数约束、部分断网后 API 不可达、同一路由属性改变、首候选无响应等场景。真实删除/恢复仍应放在隔离网络环境验证。
