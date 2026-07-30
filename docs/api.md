# Cortex API 概览

后端基于 Go/Gin，产品业务接口统一使用 `/api/v1`。

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
| `GET` | `/readyz` | 否 | 数据库、Embedding、Reranker 与菜谱索引就绪检查 |
| `GET` | `/metrics` | 否 | Prometheus 文本指标，不包含正文或身份信息 |
| `POST` | `/api/v1/auth/register` | 否 | 注册账号并创建个人空间 |
| `POST` | `/api/v1/auth/login` | 否 | 登录并返回 Token |
| `POST` | `/api/v1/auth/logout` | 是 | 撤销当前 Token |

用户名和密码至少 6 个字符，用户名与邮箱唯一。

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

无报告来源时返回 `REPORT_NO_SOURCES`，不会无依据调用 AI。

## AI 配置与通用流式生成

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/v1/settings/ai` | 返回脱敏的全局 AI 配置状态 |
| `POST` | `/api/v1/ai/providers` | 保存当前空间的 Provider 非敏感元数据 |
| `POST` | `/api/v1/ai/stream` | OpenAI 兼容通用 SSE 生成 |

服务端只通过 `AI_API_KEY`、`AI_BASE_URL` 和 `AI_MODEL` 环境变量读取实际调用配置。
Compose 将 LiteLLM 虚拟密钥注入 `AI_API_KEY`；供应商 Key 与网关 master key
不会进入业务后端或前端。未配置虚拟密钥时返回 `AI_NOT_CONFIGURED`。

## 静态 HowToCook 知识库

知识库只由仓库中的 `backend/resources/howtocook` 构成。服务启动时按固定 revision
同步到系统菜谱表并建立索引；用户、研究任务、日报、周报和个人笔记均不能写入知识库。
系统不提供 `/api/v1/knowledge/*` 上传、集合、文档管理或通用知识 Chat 接口。

| 方法与路径 | 说明 |
| --- | --- |
| `GET /api/v1/recipes/today` | 返回按用户、当地日期、语料 revision 和忌口确定性选择的菜谱及 3 个问题 |
| `POST /api/v1/recipes/chat` | 以 SSE 回答烹饪问题；可传 `featured_recipe_id` 锁定今日菜谱上下文 |
| `GET /api/v1/recipes/messages/{message_id}/sources` | 返回只读系统菜谱引用 |
| `GET /api/v1/settings/preferences` | 读取忌口、时区和版本 |
| `PUT /api/v1/settings/preferences` | 以 `version` 乐观锁更新忌口与时区 |

菜谱 Chat 的 SSE 事件顺序如下：

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

会话列表还支持 `search`、`source_scope`、`limit` 和 `offset`，响应为
`{"items":[],"total":0}`。`PATCH /api/v1/conversations/{id}` 使用 `title` 与 `version`
重命名；版本冲突返回 `CONVERSATION_VERSION_CONFLICT`。超过 20 条消息的会话会保存压缩摘要，
回答上下文使用摘要与最近 10 条消息，但事实来源仍在每轮重新检索。

失败使用 `event: error`，`data` 只包含稳定 `code` 和脱敏 `message`。菜谱来源包含
`source_type`、`source_id`、`title`、`rank`、`source_deleted` 与最小 `snippet`。

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
| `GET` | `/api/v1/backups/full` | 下载版本化完整备份 ZIP |
| `POST` | `/api/v1/backups/full/restore` | 将完整备份 ZIP 恢复到空租户 |

Markdown ZIP 只用于内容交换。完整备份使用 `cortex-full-backup-v1` 格式，包含笔记、标签、
版本、附件、定时报告以及研究来源、草稿和资产；恢复旧备份时会忽略其中的个人知识数据及
研究知识关联字段；不包含 Token、AI Provider、
用量、敏感审计、小红书 Cookie 或授权尝试。恢复会重新分配并映射资源 ID，校验 ZIP 路径和
文件 SHA-256，且只允许目标租户为空时执行。数据库与文件卷的基础设施灾备仍由部署者负责。
备份超过目标租户的笔记或附件配额时返回 `BACKUP_RESTORE_QUOTA_EXCEEDED`。

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
| `POST` | `/api/v1/research/sources/{source_id}/ignore` | 忽略待确认结果 |
| `POST` | `/api/v1/research/sources/batch-ignore` | 批量忽略待确认结果 |
| `GET` | `/api/v1/research/assets/{asset_id}` | 鉴权预览来源图片 |
| `GET` | `/api/v1/research/xhs/authorization` | 查询当前租户授权状态（不返回会话凭据） |
| `POST` | `/api/v1/research/xhs/authorizations` | 创建限时扫码授权任务 |
| `GET` | `/api/v1/research/xhs/authorizations/{attempt_id}` | 查询扫码任务状态 |
| `GET` | `/api/v1/research/xhs/authorizations/{attempt_id}/qr` | 鉴权获取登录二维码页面图片，禁止缓存 |
| `POST` | `/api/v1/research/xhs/authorizations/{attempt_id}/cancel` | 取消扫码授权任务 |
| `POST` | `/api/v1/research/xhs/authorization/verify` | 联网验证当前租户授权 |
| `DELETE` | `/api/v1/research/xhs/authorization` | 撤销授权并取消运行中的研究任务 |

授权状态响应中的 `requires_reauthorization=true` 表示现有加密会话是旧格式，必须重新扫码以
采集当前租户所需的受限浏览器状态；仅调用验证接口或重试旧任务不会升级会话格式。

授权接口只返回状态元数据，不返回 Cookie、密文、nonce 或服务器文件路径。创建授权返回
`202` 和扫码任务对象；同一租户已有未结束的扫码任务时幂等返回该任务，便于页面刷新后恢复。
二维码尚未生成时返回 `XHS_QR_PENDING`，任务结束或超时后返回
`XHS_QR_EXPIRED`。二维码响应为 `image/png`，并带有
`Cache-Control: no-store, private`。

关键词研究要求当前租户存在可解密的 `authorized` 会话，否则返回
`XHS_AUTH_REQUIRED`；授权功能未配置时返回 `XHS_AUTH_NOT_CONFIGURED`。公开 URL 模式
可以在无授权时尝试匿名采集。验证失败会把授权标记为 `expired`，撤销会清除会话密文并
取消当前租户运行中的研究任务。

授权状态响应示例：

```json
{
  "id": 12,
  "status": "authorized",
  "account_display_name": null,
  "authorized_at": "2026-07-28T06:52:00Z",
  "last_verified_at": "2026-07-28T06:52:00Z",
  "expires_at": null,
  "failure_code": null,
  "version": 2,
  "created_at": "2026-07-28T06:50:00Z",
  "updated_at": "2026-07-28T06:52:00Z"
}
```

创建或查询扫码任务返回：

```json
{
  "id": "8feaa44b-44c8-45a3-a817-f86cc6942781",
  "authorization_id": 12,
  "status": "waiting_for_scan",
  "failure_code": null,
  "expires_at": "2026-07-28T06:53:00Z",
  "created_at": "2026-07-28T06:50:00Z",
  "updated_at": "2026-07-28T06:50:04Z"
}
```

`status` 的扫码取值为 `queued`、`starting`、`waiting_for_scan`、`scanned`、
`verification_required`、`authorized`、`failed`、`cancelled` 或 `expired`。

创建任务时 `mode` 为 `keyword` 或 `urls`。关键词模式提交 `keywords`，链接模式提交
`urls`；两种模式均提交 `target_count` 和幂等键 `idempotency_key`，可选
研究任务不接受目标知识集合。关键词模式可选 `search_sort`，仅接受 `general`、
`time_descending` 或 `popularity_descending`，未知值按 `general` 处理。客户端提交的
`tenant_id` 不参与租户选择。

研究任务状态为 `queued`、`collecting`、`extracting`、`organizing`、`reviewing`、
`completed`、`failed` 或 `cancelled`。研究结果在用户确认前保持草稿状态。AI、OCR
或采集授权不可用时返回稳定错误码，不返回第三方响应正文、页面 HTML 或内部地址。

研究来源还返回 `published_at`、`like_count`、`collect_count` 和 `comment_count`。
互动字段只包含公开计数，不采集评论正文。浏览器采集会区分
`XHS_AUTH_REQUIRED`、`XHS_VERIFICATION_REQUIRED`、`XHS_RATE_LIMITED`、
`XHS_SOURCE_NOT_FOUND` 和 `XHS_LAYOUT_CHANGED`；限流任务通过数据库
`available_at` 延迟重试。

来源诊断字段包括 `parse_strategy`、`content_completeness`（0–100）、
`ocr_contribution_chars`、`formatted_content` 和 `format_status`。`format_status` 为
`deterministic`、`ai_formatted`、`ai_unavailable` 或 `ai_failed`。AI 不可用或格式化
失败时保留确定性清理后的正文，采集结果仍可人工审核。
