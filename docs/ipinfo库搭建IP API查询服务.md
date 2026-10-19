# cmd/ipinfoapi - HTTP API 服务

ipinfo 支持多种数据库类型的IP定位信息解析库 [项目地址](https://github.com/winezer0/ipinfo)

ipinfoapi 是基于 ipinfo 库构建的简单的 RESTful API 服务，提供 IP 地理位置和 ASN 信息查询接口。

当前已在NetGap中携带该接口格式自定义API地址支持

### API 端点

| 方法  | 路径                      | 说明               | 认证 |
| --- | ----------------------- | ---------------- | -- |
| GET | `/ping`                 | 健康检查             | 不需要 |
| GET | `/ip?q=<ip>`            | 查询 IP 地理位置       | 需要 |
| GET | `/asn?q=<ip>`           | 查询 ASN 信息        | 需要 |
| GET | `/all?q=<ip>`           | 查询完整信息（IP + ASN） | 需要 |
| GET | `/full?q=<ip1,ip2,...>` | 批量查询             | 需要 |

### 认证方式

支持两种认证方式（优先级从高到低）：

1. **Authorization 请求头**：`Authorization: Bearer <token>`
2. **URL 参数**：`?token=<token>`

### 请求示例

```bash
# 健康检查
curl "http://localhost:8080/ping"

# 查询客户端 IP 完整信息
curl "http://localhost:8080/all?token=your-token"

# 查询 IP 地理位置
curl "http://localhost:8080/ip?q=8.8.8.8&token=your-token"

# 查询 ASN 信息
curl "http://localhost:8080/asn?q=8.8.8.8&token=your-token"

# 查询完整信息
curl "http://localhost:8080/all?q=8.8.8.8&token=your-token"

# 批量查询
curl "http://localhost:8080/full?q=8.8.8.8,1.1.1.1,114.114.114.114&token=your-token"
```

### 响应格式

**成功响应：**

```json
{
  "success": true,
  "code": 0,
  "data": { ... }
}
```

**错误响应：**

```json
{
  "success": false,
  "code": 1001,
  "error": "错误描述"
}
```

### 部署方式

#### 1. 准备数据库文件

将所需的数据库文件放入 `assets/` 目录：

默认数据库文件可从 [cdninfo/assets](https://github.com/winezer0/cdninfo/tree/main/assets) 下载。

```
assets/
├── qqwry.dat              # 纯真 IP 数据库
├── zxipv6wry.db           # 紫薇 IPv6 数据库
├── geolite2-asn.mmdb      # GeoLite2 ASN 数据库
└── ...
```

#### 2. 配置文件

复制并编辑配置文件 `ipinfoapi.yaml`：

```yaml
auth:
  token: "your-secret-token"
  enable: true

http:
  enable: true
  port: 8080
  https: false
  read_timeout: 10
  write_timeout: 10
  idle_timeout: 30

database:
  asn_ipvx_db: "assets/geolite2-asn.mmdb"
  ipv4_locate_db: "assets/qqwry.dat"
  ipv6_locate_db: "assets/zxipv6wry.db"

log:
  level: "info"
  file: "logs/ipinfoapi.log"
  console: "TLCM"
  max_size: 100
  max_backups: 3
```

#### 3. 编译

```bash
# Windows
.\build-win.bat

# Linux
./build-linux.bat

# 手动编译
go build -o ipinfoapi ./cmd/ipinfoapi
```

#### 4. 运行

```bash
# 前台运行
./ipinfoapi

# 后台运行（Linux）
nohup ./ipinfoapi &

# 后台运行（Windows）
Start-Process -NoNewWindow .\ipinfoapi.exe
```
