# AI 聊天 & 轻日记 接口明细

后端使用 FastAPI + SQLAlchemy 暴露接口，前端通过 `/api` 访问（开发环境由 Webpack DevServer 代理到后端）。需要登录的接口使用 Token 认证：

```http
Authorization: Token <token>
```

FastAPI 自动文档地址：

- Swagger UI: `/docs`
- OpenAPI JSON: `/openapi.json`

## 前端 URL 包装

前端统一在 `frontend/src/api/urls.ts` 维护接口 URL：

| 包装字段 | URL |
| --- | --- |
| `apiUrls.auth.login` | `/api/login/` |
| `apiUrls.auth.register` | `/api/register/` |
| `apiUrls.chat.send` | `/api/chat/` |
| `apiUrls.chat.conversations` | `/api/chat/conversations/` |
| `apiUrls.chat.conversation(id)` | `/api/chat/conversations/{id}/` |
| `apiUrls.diary.list` | `/api/diary/` |
| `apiUrls.diary.create` | `/api/diary/` |
| `apiUrls.diary.detail(id)` | `/api/diary/{id}/` |

## 认证接口

### 注册

- Method: `POST`
- URL: `/api/register/`
- Body:

```json
{ "username": "username", "email": "user@example.com", "password": "password" }
```

- 规则：用户名和密码至少 6 个字符，用户名与邮箱唯一。

### 登录

- Method: `POST`
- URL: `/api/login/`
- Body:

```json
{ "username": "username", "password": "password" }
```

- Response:

```json
{ "token": "token-value", "username": "username" }
```

## 聊天接口

### 发送消息（多轮对话）

- Method: `POST`
- URL: `/api/chat/`
- 认证：需要
- Body:

```json
{ "content": "你好", "conversation_id": 12 }
```

- `conversation_id` 为空时会自动新建一个对话；后端会带上最近若干条历史消息请求 AI。
- Response:

```json
{
  "conversation_id": 12,
  "title": "你好",
  "user_message": { "id": 1, "role": "user", "content": "你好", "created_at": "..." },
  "assistant_message": { "id": 2, "role": "assistant", "content": "...", "created_at": "..." }
}
```

### 对话列表

- Method: `GET`
- URL: `/api/chat/conversations/`
- 认证：需要
- Response: `Conversation[]`（按更新时间倒序）

### 对话详情（含全部消息）

- Method: `GET`
- URL: `/api/chat/conversations/{id}/`
- 认证：需要
- Response: `ConversationDetail`（含 `messages`）

### 删除对话

- Method: `DELETE`
- URL: `/api/chat/conversations/{id}/`
- 认证：需要
- Response: `204 No Content`

## 日记接口

### 日记列表

- Method: `GET`
- URL: `/api/diary/`
- 认证：需要
- Response: `DiaryEntry[]`（按创建时间倒序）

### 发布日记

- Method: `POST`
- URL: `/api/diary/`
- 认证：需要
- Content-Type: `multipart/form-data`
- Form:

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `content` | string | 否 | 配图文字 |
| `image` | file | 否 | 图片文件（jpg/png/gif/webp/bmp） |

- 规则：`content` 与 `image` 至少提供一个。

### 删除日记

- Method: `DELETE`
- URL: `/api/diary/{id}/`
- 认证：需要
- Response: `204 No Content`

## AI 配置

后端通过 `backend/config.json`（已在 `.gitignore` 中，不会提交）配置 DeepSeek 兼容接口：

```json
{
  "ai": {
    "api_key": "sk-xxx",
    "base_url": "https://api.deepseek.com/v1",
    "model": "deepseek-chat",
    "system_prompt": "你是一个温暖、贴心的 AI 助手。"
  }
}
```

未配置 `api_key` 时，后端会返回本地模拟回复，便于在无凭据时直接体验。

## v1 个人租户与笔记

所有接口从认证 Token 服务端解析唯一的个人租户，不接受 `tenant_id` 参数。

- `GET /api/v1/tenant`：个人空间、笔记配额和 AI Token 用量摘要。
- `PATCH /api/v1/tenant`：修改个人空间显示名称。
- `DELETE /api/v1/tenant`：软删除个人空间。
- `POST /api/v1/tenant/restore`：恢复软删除的个人空间。
- `GET /api/v1/notes`：分页列表；支持 `page`、`page_size`、`type`、`start_date`、`end_date`。
- `POST /api/v1/notes`：创建 `normal`、`daily`、`weekly` 或 `monthly` 笔记。
- `GET/PATCH/DELETE /api/v1/notes/{id}`：读取、更新或软删除笔记。
- `GET /api/v1/notes/{id}/revisions`：历史正文列表。
- `POST /api/v1/notes/{id}/revisions/{revision_id}/restore`：恢复历史正文。

周报日期统一归一到周一，月报日期统一归一到每月一日。同一租户同一周期只允许一份未删除的周期笔记。
