# configure

**解决私有配置公开访问时的隐私泄露问题**

例如在GitHub中公开项目时，可能会导致数据库密码、证书文件等敏感数据公开，造成不可预知的风险。

## 特性

- **白名单控制** — 支持 IPv4 和域名，支持 IPv6 格式
- **AES 对称加密** — 使用 AES-CTR 模式，每次加密使用随机 IV，防止 MITM 攻击
- **HTTPS 支持** — 自动检测 TLS 证书，支持 HTTPS 访问
- **JSON 配置管理** — 支持 `base.json` + `overrides.json` 的 IP 级配置覆盖
- **配置继承** — 支持 `__extends` 字段实现多级配置继承（JSON 和 YAML）
- **YAML 支持** — 支持 YAML 格式配置，自动转换为 JSON 返回
- **Redis 通知** — 配置文件变更时自动向 Redis 发布通知
- **实时重载** — 使用 fsnotify 监听文件变化，自动重新加载配置
- **日志管理** — 自动轮转日志文件，防止磁盘空间无限增长

## 项目结构

```
.
├── main.go              # 主程序入口
├── common/
│   ├── config.go        # 配置管理器（JSON/YAML 解析、继承、深度合并）
│   ├── encryption.go    # AES 和 RSA 加密工具
│   └── redis.go         # Redis 客户端和通知发布
├── client/
│   └── main.go          # 客户端示例（获取并解密配置）
├── conf/
│   ├── json/            # JSON 配置文件
│   │   ├── base.json    # 基础配置
│   │   ├── overrides.json # IP 覆盖配置
│   │   ├── sub1.json    # 子配置（继承 base.json）
│   │   └── sub2.json    # 子配置（多级继承）
│   └── yml/             # YAML 配置文件
│       ├── base.yml     # 基础 YAML 配置
│       └── sub1.yml     # 子 YAML 配置（继承 base.yml）
├── whitelist.txt        # IP 白名单配置
├── Dockerfile           # Docker 构建文件
├── docker-compose.yml   # Docker Compose 配置
└── AGENTS.md            # 项目说明文档
```

## 使用

### 本地开发

```bash
# 编译并运行
go build -o configure .
./configure

# 指定 AES 密钥（16/24/32 字节）
AES_KEY=your16byteKeyHere ./configure

# 指定 Redis 地址
REDIS_ADDR=127.0.0.1:6379 REDIS_PASSWORD=secret AES_KEY=your16byteKeyHere ./configure
```

### Docker 部署

```bash
docker-compose up --build
```

### 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `AES_KEY` | AES 加密密钥（16/24/32 字节） | 空（不加密） |
| `REDIS_ADDR` | Redis 服务器地址 | 127.0.0.1:6379 |
| `REDIS_PASSWORD` | Redis 密码 | 空 |

### 白名单配置

编辑 `whitelist.txt`，每行一个 IP 或域名：

```
127.0.0.1
192.168.1.100
example.com
```

### 配置文件继承

JSON 配置支持 `__extends` 字段继承：

```json
// conf/json/sub1.json
{
  "__extends": "base.json",
  "Debug": false,
  "ServerTCP": {
    "Port": "27001"
  }
}
```

YAML 配置同样支持继承：

```yaml
# conf/yml/sub1.yml
__extends: base.yml
Debug: false
ServerTCP:
  Port: "28001"
```

### 配置覆盖

`conf/json/overrides.json` 定义 IP 级别的覆盖：

```json
{
  "127.0.0.1": {
    "Debug": false
  },
  "172.25.0.2": {
    "AppName": "custom-service",
    "ServerTCP": {
      "Port": "27001"
    }
  }
}
```

### API 接口

| 端点 | 方法 | 说明 |
|------|------|------|
| `/configFile?fileName=xxx` | GET | 返回原始配置文件（支持 json 和 yml 目录） |
| `/customConfig` | GET | 返回当前 IP 的合并配置（base + IP 覆盖） |
| `/customConfig?config=sub1` | GET | 返回指定配置的合并结果（支持继承） |
| `/customConfig?config=sub1.yml` | GET | 返回 YAML 配置的合并结果（JSON 格式） |

### 客户端示例

```go
// 获取并解密配置文件
configData := GetRemoteConfigData("config.toml")
println(string(configData))
```

## 安全说明

- **AES 加密** — 使用 AES-CTR 模式，每次加密生成随机 IV，IV 前缀附加在密文前
- **白名单** — 支持域名解析，自动将域名解析为 IP 并加入白名单
- **日志管理** — 日志文件自动轮转，超过 300 行时删除前 200 行
- **客户端示例** — `client/main.go` 中的硬编码密钥仅为示例，生产环境应使用安全方式获取

## 日志

日志写入 `configure.log` 文件，包含以下信息：

- 启动日志（Redis 状态、白名单加载）
- 请求日志（`[REQUEST]` 和 `[RESPONSE]`）
- Redis 通知日志（`[REDIS]`）
- 文件变更日志

## 开发

```bash
# 格式化代码
go fmt ./...

# 检查代码
go vet ./...

# 编译
go build -o configure .

# 测试
# 启动服务器后使用 curl 测试
curl "http://127.0.0.1:6001/configFile?fileName=config.toml"
curl "http://127.0.0.1:6001/customConfig"
curl "http://127.0.0.1:6001/customConfig?config=sub1"
```

## 注意事项

1. **配置加载延迟** — 文件修改后 fsnotify 需要短暂时间处理，curl 立即执行可能看到旧内容
2. **AES 密钥长度** — 必须为 16/24/32 字节，否则记录警告并回退到明文
3. **Redis 连接** — 连接失败不会阻塞服务，仅记录日志
4. **YAML 继承** — YAML 配置继承 JSON 配置时，扩展名会自动推断

## 参考

- `AGENTS.md` — 详细的项目说明和开发指南
- `docker-compose.yml` — Docker 部署配置和卷映射
