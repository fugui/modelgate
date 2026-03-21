# 模界（Model Gate）- 企业大模型管理平台

[![Go Version](https://img.shields.io/badge/Go-1.22+-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

模界（Model Gate）是一个为企业内部提供统一大模型服务入口的管理平台，实现用户管理、权限控制、配额计费、多后端负载均衡和审计追踪。

## 核心功能

- **用户管理**：JWT 认证、角色管理（管理员/经理/普通用户）、自助注册 + 管理员审核
- **API Key 管理**：用户自助创建/删除 API Key，支持过期时间和模型限制
- **多后端架构**：一个模型可配置多个后端实例，支持权重轮询负载均衡
- **健康检查**：自动检测后端可用性，自动剔除故障节点
- **配额控制**：速率限制、Token 配额、并发控制、可用时间段限制
- **模型参数**：支持自定义模型请求参数（如禁用思考模式、自定义 HTTP Header）
- **并发控制**：全局和用户级并发限制，防止服务过载
- **SSO 支持**：支持 Azure AD 等企业身份提供商
- **本地缓存**：API Key 和用户信息本地缓存，减少数据库查询
- **审计日志**：完整的请求日志（7天自动清理）
- **OpenAI 兼容**：提供与 OpenAI API 兼容的接口
- **多客户端协议**：同时支持 OpenAI 和 Anthropic 客户端，自动转换为后端 LLM 支持的协议
- **单文件部署**：前端资源嵌入二进制，仅需一个可执行文件
- **默认模型 Fallback**：当请求的模型无可用后端时，自动 fallback 到默认模型

## 技术栈

- **后端**: Go 1.22+ + Gin
- **数据库**: SQLite（单文件，零配置）
- **缓存**: 内存（内置实现）
- **前端**: React + TypeScript + Ant Design
- **部署**: 单二进制文件，无需 Docker

## 快速开始

### 环境要求

- Go 1.22 或更高版本（仅编译时需要）
- Node.js 18+（仅编译前端时需要）

### 构建

```bash
# 完整构建（前端 + 后端）
make build

# 构建多平台发布包
make release
```

构建产物：
- `modelgate` - 单个可执行文件（包含前端资源，约 12MB）
- `config.yaml` - 配置文件

### 部署

```bash
# 仅需两个文件即可部署
./modelgate
```

访问 http://localhost:8080 即可使用 Web 管理界面。

### 开发模式

```bash
# 同时启动前端开发服务器和后端服务
make dev

# 前端: http://localhost:5173
# 后端: http://localhost:8080
```

### 默认管理员账号

- **邮箱**: admin@modelgate.local
- **密码**: admin123

**注意**：首次登录后请立即修改默认密码。

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
```

Header 命名转换规则：`__user_agent__` → `User-Agent`

## API 接口

### 认证接口

```bash
# 登录
POST /api/v1/auth/login
Content-Type: application/json

{
  "email": "user@example.com",
  "password": "password"
}

# 响应
{
  "data": {
    "token": "eyJhbGciOiJIUzI1NiIs...",
    "user": {
      "id": "...",
      "email": "user@example.com",
      "name": "User Name",
      "role": "user"
    }
  }
}

# 用户自助注册（需开启 registration_enabled）
POST /api/v1/auth/register
Content-Type: application/json

{
  "email": "newuser@example.com",
  "password": "password123",
  "name": "新用户"
}

# 响应（注册后需管理员审核才能登录）
{
  "message": "注册成功，请等待管理员审核"
}
```

### API Key 管理

```bash
# 创建 API Key（需登录）
POST /api/v1/user/keys
Authorization: Bearer <jwt-token>
Content-Type: application/json

{
  "name": "开发测试",
  "models": ["glm4.7", "kimi2.5"],
  "expires_at": "2024-12-31T23:59:59Z"
}

# 列出 API Keys
GET /api/v1/user/keys
Authorization: Bearer <jwt-token>

# 删除 API Key
DELETE /api/v1/user/keys/:id
Authorization: Bearer <jwt-token>
```

### LLM 代理接口（OpenAI 兼容）

```bash
# 列出可用模型
GET /v1/models
Authorization: Bearer <api-key>

# 聊天补全
POST /v1/chat/completions
Authorization: Bearer <api-key>
Content-Type: application/json

{
  "model": "kimi2.5",
  "messages": [
    {"role": "user", "content": "Hello, how are you?"}
  ],
  "temperature": 0.7,
  "max_tokens": 1000
}
```

### LLM 代理接口（Anthropic 兼容）

系统同时支持 Anthropic API 协议，自动将请求转换为后端 LLM 支持的格式：

```bash
# 消息完成（Anthropic 格式）
POST /v1/messages
Authorization: Bearer <api-key>
Content-Type: application/json
anthropic-version: 2023-06-01

{
  "model": "kimi2.5",
  "messages": [
    {"role": "user", "content": "Hello, how are you?"}
  ],
  "max_tokens": 1000,
  "stream": false
}
```

**支持的 Anthropic 特性：**
- Messages API 格式
- 流式响应（SSE）
- 系统提示词（system prompt）
- 工具调用（function calling）
- 多模态内容（文本 + 图像）
- 思考模式（thinking blocks）

**协议转换说明：**
- Anthropic 请求 → 转换为 OpenAI 格式 → 转发给后端 LLM
- OpenAI 响应 → 转换为 Anthropic 格式 → 返回给客户端
- 自动处理字段映射（roles, content blocks, usage 等）

### 管理接口

#### 用户管理
```bash
GET    /api/v1/admin/users          # 列出用户
POST   /api/v1/admin/users          # 创建用户
PUT    /api/v1/admin/users/:id      # 更新用户
DELETE /api/v1/admin/users/:id      # 删除用户
```

#### 模型管理
```bash
GET    /api/v1/admin/models              # 列出模型
POST   /api/v1/admin/models              # 创建模型
PUT    /api/v1/admin/models/:id          # 更新模型
DELETE /api/v1/admin/models/:id          # 删除模型
GET    /api/v1/admin/models/:id/backends # 获取模型后端列表
```

#### 后端管理
```bash
GET    /api/v1/admin/backends              # 列出所有后端
POST   /api/v1/admin/models/:id/backends   # 为模型添加后端
PUT    /api/v1/admin/backends/:id          # 更新后端
DELETE /api/v1/admin/backends/:id          # 删除后端
POST   /api/v1/admin/backends/:id/health   # 手动健康检查
```

#### 配额策略管理
```bash
GET    /api/v1/admin/policies
POST   /api/v1/admin/policies
PUT    /api/v1/admin/policies/:name
DELETE /api/v1/admin/policies/:name
```

#### SSO 配置
```bash
GET    /api/v1/admin/sso/status     # 获取 SSO 状态
PUT    /api/v1/admin/sso/config    # 更新 SSO 配置
POST   /api/v1/admin/sso/test      # 测试 SSO 连接
```

### 后端数据结构

```typescript
interface Backend {
  id: string;
  model_id: string;
  name: string;
  base_url: string;
  model_name: string;
  weight: number;
  region: string;
  enabled: boolean;
  healthy: boolean;        // 健康状态
  last_check_at: string;   // 最后检查时间
  created_at: string;
  updated_at: string;
}
```

## 项目结构

```
modelgate/
├── cmd/
│   ├── server/              # 主程序入口
│   └── import_users/        # 批量导入用户工具
├── internal/                # 内部包
│   ├── apikey/             # API Key 管理
│   ├── auth/               # JWT 认证
│   ├── config/             # 配置管理
│   ├── db/                 # 数据库（SQLite）
│   ├── logger/             # 日志记录
│   ├── middleware/         # HTTP 中间件
│   ├── model/              # 模型 HTTP 处理
│   ├── models/             # 数据模型定义（Model, Backend, User...）
│   ├── proxy/              # LLM 代理和负载均衡
│   ├── quota/              # 配额检查
│   ├── static/             # 静态文件嵌入
│   ├── usage/              # 使用记录
│   └── user/               # 用户管理
├── web/                    # Web 前端（React + TS + Ant Design）
├── config.yaml             # 配置文件
├── Makefile                # 构建脚本
└── README.md
```

## 负载均衡与健康检查

### 负载均衡策略

模界（Model Gate）采用**权重轮询**算法进行负载均衡：

1. 根据后端 `weight` 值计算选择概率
2. 优先选择健康（`healthy=true`）的后端
3. 如果所有后端都不健康，返回 503 错误

示例：三个后端权重分别为 20, 20, 15，则选择概率为 36%, 36%, 28%

### 健康检查机制

- **自动检查**：系统定期（默认 30 秒）检查所有后端健康状态
- **手动检查**：管理员可通过 API 或 Web 界面触发检查
- **故障转移**：后端标记为不健康后自动剔除，恢复后自动加入
- **检查方式**：向后端发送轻量级探测请求

## 构建命令

```bash
make build      # 构建完整应用（前端 + Go）
make build-go   # 仅构建 Go（不构建前端）
make run        # 构建并运行
make dev        # 开发模式（前后端同时运行）
make release    # 构建多平台发布包
make clean      # 清理构建产物
make test       # 运行测试
```

## 批量导入用户 (CSV)

系统附带了 `import_users` 工具，可通过 CSV 文件快速导入大批用户。

CSV 文件必须包含表头，格式要求如下：
- **必填项**：`email`, `password`, `name`, `role` (可选: `admin`, `manager`, `user`)
- **选填项**：`department`, `quota_policy` (默认为 `default`)

执行示例：
```bash
./import_users -csv import_users_template.csv -config config.yaml
```

## 测试

```bash
# 运行所有场景测试
go test ./test/scenarios/... -v

# 运行特定场景
go test ./test/scenarios/... -v -run TestScenario_UserEndToEndFlow

# 生成覆盖率报告
go test ./test/scenarios/... -cover
```

测试覆盖场景：
- ✅ 用户完整流程（注册→登录→创建Key→调用→扣减）
- ✅ 配额限制（日配额超限、多模型配额、可用时间段）
- ✅ API Key 生命周期（过期、禁用、用户禁用）
- ✅ 速率限制与并发控制
- ✅ 负载均衡与后端故障转移

## 部署建议

### 单机部署

```bash
# 1. 复制两个文件到服务器
scp modelgate config.yaml user@server:/opt/modelgate/

# 2. 使用 systemd 管理服务
sudo systemctl enable --now modelgate
```

### 使用 Nginx 反向代理

```nginx
server {
    listen 80;
    server_name llm.company.com;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        
        # WebSocket 支持（用于流式响应）
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        
        # 超时设置
        proxy_read_timeout 600s;
        proxy_send_timeout 600s;
    }
}
```

### 多实例高可用部署

```
                    ┌─────────────┐
                    │   Nginx     │
                    │  (负载均衡)  │
                    └──────┬──────┘
                           │
           ┌───────────────┼───────────────┐
           │               │               │
      ┌────┴────┐     ┌────┴────┐     ┌────┴────┐
      │Model Gate│     │Model Gate│     │Model Gate│
      │Instance1│     │Instance2│     │Instance3│
      └────┬────┘     └────┬────┘     └────┬────┘
           │               │               │
           └───────────────┼───────────────┘
                           │
                    ┌──────┴──────┐
                    │  SQLite     │
                    │  (共享存储)  │
                    └─────────────┘
```

**注意**：多实例部署需要使用共享存储（如 NFS）存放 SQLite 数据库文件。

## 安全建议

1. **修改默认密码**：首次登录后立即修改管理员密码
2. **更换 JWT Secret**：生产环境务必使用强密钥
3. **启用 HTTPS**：使用 Nginx 或 Caddy 提供 HTTPS
4. **API Key 保护**：不要将 API Key 硬编码在客户端代码中
5. **定期备份**：备份 SQLite 数据库文件
6. **日志审计**：定期检查日志目录的访问记录
7. **网络隔离**：LLM 后端服务应部署在内网，通过 Model Gate 统一暴露
8. **SSO 配置**：如启用 SSO，确保正确配置 issuer_url 和 client_secret

## 版本历史

### v0.6.0 (2026-03)
- ✨ 新增用户自助注册功能（需管理员审核后方可使用）
- ✨ 前端新增注册页面，登录页条件显示注册入口
- ✨ 管理员用户列表显示“待审核”标签
- ✨ 新增 `frontend.registration_enabled` 配置项控制注册开关
- 🐛 修复流式响应下行 Token 统计为 0 的问题
- 🐛 修复 SSE 解析不兼容 `data:` 无空格格式的问题
- 🐛 修复思考模型 `reasoning_content` 未计入 Token 统计

### v0.5.0 (2025-03)
- ✨ 新增配额策略「可用时间段」功能（`available_time_ranges`）
- ✨ 支持多个时间段配置，如仅允许非工作时间使用
- ✨ 支持跨午夜时段（如 `22:00-06:00`）
- ✨ 前端策略管理页面增加时间段动态编辑
- ✨ 向后兼容：不配置时间段等同于全天可用

### v0.4.2 (2025-03)
- ✨ 新增 HTTP 服务器超时配置（读/写/空闲超时、请求头限制）
- ✨ 新增优雅关闭机制，支持 `shutdown_timeout` 配置
- 🔒 增强服务安全性，防止连接泄漏和大请求头攻击

### v0.4.1 (2025-03)
- ✨ 新增默认模型 Fallback 功能（模型无后端时自动切换）
- ✨ 访问日志支持 Claude 格式流式响应解析
- ✨ 支持 Claude 思考块（thinking blocks）显示
- 🐛 修复流式响应 Body 在 stats 页面显示为空的问题
- ♻️ 优化响应体捕获大小限制处理

### v0.4.0 (2025-03)
- ✨ 新增 SSO 单点登录支持（Azure AD / OIDC）
- ✨ 新增并发控制（全局/用户级限制）
- ✨ 新增 `request_quota_daily` 请求配额限制
- ✨ 支持自定义模型 HTTP Headers（双下划线前缀）
- ✨ 新增管理员手动触发后端健康检查
- ✨ **新增 Anthropic API 协议支持**（自动转换为 OpenAI 协议）
- ✨ 支持流式响应转换（SSE）
- ✨ 支持工具调用和多模态内容转换
- ♻️ 优化配额计数逻辑（内存计数器 + 定时持久化）
- ♻️ 重构响应处理，避免 Body 重复读取问题

### v0.3.0 (2025-03)
- ✨ 新增 `model_params` 模型参数配置
- ✨ 支持自定义 User-Agent 和 HTTP Header
- ✨ 支持禁用模型思考模式（如 DeepSeek `enable_thinking: false`）
- ✨ 前端 Chat 界面支持 Markdown 渲染
- ✨ API Key 本地缓存，提升验证性能
- ✨ 配额使用量内存计数器，减少 DB 查询
- ♻️ 优化后端错误码透传（503/504/429）
- ♻️ 增强请求日志（客户端 IP、User-Agent、错误详情）

### v0.2.0 (2025-03)
- ✨ 重构为多后端架构，支持负载均衡
- ✨ 新增后端健康检查机制
- ✨ 支持按地域分配后端
- ♻️ 配置格式更新（向后兼容）

### v0.1.0 (2025-02)
- 🎉 初始版本发布
- ✨ 用户管理、API Key 管理
- ✨ 配额控制和审计日志
- ✨ OpenAI 兼容接口

## License

MIT License - 详见 [LICENSE](LICENSE) 文件

---

**注意**：本项目默认配置仅供开发测试使用，生产环境请务必修改默认密码和 JWT Secret。
