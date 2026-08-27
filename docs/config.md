# ModelGate 配置说明文档

ModelGate 支持通过 `config.yaml` 配置文件及环境变量进行灵活的运行时配置，同时支持核心参数（如模型、后端、配额）的热重载。

---

## 配置文件完整示例 (`config.yaml`)

```yaml
# 1. 服务运行基础配置
server:
  port: 8080                    # 服务监听端口
  mode: "release"               # 运行模式: debug / release / test
  read_timeout: 60s             # 读取请求超时时间（默认 60s）
  write_timeout: 30m            # 写入响应超时（默认 30m，以支持超长流式生成）
  idle_timeout: 300s            # 空闲连接保持超时（默认 300s）
  max_header_bytes: 1048576     # 请求头限制，单位字节（默认 1MB）
  shutdown_timeout: 30s         # 优雅停机超时（默认 30s）

# 2. 数据库配置
database:
  path: "modelgate.db"          # SQLite 数据库文件路径

# 3. 认证安全配置
jwt:
  secret: "your-jwt-secret-key-change-this-in-production"  # JWT 签名密钥
  expire_hours: 24              # 登录令牌有效期（小时）

admin:
  default_email: "admin@modelgate.local"   # 首次启动时自动初始化的管理员账号
  default_password: "admin123"             # 首次管理员密码（生产环境请登录后立即修改）

# 4. 日志与调试诊断
logs:
  path: "./logs"                # 访问日志存储目录
  retention_days: 7             # 日志自动轮转保留天数
  log_payloads: false           # 是否在访问日志中记录完整请求/响应 Body（生产环境建议 false）
  raw_dumps: "none"             # 原始流量诊断转储模式: "none" / "error" / "full"

# 5. 模型及多后端负载均衡
models:
  - id: "kimi2.5"
    name: "Kimi 2.5"
    description: "Moonshot Kimi 2.5 大模型"
    enabled: true
    context_window: 128000        # 模型上下文窗口大小（Token 数）
    model_params:                 # 模型级特定参数注入
      max_tokens: 4096
      temperature: 0.7
      enable_thinking: false      # 禁用思考模式（如 DeepSeek 等）
      __user_agent__: "ModelGate/1.0"   # 双下划线前缀表示自定义 Header
    backends:
      - id: "kimi2.5-gz-01"
        base_url: "https://api.moonshot.cn/v1"
        api_key: "sk-your-moonshot-api-key"
        model_name: "kimi2.5"     # 转发至后端的真实模型名
        weight: 20                # 负载均衡权重
        max_concurrency: 10       # 单实例并发上限（0 为不限制）
        enabled: true
      - id: "kimi2.5-bj-01"
        base_url: "https://api.moonshot.cn/v1"
        api_key: "sk-your-moonshot-api-key"
        model_name: "kimi2.5"
        weight: 15
        max_concurrency: 5
        enabled: true

# 6. 配额策略配置
quota_policies:
  - name: "default"
    rate_limit: 60              # 每分钟请求速率限制 (RPM)
    rate_limit_window: 60       # 限速窗口时长（秒）
    request_quota_daily: 500    # 每日请求配额上限 (RPD)
    available_time_ranges:      # 允许调用的时间段（空表示全天可用）
      - start: "00:00"
        end: "10:00"
      - start: "18:00"
        end: "24:00"
    models: ["*"]               # 授权模型列表（"*" 表示全部）
    default_model: "kimi2.5"    # 默认降级模型（当请求模型无可用后端时自动 Fallback）
    description: "默认用户策略"

  - name: "vip"
    rate_limit: 300
    rate_limit_window: 60
    request_quota_daily: 5000
    models: ["*"]
    description: "VIP 用户策略"

# 7. 前端设置
frontend:
  feedback_url: "https://feedback.example.com"      # 用户反馈外链
  dev_manual_url: "https://docs.example.com"        # 开发手册外链
  registration_enabled: false                       # 是否开放用户注册（需管理员审核）

# 8. SSO 单点登录（可选）
sso:
  enabled: false
  provider: "azure"             # 支持: azure, generic-oidc
  client_id: "your-client-id"
  client_secret: "your-client-secret"
  issuer_url: "https://login.microsoftonline.com/{tenant}/v2.0"
  email_claim: "email"          # JWT 中提取用户邮箱的 Claim 字段

# 9. 客户端访问控制（基于 User-Agent 过滤）
client_filter:
  rules:
    - name: "Claude Code"
      pattern: "claude-code"    # User-Agent 不区分大小写子串匹配
      enabled: true             # true 表示封禁此类客户端
```

---

## 核心配置项说明

### 1. 模型参数注入 (`model_params`)
`model_params` 支持向转发后端的请求中自动注入参数：

1. **请求体参数（Body Params）**：直接配置键值对（如 `max_tokens: 4096`, `temperature: 0.7`, `enable_thinking: false`）。若客户端请求已指定同名参数，则优先尊重客户端入参。
2. **HTTP Header 参数**：以 `__` 双下划线包裹的字段会被映射为 HTTP Header（如 `__user_agent__: "custom-ua"` 会转换为 `User-Agent: custom-ua`），此项会覆盖客户端的同名 Header。

### 2. 后端实例并发控制 (`max_concurrency`)
在每个 `backend` 下配置 `max_concurrency`（最大并发数）：
- 设置为 `0` 表示不作限制。
- 设置大于 0 时，负载均衡器会追踪当前正在执行的活跃请求数；当达到上限时，自动将流量调度至其他可用后端，全部打满时返回 429 提示。

### 3. 可用时间段控制 (`available_time_ranges`)
配额策略支持配置每日开放调用的时间段（如晚高峰或非工作时段使用）：
- 格式为 `HH:MM`（如 `08:30` - `17:30`）。
- 原生支持跨午夜时段配置（如 `22:00` - `06:00`）。
- 若不配置或为空列表，代表全天 24 小时均可调用。
