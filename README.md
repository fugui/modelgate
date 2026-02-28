# LLMGate - 企业内部 LLM 管理平台

[![Go Version](https://img.shields.io/badge/Go-1.22+-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

LLMGate 是一个为企业内部提供统一 LLM 服务入口的管理平台，实现用户管理、权限控制、配额计费和审计追踪。

## 核心功能

- **用户管理**：JWT 认证、角色管理（管理员/经理/普通用户）
- **API Key 管理**：用户自助创建/删除 API Key，支持过期时间和模型限制
- **模型管理**：支持多个后端 LLM 服务器的负载均衡
- **配额控制**：速率限制、Token 配额、并发控制
- **审计日志**：完整的请求日志（7天自动清理）
- **OpenAI 兼容**：提供与 OpenAI API 兼容的接口

## 技术栈

- **后端**: Go 1.22+ + Gin
- **数据库**: SQLite (本地开发) / PostgreSQL (生产)
- **缓存**: 内存 (本地开发) / Redis (生产)
- **部署**: 单二进制文件 / Docker

## 快速开始

### 环境要求

- Go 1.22 或更高版本
- Make (可选，用于快捷命令)

### 本地开发

```bash
# 克隆仓库
git clone https://github.com/fugui/llmgate.git
cd llmgate

# 运行服务
go run cmd/server/main.go

# 或使用 Makefile
make run
```

服务启动后访问：http://localhost:8080

### 使用 Docker

```bash
# 构建镜像
docker build -t llmgate .

# 运行
docker run -p 8080:8080 llmgate
```

### 默认管理员账号

- **邮箱**: admin@llmgate.local
- **密码**: admin123

**注意**：首次登录后请立即修改默认密码。

## 测试

项目使用**场景驱动测试**，验证核心业务逻辑：

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
- ✅ 配额限制（日配额超限、多模型配额）
- ✅ API Key 生命周期（过期、禁用、用户禁用）
- ✅ 速率限制与并发控制

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
```

### API Key 管理

```bash
# 创建 API Key（需登录）
POST /api/v1/user/keys
Authorization: Bearer <jwt-token>
Content-Type: application/json

{
  "name": "开发测试",
  "models": ["llama3-70b"],
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
  "model": "llama3-70b",
  "messages": [
    {"role": "user", "content": "Hello, how are you?"}
  ],
  "temperature": 0.7,
  "max_tokens": 1000
}
```

### 管理接口

```bash
# 用户管理
GET    /api/v1/admin/users          # 列出用户
POST   /api/v1/admin/users          # 创建用户
PUT    /api/v1/admin/users/:id      # 更新用户
DELETE /api/v1/admin/users/:id      # 删除用户

# 模型管理
GET    /api/v1/admin/models
POST   /api/v1/admin/models
PUT    /api/v1/admin/models/:id
DELETE /api/v1/admin/models/:id

# 配额策略管理
GET    /api/v1/admin/policies
POST   /api/v1/admin/policies
PUT    /api/v1/admin/policies/:name
DELETE /api/v1/admin/policies/:name
```

## 配置说明

### config.yaml

```yaml
server:
  port: 8080              # 服务端口
  mode: "release"         # debug 或 release

database:
  path: "llmgate.db"      # SQLite 数据库文件路径

logs:
  path: "./logs"          # 日志目录
  retention_days: 7       # 日志保留天数

jwt:
  secret: "your-jwt-secret-change-in-production"
  expire_hours: 24

admin:
  default_email: "admin@llmgate.local"
  default_password: "admin123"

# LLM 后端配置
models:
  - id: "llama3-70b"
    name: "Llama 3 70B"
    backend: "http://llm-server-1:8000"
    enabled: true
    weight: 1
  - id: "qwen-72b"
    name: "Qwen 72B"
    backend: "http://llm-server-2:8000"
    enabled: true
    weight: 1

# 配额策略
quota_policies:
  - name: "default"
    rate_limit: 60              # 每分钟请求数
    rate_limit_window: 60       # 窗口秒数
    token_quota_daily: 100000   # 每日 Token 上限
    models: ["*"]               # "*" 表示所有模型
```

## 项目结构

```
llmgate/
├── cmd/
│   └── server/              # 主程序入口
├── internal/                # 内部包（不可外部导入）
│   ├── apikey/             # API Key 管理
│   ├── auth/               # JWT 认证
│   ├── config/             # 配置管理
│   ├── db/                 # 数据库连接
│   ├── logger/             # 日志记录
│   ├── middleware/         # HTTP 中间件
│   ├── model/              # 模型管理
│   ├── models/             # 数据模型定义
│   ├── proxy/              # LLM 代理和负载均衡
│   ├── quota/              # 配额检查
│   ├── usage/              # 使用记录
│   └── user/               # 用户管理
├── test/
│   └── scenarios/          # 场景测试
├── migrations/             # 数据库迁移脚本
├── web/                    # Web 前端（React）
├── config.yaml             # 配置文件
├── Dockerfile              # Docker 构建
├── Makefile                # 快捷命令
└── README.md
```

## 开发计划

- [x] 用户认证（JWT）
- [x] API Key 管理（创建、删除、过期）
- [x] 模型管理
- [x] 配额检查（速率限制、Token 配额）
- [x] 负载均衡（轮询）
- [x] 使用记录和审计日志
- [x] 场景测试
- [x] 用户禁用后 Key 失效
- [ ] Web 管理界面（开发中）
- [ ] 企业 SSO 集成（OIDC/SAML）
- [ ] 实时监控仪表盘
- [ ] 更多负载均衡策略（最少连接、权重）

## 贡献指南

1. Fork 本仓库
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 创建 Pull Request

## 安全

如发现安全问题，请直接联系维护者，不要公开提 Issue。

## License

MIT License - 详见 [LICENSE](LICENSE) 文件

---

**注意**：本项目默认配置仅供开发测试使用，生产环境请务必修改默认密码和 JWT Secret。
