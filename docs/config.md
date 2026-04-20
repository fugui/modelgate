## 配置说明

### config.yaml

```yaml
server:
  port: 8080              # 服务端口
  mode: "release"         # debug 或 release
  # 以下为可选的超时配置（使用默认值时可省略）
  read_timeout: 60s       # 读取请求超时（默认 60s）
  write_timeout: 120s     # 写入响应超时（默认 120s）
  idle_timeout: 300s      # 空闲连接超时（默认 300s）
  max_header_bytes: 1048576  # 请求头大小限制，单位字节（默认 1MB）
  shutdown_timeout: 30s   # 优雅关闭超时（默认 30s）

database:
  path: "modelgate.db"      # SQLite 数据库文件路径

# 默认模型 Fallback（可选）
# 当客户端请求的模型没有可用后端时，自动使用此模型
# 示例：客户端请求 "gpt-4" 但只配置了 kimi2.5 后端，系统自动使用 kimi2.5 处理
#default_model: "kimi2.5"

logs:
  path: "./logs"          # 日志目录
  retention_days: 7       # 日志保留天数

jwt:
  secret: "your-jwt-secret-change-in-production"
  expire_hours: 24

admin:
  default_email: "admin@modelgate.local"
  default_password: "admin123"

# LLM 后端配置 - 支持多后端架构
models:
  # 单实例模型配置示例
  - id: "glm4.7"
    name: "GLM 4.7"
    description: "智谱 GLM4.7 模型"
    enabled: true
    backends:
      - id: "glm4.7-prod-01"
        name: "北京节点-01"
        base_url: "http://glm4-7.internal:8000"
        api_key: "glm-api-key-xxx"
        model_name: "glm4"          # 后端实际模型名
        weight: 10                   # 权重（用于负载均衡）
        region: "beijing"

  # 多实例负载均衡配置示例
  - id: "kimi2.5"
    name: "Kimi 2.5"
    description: "Moonshot Kimi 2.5"
    enabled: true
    backends:
      - id: "kimi2.5-gz-01"
        name: "广州节点-01"
        base_url: "http://kimi25-gz-01.internal:8000"
        api_key: "moonshot-key-gz"
        model_name: "kimi2.5_guangzhou"
        weight: 20
        region: "guangzhou"
      - id: "kimi2.5-gz-02"
        name: "广州节点-02"
        base_url: "http://kimi25-gz-02.internal:8000"
        api_key: "moonshot-key-gz"
        model_name: "kimi2.5_guangzhou"
        weight: 20
        region: "guangzhou"
      - id: "kimi2.5-bj-01"
        name: "北京节点-01"
        base_url: "http://kimi25-bj-01.internal:8000"
        api_key: "moonshot-key-bj"
        model_name: "kimi2.5_beijing"
        weight: 15
        region: "beijing"

# 配额策略
quota_policies:
  - name: "default"
    rate_limit: 60              # 每分钟请求数
    rate_limit_window: 60       # 窗口秒数
    request_quota_daily: 500    # 每日请求配额上限
    available_time_ranges:      # 可用时间段（不配置或空列表表示全天可用）
      - start: "00:00"          # 支持跨午夜，如 22:00-06:00
        end: "10:00"
      - start: "18:00"
        end: "24:00"
    models: ["*"]               # "*" 表示所有模型
  - name: "vip"
    rate_limit: 300
    rate_limit_window: 60
    request_quota_daily: 5000
    models: ["*"]               # 不配置 available_time_ranges = 全天可用

# 前端配置
frontend:
  feedback_url: "https://feedback.example.com"
  dev_manual_url: "https://docs.example.com"
  registration_enabled: false   # 开放用户自助注册（注册后需管理员审核）

# 并发控制
concurrency:
  global_limit: 100         # 全局最大并发请求数
  user_limit: 10            # 每用户最大并发请求数

# SSO 配置（可选）
sso:
  enabled: false
  provider: "azure"         # 支持: azure, generic-oidc
  client_id: "your-client-id"
  client_secret: "your-client-secret"
  issuer_url: "https://login.microsoftonline.com/{tenant}/v2.0"
  email_claim: "email"
```

### Backend 配置字段说明

| 字段 | 必填 | 说明 |
|------|------|------|
| `id` | 是 | 后端唯一标识 |
| `name` | 否 | 后端显示名称 |
| `base_url` | 是 | LLM 后端服务地址 |
| `api_key` | 否 | 后端 API 认证密钥 |
| `model_name` | 否 | 后端实际模型名称（转发时使用）|
| `weight` | 否 | 负载均衡权重，默认 1 |
| `region` | 否 | 地域标识 |
| `enabled` | 否 | 是否启用，默认 true |

### model_params 模型参数

`model_params` 用于配置模型特定的请求参数，支持两种类型：

**1. 请求体参数**（普通键值对）
- 自动注入到转发给后端的请求体中
- 不覆盖用户传入的同名参数

```yaml
model_params:
  max_tokens: 4096
  temperature: 0.7
  enable_thinking: false    # 禁用思考模式（DeepSeek等）
```

**2. HTTP Header 参数**（双下划线前缀）
- 以 `__` 开头和结尾的键会被识别为 HTTP Header
- 会覆盖原始请求中的同名 Header

```yaml
model_params:
  __user_agent__: "ModelGate/1.0"      # 自定义 User-Agent
  __header_x_source__: "modelgate"     # 自定义 X-Source 头

# 强制设置 User-Agent 为 Claude Code CLI 的例子：
model_params:
  __user_agent__: "claude-cli/2.1.74 (external, cli)"    # 转发时将强制使用该 User-Agent
```

Header 命名转换规则：`__user_agent__` → `User-Agent`

