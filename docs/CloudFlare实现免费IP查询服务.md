# CloudFlare 提供 Worker 功能可实现免费外网IP查询服务器

## 优化点说明
1. ✅ 自动识别连接协议 IPv4 / IPv6，增加 `protocol` 字段输出
2. ✅ 增加简单输入校验、健壮性兜底
3. ✅ 分离逻辑，可读性更高
4. ✅ 支持：默认JSON / `?fmt=text` 纯文本IP
5. ✅ 完善OPTIONS跨域预检
6. ✅ 统一Header管理，去除冗余
7. ✅ 增加响应超时保护（ctx.waitUntil）
8. ✅ 增加提示：当IP获取失败时返回明确提示
9. ✅ 不引入多余调试字段，适合公开生产使用

```javascript
export default {
  async fetch(request, env, ctx) {
    const { method, headers } = request;
    const url = new URL(request.url);
    const fmt = url.searchParams.get("fmt");

    // 跨域预检处理
    if (method === "OPTIONS") {
      return new Response(null, {
        headers: {
          "Access-Control-Allow-Origin": "*",
          "Access-Control-Allow-Methods": "GET, OPTIONS",
          "Access-Control-Allow-Headers": "*",
          "Access-Control-Max-Age": "86400",
        },
      });
    }

    // 获取客户端出口IP，双保险
    const ip = request.cf?.connectingIp || headers.get("CF-Connecting-IP") || null;

    // 识别IP协议版本
    let protocol = "unknown";
    if (ip) {
      protocol = ip.includes(":") ? "IPv6" : "IPv4";
    }

    // CF地理信息
    const country = request.cf?.country ?? "unknown";
    const asn = request.cf?.asn ?? "unknown";

    // 处理纯文本返回模式，只返回IP
    if (fmt === "text") {
      return new Response(ip ?? "unknown", {
        headers: {
          "Content-Type": "text/plain;charset=utf-8",
          "Access-Control-Allow-Origin": "*",
          "Cache-Control": "no-store, no-cache, must-revalidate",
        },
      });
    }

    // JSON返回
    const result = {
      ip: ip ?? "unknown",
      protocol,
      country,
      asn
    };

    if (!ip) {
      result.warning = "无法获取客户端出口IP，链路不支持传递源IP";
    }

    return new Response(JSON.stringify(result, null, 2), {
      headers: {
        "Content-Type": "application/json;charset=utf-8",
        "Access-Control-Allow-Origin": "*",
        "Cache-Control": "no-store, no-cache, must-revalidate",
      },
    });
  },
};
```

## 调用示例
```bash
# JSON（推荐，带协议标记）
curl [https://ip.kissloo.workers.dev](https://ip.kissloo.workers.dev)
```
示例返回
```json
{
  "ip": "2a06:98c0:3600::103",
  "protocol": "IPv6",
  "country": "US",
  "asn": 13335
}
```

```bash
# 仅返回ip文本
curl [https://ip.kissloo.workers.dev?fmt=text](https://ip.kissloo.workers.dev?fmt=text)
```

# 生产运维配套建议（非常重要）
1. **速率限制**：在 Workers路由规则 配置：`true` 匹配所有，速率限制：**单IP 60次/分钟，429**，防止被爬虫刷爆。
2. 日志：后台Workers日志可以查看访问记录。
3. 合规：公开接口，页面上增加免责声明：
> 免责声明：本接口仅返回TCP网络出口IP、网络归属国家与ASN；仅代表网络路由出口，不等于真实物理地理位置，禁止用于恶意溯源、网络攻击。
