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

通用 AI 流式接口返回 `text/event-stream`。知识 Chat 使用具名的
`retrieval`、`delta`、`sources`、`error` 和 `done` 事件：

```text
data: {"content":"文本片段"}

data: [DONE]
```

## 系统与认证

| 方法 | 路径 | 认证 | 说明 |
| --- | --- | --- | --- |
| `GET` | `/healthz` | 否 | 进程存活检查 |
| `GET` | `/readyz` | 否 | 数据库就绪检查 |
| `GET` | `/metrics` | 否 | Prometheus 文本指标，不包含正文或身份信息 |
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

## AI 整理与报告

所有生成内容先作为草稿返回，只有确认接口会修改笔记。报告会持久化来源关系。

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/api/v1/ai/organize` | 将快速记录整理为结构化草稿，不写库 |
| `POST` | `/api/v1/ai/organize/confirm` | 用户编辑确认后创建或更新笔记 |
| `POST` | `/api/v1/reports/preview` | 计算周期范围并返回候选来源 |
| `POST` | `/api/v1/reports/generate` | 仅基于候选来源流式生成报告草稿 |
| `POST` | `/api/v1/reports/confirm` | 保存报告、来源及明确的覆盖选择 |
| `GET` | `/api/v1/reports/{note_id}/sources` | 查询报告来源 |

无报告来源时返回 `REPORT_NO_SOURCES`，不会无依据调用 AI。笔记问答统一使用成长助手的
`/api/v1/knowledge/chat`，并传入 `source_scope: "growth"`。

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
| `DELETE` | `/api/v1/knowledge/collections/{collection_id}` | 删除空集合；非空返回 `COLLECTION_NOT_EMPTY` |
| `GET` / `POST` | `/api/v1/knowledge/documents` | 分页查询或上传知识文件 |
| `GET` | `/api/v1/knowledge/documents/{document_id}` | 获取文件与索引状态 |
| `GET` | `/api/v1/knowledge/documents/{document_id}/download` | 鉴权下载原文件 |
| `GET` | `/api/v1/knowledge/documents/{document_id}/preview` | 获取受限的提取预览 |
| `POST` | `/api/v1/knowledge/documents/{document_id}/reindex` | 重新加入索引队列 |
| `DELETE` | `/api/v1/knowledge/documents/{document_id}` | 删除原文件并立即使索引失效 |
| `POST` | `/api/v1/knowledge/chat` | 在指定集合/文件范围内检索并以 SSE 回答 |
| `GET` | `/api/v1/knowledge/messages/{message_id}/sources` | 查询回答引用的知识来源 |

文件列表支持 `collection_id`、`search`、`status`、`limit` 和 `offset` 查询参数。
`status` 可为 `uploaded`、`extracting`、`indexing`、`ready` 或 `failed`；进入删除流程的
文件立即从普通列表消失。

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/v1/conversations` | 查询知识、成长与全部来源会话 |
| `POST` | `/api/v1/conversations` | 使用 `title` 和 `source_scope` 新建会话 |
| `GET` | `/api/v1/conversations/{conversation_id}` | 读取会话及消息 |
| `DELETE` | `/api/v1/conversations/{conversation_id}` | 删除会话及其消息和引用 |

会话使用 `/api/v1/conversations` 的列表、新建、读取和删除接口。Chat 请求包含
`question`、幂等键 `request_id`、`source_scope`（`knowledge`、`growth` 或 `all`），
可选 `conversation_id`、`collection_ids` 和 `document_ids`。省略 `request_id` 时沿用
服务端生成的请求追踪 ID。重试相同键会重放已保存回答，不会产生重复消息。
检索采用经 LiteLLM 调用本地 `qwen3-embedding:0.6b` 得到的向量召回与 PostgreSQL
全文召回进行混合排序，再使用可选的 `Qwen/Qwen3-Reranker-0.6B` 重排；回答只能依据返回的
父块上下文。无证据时返回 `KNOWLEDGE_NO_EVIDENCE`，不会调用生成模型。Embedding
不可用时降级为 FTS，不绕过 LiteLLM 切换调用路径。

知识 Chat 的 SSE 事件顺序如下：

```text
event: retrieval
data: {"count":2,"items":[...]}

event: delta
data: {"content":"增量文本"}

event: sources
data: {"items":[...]}

event: done
data: {"conversation_id":12,"message_id":34}
```

失败使用 `event: error`，`data` 只包含稳定 `code` 和脱敏 `message`。来源统一包含
`source_type`、`source_id`、`title`、`rank` 和 `source_deleted`，知识文件还可包含
`heading`、`page_from`、`page_to` 与最小 `snippet`。

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

## 小红书研究

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/api/v1/research/jobs` | 按关键词或公开链接创建研究任务 |
| `GET` | `/api/v1/research/jobs` | 分页查询研究任务 |
| `GET` | `/api/v1/research/jobs/{job_id}` | 查询任务状态与持久化进度 |
| `POST` | `/api/v1/research/jobs/{job_id}/cancel` | 请求取消运行中任务 |
| `POST` | `/api/v1/research/jobs/{job_id}/retry` | 将失败或取消的任务重新排队 |
| `GET` | `/api/v1/research/sources` | 筛选、搜索和分页查询研究结果 |
| `GET` | `/api/v1/research/sources/{source_id}` | 查询来源、图片、OCR 和整理草稿 |
| `DELETE` | `/api/v1/research/sources/{source_id}` | 软删除来源并使关联知识文档停止检索 |
| `POST` | `/api/v1/research/sources/{source_id}/retry` | 为失败来源创建重新采集任务 |
| `POST` | `/api/v1/research/sources/{source_id}/recollect` | 为来源创建重新采集任务 |
| `PATCH` | `/api/v1/research/sources/{source_id}/draft` | 使用版本号更新整理草稿 |
| `POST` | `/api/v1/research/sources/{source_id}/save` | 确认并保存到个人知识库 |
| `POST` | `/api/v1/research/sources/{source_id}/ignore` | 忽略待确认结果 |
| `POST` | `/api/v1/research/sources/batch-save` | 批量保存待确认结果 |
| `POST` | `/api/v1/research/sources/batch-ignore` | 批量忽略待确认结果 |
| `GET` | `/api/v1/research/assets/{asset_id}` | 鉴权预览来源图片 |

创建任务时 `mode` 为 `keyword` 或 `urls`。关键词模式提交 `keywords`，链接模式提交
`urls`；两种模式均提交 `target_count` 和幂等键 `idempotency_key`，可选
`target_collection_id`。客户端提交的 `tenant_id` 不参与租户选择。

研究任务状态为 `queued`、`collecting`、`extracting`、`organizing`、`reviewing`、
`completed`、`failed` 或 `cancelled`。研究结果在用户确认前保持草稿状态。AI、OCR
或采集授权不可用时返回稳定错误码，不返回第三方响应正文、页面 HTML 或内部地址。

## 旧版兼容接口

以下接口仍由后端提供，但已不属于重构后主流程：

- `/api/chat/`、`/api/chat/conversations/`：旧版同步聊天。
- `/api/diary/`：旧版图片轻日记。

新功能应优先使用 `/api/v1` 笔记、AI 整理、周期报告和回忆接口。
