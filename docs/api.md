# ModelGate API 接口文档

模界（Model Gate）网关提供两类核心接口：
1. **LLM 代理接口（Proxy APIs）**：兼容 OpenAI 标准协议及 Responses API，用于与下游大模型通信。
2. **管理与用户接口（Management APIs）**：RESTful 风格，用于控制台、用户自助服务与管理员运维。

---

## 一、LLM 代理接口

所有 LLM 代理接口均通过 HTTP Header 进行鉴权：
```http
Authorization: Bearer <MODELGATE_API_KEY>
```

### 1. 模型列表接口
获取当前可调用的模型列表。

- **请求方式**：`GET /v1/models`
- **响应示例**：
```json
{
  "object": "list",
  "data": [
    {
      "id": "kimi2.5",
      "object": "model",
      "created": 1710000000,
      "owned_by": "modelgate"
    },
    {
      "id": "glm4.7",
      "object": "model",
      "created": 1710000000,
      "owned_by": "modelgate"
    }
  ]
}
```

### 2. Chat Completions 接口 (OpenAI 兼容)
兼容 OpenAI `/v1/chat/completions` 标准协议，支持流式响应（SSE）、工具调用（Function Calling）、思考链（Reasoning）透传。

- **请求方式**：`POST /v1/chat/completions`
- **Content-Type**：`application/json`
- **非流式请求示例**：
```json
{
  "model": "kimi2.5",
  "messages": [
    {"role": "system", "content": "You are a helpful assistant."},
    {"role": "user", "content": "你好，请介绍一下你自己。"}
  ],
  "temperature": 0.7,
  "max_tokens": 2048,
  "stream": false
}
```
- **流式请求示例**：
```json
{
  "model": "kimi2.5",
  "messages": [
    {"role": "user", "content": "写一段 Go 语言的 Hello World。"}
  ],
  "stream": true
}
```

### 3. Responses API 接口 (OpenCode / Codex 直通)
针对 OpenCode、Codex CLI 等 Agent 工具的原生 `/v1/responses` 接口。

- **请求方式**：`POST /v1/responses`
- **Content-Type**：`application/json`
- **请求示例**：
```json
{
  "model": "kimi2.5",
  "instructions": "You are an expert coding assistant.",
  "input": "请帮我重构这段函数",
  "stream": true
}
```

---

## 二、用户与认证接口

### 1. 用户登录
- **请求方式**：`POST /api/v1/auth/login`
- **请求体**：
```json
{
  "email": "user@example.com",
  "password": "your-password"
}
```
- **响应体**：
```json
{
  "code": 200,
  "data": {
    "token": "eyJhbGciOiJIUzI1NiIs...",
    "user": {
      "id": "b822e407-b565-4088-9071-9666cfa3ac15",
      "email": "user@example.com",
      "name": "张三",
      "role": "user"
    }
  }
}
```

### 2. 用户注册
当配置开启 `frontend.registration_enabled` 时可用（注册成功后需管理员审核启用）。

- **请求方式**：`POST /api/v1/auth/register`
- **请求体**：
```json
{
  "email": "newuser@example.com",
  "password": "secure-password",
  "name": "新用户"
}
```

### 3. SSO 单点登录（如果启用）
- `GET /api/v1/auth/sso/config`：获取前端 SSO 配置（Client ID、Authorize URL 等）
- `GET /api/v1/auth/sso/login`：发起 SSO 重定向登录
- `GET /api/v1/auth/sso/callback`：SSO 授权回调接口

### 4. 获取前端公开配置
- `GET /api/v1/config/frontend`：获取反馈链接、开发手册链接及注册开放状态。

---

## 三、用户自助服务接口

以下接口需携带 JWT Token：
```http
Authorization: Bearer <JWT_TOKEN>
```

| 接口 | 方法 | 说明 |
| :--- | :--- | :--- |
| `/api/v1/user/profile` | GET | 获取当前用户个人信息与权限角色 |
| `/api/v1/user/quota` | GET | 获取当前用户配额使用情况与剩余额度 |
| `/api/v1/user/usage` | GET | 获取个人近期 Token 消耗统计 |
| `/api/v1/user/access-logs` | GET | 获取个人 API 访问日志流水 |
| `/api/v1/user/password` | PUT | 修改当前登录密码 |
| `/api/v1/user/keys` | GET | 列出当前名下的所有 API Key |
| `/api/v1/user/keys` | POST | 创建新的 API Key（可绑定允许模型、过期时间） |
| `/api/v1/user/keys/:id` | DELETE | 删除指定的 API Key |

---

## 四、管理员运维管理接口

管理员接口仅限 `role = "admin"` 的用户访问：

### 1. 用户管理
| 接口 | 方法 | 说明 |
| :--- | :--- | :--- |
| `/api/v1/admin/users` | GET | 分页获取所有用户列表（含待审核/禁用状态） |
| `/api/v1/admin/users` | POST | 手动创建用户 |
| `/api/v1/admin/users/:id` | PUT | 更新用户信息（修改配额策略、重置密码、启用/禁用、审核通过） |
| `/api/v1/admin/users/:id` | DELETE | 删除指定用户 |

### 2. 模型与后端实例管理
| 接口 | 方法 | 说明 |
| :--- | :--- | :--- |
| `/api/v1/admin/models` | GET | 获取网关配置的所有模型及其状态 |
| `/api/v1/admin/models` | POST | 新增模型配置 |
| `/api/v1/admin/models/:id` | PUT | 更新模型属性、参数与上下文窗口 |
| `/api/v1/admin/models/:id` | DELETE | 删除模型配置 |
| `/api/v1/admin/models/health` | GET | 获取所有后端实例的实时健康探测状态 |
| `/api/v1/admin/loadbalancer/status` | GET | 获取负载均衡器健康与选路状态 |
| `/api/v1/admin/models/:id/backends` | GET | 获取某模型下的全部后端实例列表 |
| `/api/v1/admin/models/:id/backends` | POST | 为指定模型添加后端实例 |
| `/api/v1/admin/models/:id/backends/:backend_id` | PUT | 修改指定后端实例（权重、URL、APIKey、并发上限等） |
| `/api/v1/admin/models/:id/backends/:backend_id` | DELETE | 移除指定后端实例 |

### 3. 配额策略管理
| 接口 | 方法 | 说明 |
| :--- | :--- | :--- |
| `/api/v1/admin/policies` | GET | 获取所有配额策略列表 |
| `/api/v1/admin/policies` | POST | 创建新配额策略 |
| `/api/v1/admin/policies/:name` | PUT | 修改策略（每分钟限速、每日限额、可用时间段、默认降级模型） |
| `/api/v1/admin/policies/:name` | DELETE | 删除配额策略 |

### 4. 统计与监控看板
| 接口 | 方法 | 说明 |
| :--- | :--- | :--- |
| `/api/v1/dashboard/stats` | GET | 汇总统计（今日请求数、Token 消耗、活跃用户等） |
| `/api/v1/dashboard/trends` | GET | 趋势分析数据（请求量曲线、模型消耗分布） |
| `/api/v1/dashboard/logs` | GET | 全局审计日志（支持按状态码、用户、模型筛选） |
| `/api/v1/admin/concurrency/stats` | GET | 当前实时并发统计信息 |
| `/api/v1/admin/cache/stats` | GET | 本地缓存命中率与容量监控 |
| `/api/v1/admin/config/system` | GET/PUT | 查看与动态更新系统超时等运行配置 |
