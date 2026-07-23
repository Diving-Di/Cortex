# Diary Listener API 概览

后端基于 FastAPI，当前产品接口以 `/api/v1` 为主；认证接口同时提供无版本前缀的兼容路径。完整且可执行的请求/响应 Schema 请以运行时 Swagger UI（`/docs`）或 OpenAPI JSON（`/openapi.json`）为准。

## 通用约定

除注册、登录和健康检查外，请求均使用 Token 认证：

```http
Authorization: Token <token>
```

每个账号自动关联唯一个人空间。租户由服务端根据 Token 解析，业务接口不接受 `tenant_id`。错误响应使用稳定错误码；跨租户资源不会被返回。

AI 流式接口返回 `text/event-stream`：

```text
data: {"content":"文本片段"}

data: [DONE]
```

## 系统与认证

| 方法 | 路径 | 认证 | 说明 |
| --- | --- | --- | --- |
| `GET` | `/healthz` | 否 | 进程存活检查 |
| `GET` | `/readyz` | 否 | 数据库就绪检查 |
| `POST` | `/api/v1/auth/register` | 否 | 注册账号并创建个人空间 |
| `POST` | `/api/v1/auth/login` | 否 | 登录并返回 Token |
| `POST` | `/api/v1/auth/logout` | 是 | 撤销当前 Token |

认证接口兼容路径为 `/api/register/`、`/api/login/` 和 `/api/logout/`。用户名和密码至少 6 个字符，用户名与邮箱唯一。

## 个人空间与工作台

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/v1/tenant` | 获取空间状态、配额与用量 |
| `PATCH` | `/api/v1/tenant` | 修改空间显示名称 |
| `DELETE` | `/api/v1/tenant` | 软删除个人空间 |
| `POST` | `/api/v1/tenant/restore` | 恢复个人空间 |
| `GET` | `/api/dashboard?timezone=Asia/Shanghai` | 获取工作台统计摘要 |

## 笔记与版本

笔记类型包括 `normal`、`daily`、`weekly`、`monthly`。周报日期归一到所在周周一，月报日期归一到当月一日；同一空间、类型和周期只允许一份未删除的周期笔记。

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/v1/notes` | 分页查询；支持类型、日期范围等筛选 |
| `POST` | `/api/v1/notes` | 创建笔记 |
| `GET` | `/api/v1/notes/{note_id}` | 获取笔记 |
| `PATCH` | `/api/v1/notes/{note_id}` | 更新笔记并保留历史正文 |
| `DELETE` | `/api/v1/notes/{note_id}` | 软删除笔记 |
| `GET` | `/api/v1/notes/{note_id}/revisions` | 获取版本历史 |
| `POST` | `/api/v1/notes/{note_id}/revisions/{revision_id}/restore` | 恢复指定版本 |

## 标签、附件与搜索

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` / `POST` | `/api/v1/tags` | 查询或创建标签 |
| `GET` / `PUT` | `/api/v1/notes/{note_id}/tags` | 查询或替换笔记标签 |
| `POST` | `/api/v1/attachments?note_id={note_id}` | 上传附件 |
| `GET` | `/api/v1/attachments/note/{note_id}` | 查询笔记附件 |
| `GET` | `/api/v1/attachments/{attachment_id}` | 鉴权下载附件 |
| `DELETE` | `/api/v1/attachments/{attachment_id}` | 删除附件 |
| `GET` | `/api/v1/search` | 按关键词、类型、日期和标签检索 |

附件允许 PNG、JPEG、PDF、UTF-8 TXT 和 Markdown，默认单文件上限为 20 MiB。文件不通过公开静态路径访问。

## AI 整理、报告与回忆

所有生成内容先作为草稿返回，只有确认接口会修改笔记。报告和回忆回答均持久化来源关系。

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/api/v1/ai/organize` | 将快速记录整理为结构化草稿，不写库 |
| `POST` | `/api/v1/ai/organize/confirm` | 用户编辑确认后创建或更新笔记 |
| `POST` | `/api/v1/reports/preview` | 计算周期范围并返回候选来源 |
| `POST` | `/api/v1/reports/generate` | 仅基于候选来源流式生成报告草稿 |
| `POST` | `/api/v1/reports/confirm` | 保存报告、来源及明确的覆盖选择 |
| `GET` | `/api/v1/reports/{note_id}/sources` | 查询报告来源 |
| `POST` | `/api/v1/memory/chat` | 检索当前空间笔记并流式回答 |
| `GET` | `/api/v1/memory/messages/{message_id}/sources` | 查询回答引用 |

无报告来源时返回 `REPORT_NO_SOURCES`，无回忆证据时返回 `MEMORY_NO_EVIDENCE`，两种情况都不会无依据调用 AI。

## AI 配置与通用流式生成

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/v1/settings/ai` | 返回脱敏的全局 AI 配置状态 |
| `POST` | `/api/v1/ai/providers` | 保存当前空间的 Provider 非敏感元数据 |
| `POST` | `/api/v1/ai/stream` | OpenAI 兼容通用 SSE 生成 |

服务端通过 `AI_API_KEY`、`AI_BASE_URL` 和 `AI_MODEL`（或被忽略的 `backend/config.json`）读取实际调用凭据。API Key 不会返回前端；未配置时返回 `AI_NOT_CONFIGURED`。

## 导出、备份与恢复

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/api/v1/exports/markdown` | 下载全部有效笔记的 Markdown ZIP |
| `POST` | `/api/v1/backups` | 创建带 SHA-256 清单的完整备份 ZIP |
| `POST` | `/api/v1/backups/restore` | 将备份恢复到空个人空间 |

恢复是受控操作：目标空间必须为空，上传包具有大小限制，并会校验清单及租户边界。

## 旧版兼容接口

以下接口仍由后端提供，但已不属于重构后主流程：

- `/api/chat/`、`/api/chat/conversations/`：旧版同步聊天。
- `/api/diary/`：旧版图片轻日记。

新功能应优先使用 `/api/v1` 笔记、AI 整理、周期报告和回忆接口。
