# 模界（Model Gate）- 企业大模型统一接入网关

[![Go Version](https://img.shields.io/badge/Go-1.22+-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

模界（Model Gate）是一个为企业内部提供统一大模型服务入口的接入网关，实现用户管理、权限控制、配额计费、多后端负载均衡和审计追踪。

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

详见 [配置说明文档](docs/config.md)。

## API 接口

详见 [API 接口文档](docs/api.md)。

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

详见 [部署文档](docs/deployment.md)。

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

### v0.7.0 (2026-04)
- ✨ 新增对 Anthropic 协议的高级转换与兼容，完美支持 Claude Code 的 Tool Use 机制
- ✨ 引入并发安全的无损 RAW Dump 调试诊断功能，支持 `debug_raw_payloads` 配置（`none`, `error`, `full`）
- 🐛 修复了后端 LLM 返回 `tool_calls` 时流式响应的提前 `stop` 异常
- ♻️ 增强大模型工具容错，执行失败的 Tool 强制注入 `[Error]` 前缀标识
- ♻️ 拆分重组文档结构，将配置、API和部署说明归档入 `docs` 目录

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
