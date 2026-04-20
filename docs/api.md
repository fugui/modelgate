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

