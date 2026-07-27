# Cortex API 概览

后端基于 Go/Gin，当前产品接口以 `/api/v1` 为主；认证和旧聊天/轻日记接口同时提供无版本前缀的兼容路径。

## 通用约定

除注册、登录和健康检查外，请求均使用 Token 认证：

```http
Authorization: Token <token>
```

每个账号自动关联唯一个人空间。租户由服务端根据 Token 解析，业务接口不接受 `tenant_id`。错误响应使用稳定错误码；跨租户资源不会被返回。

客户端可以发送由字母、数字及 `._:-` 组成且不超过 128 字符的
`X-Request-ID`。非法或缺失时服务端生成 UUID，所有响应都通过
`X-Request-ID` 返回最终追踪标识。

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

服务端只通过 `AI_API_KEY`、`AI_BASE_URL` 和 `AI_MODEL` 环境变量读取实际调用配置。
Compose 将 LiteLLM 虚拟密钥注入 `AI_API_KEY`；供应商 Key 与网关 master key
不会进入业务后端或前端。未配置虚拟密钥时返回 `AI_NOT_CONFIGURED`。

## 个人知识库与 RAG Chat

所有知识资源均从服务端认证主体解析租户，客户端不能提交 `tenant_id` 选择空间。上传使用 `multipart/form-data`，字段为 `file`，可选 `collection_id`；支持 `.txt`、文本型 `.pdf` 和 `.docx`，默认上限 50 MiB。文件进入异步索引流程，状态依次为 `uploaded`、`extracting`、`indexing`、`ready` 或 `failed`。

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` / `POST` | `/api/v1/knowledge/collections` | 查询或创建知识集合 |
| `GET` / `POST` | `/api/v1/knowledge/documents` | 分页查询或上传知识文件 |
| `GET` | `/api/v1/knowledge/documents/{document_id}` | 获取文件与索引状态 |
| `GET` | `/api/v1/knowledge/documents/{document_id}/download` | 鉴权下载原文件 |
| `DELETE` | `/api/v1/knowledge/documents/{document_id}` | 删除原文件并立即使索引失效 |
| `POST` | `/api/v1/knowledge/chat` | 在指定集合/文件范围内检索并以 SSE 回答 |
| `GET` | `/api/v1/knowledge/messages/{message_id}/sources` | 查询回答引用的知识来源 |

Chat 请求包含 `question`，可选 `conversation_id`、`collection_ids` 和 `document_ids`。
检索采用经 LiteLLM 调用本地 `qwen3-embedding:0.6b` 得到的向量召回与 PostgreSQL
全文召回进行混合排序，再使用可选的 BGE Reranker v2 M3 重排；回答只能依据返回的
父块上下文。无证据时返回 `KNOWLEDGE_NO_EVIDENCE`，不会调用生成模型。Embedding
不可用时降级为 FTS，不绕过 LiteLLM 切换调用路径。

## 定时报告

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/v1/scheduled-reports` | 查询定时报告任务 |
| `POST` | `/api/v1/scheduled-reports` | 创建 daily、weekly 或 monthly 任务 |
| `PATCH` | `/api/v1/scheduled-reports/{task_id}?enabled={bool}` | 启用或停用任务 |
| `POST` | `/api/v1/scheduled-reports/{task_id}/retry` | 异步立即重试，返回 `queued` |
| `GET` | `/api/v1/scheduled-reports/{task_id}/runs` | 查询最近运行记录 |

任务使用 IANA 时区，调度时间在数据库中保存为 UTC。多个 worker 通过数据库
claim 保证同一到期任务只生成一条运行记录。

## 内容导出

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/api/v1/exports/markdown` | 下载全部有效笔记的 Markdown ZIP |

Markdown ZIP 用于内容交换，不是完整备份。Cortex 不提供应用级完整备份/恢复 API；数据库与文件卷灾备由部署者负责。

## 旧版兼容接口

以下接口仍由后端提供，但已不属于重构后主流程：

- `/api/chat/`、`/api/chat/conversations/`：旧版同步聊天。
- `/api/diary/`：旧版图片轻日记。

新功能应优先使用 `/api/v1` 笔记、AI 整理、周期报告和回忆接口。
