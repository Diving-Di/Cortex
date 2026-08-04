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

通用 AI 流式接口返回 `text/event-stream`，增量内容格式为：

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

用户名和密码至少 6 个字符，用户名与邮箱唯一。

## 个人空间与工作台

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/v1/tenant` | 获取空间状态、配额与用量 |
| `PATCH` | `/api/v1/tenant` | 修改空间显示名称 |
| `DELETE` | `/api/v1/tenant` | 软删除个人空间 |
| `POST` | `/api/v1/tenant/restore` | 恢复个人空间 |
| `GET` | `/api/dashboard?timezone=Asia/Shanghai` | 获取工作台统计摘要 |

`/api/dashboard` 是当前保留的未版本化工作台接口；其他产品业务接口使用 `/api/v1`。

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

## 个人知识库

知识问答只检索当前租户主动上传的 Markdown、Markdown ZIP 和明确开启知识问答的个人笔记。
历史内置语料已一次性迁移到用户 `Diving` 的运行时私有知识库，不再作为系统级全局语料或应用种子分发。

| 方法与路径 | 说明 |
| --- | --- |
| `POST /api/v1/knowledge/uploads` | 上传一个 `.md` 或 `.zip`，安全落盘后返回 202 |
| `GET /api/v1/knowledge/uploads/{id}` | 查询上传和索引状态 |
| `GET /api/v1/knowledge/documents` | 列出当前租户文档与 3 GiB 配额 |
| `DELETE /api/v1/knowledge/documents/{id}` | 使文档立即退出检索并删除 |
| `PATCH /api/v1/notes/{id}/knowledge` | 开启或关闭笔记知识索引 |
| `POST /api/v1/knowledge/chat/stream` | 在服务端验证的范围内混合检索、精排并 SSE 回答 |

客户端提交的 `tenant_id` 始终被忽略；无当前租户证据时返回 `KNOWLEDGE_NO_EVIDENCE`。

## 旧菜谱兼容接口

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

## 模板广场

公开模板作者必须先设置公开昵称。模板由作者自主上架和下架，不经过管理员审核；公开读取只
访问脱敏快照，不访问其他租户的私有原稿。

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` / `PUT` | `/api/v1/public-profile` | 读取或设置公开昵称 |
| `GET` / `POST` | `/api/v1/templates/mine`、`/api/v1/templates` | 查询或创建私有模板 |
| `GET` / `PATCH` / `DELETE` | `/api/v1/templates/{id}` | 读取、乐观锁更新或软删除自己的模板 |
| `POST` | `/api/v1/templates/{id}/publish` | 作者上架当前版本并生成公开快照 |
| `POST` | `/api/v1/templates/{id}/withdraw` | 作者下架公开快照 |
| `POST` | `/api/v1/templates/{id}/use` | 携带 `Idempotency-Key` 从自己的模板原子创建笔记 |
| `GET` | `/api/v1/templates/public` | 查询公开模板；支持四种 `ranking`、筛选和服务端签名 `cursor` |
| `GET` | `/api/v1/templates/public/{public_id}` | 获取公开模板详情 |
| `PUT` / `DELETE` | `/api/v1/templates/public/{public_id}/like` | 点赞或取消点赞 |
| `PUT` / `DELETE` | `/api/v1/templates/public/{public_id}/favorite` | 收藏或取消收藏 |
| `POST` | `/api/v1/templates/public/{public_id}/use` | 携带 `Idempotency-Key` 原子创建笔记 |
| `POST` | `/api/v1/templates/public/{public_id}/views` | 记录有效浏览 |
| `POST` | `/api/v1/templates/public/{public_id}/reports` | 提交举报反馈，不自动上下架 |

## 每日限量 AI 深度月报

活动按数据库配置的 `Asia/Shanghai` 时间每天 20:00 开放、20:10 关闭，共 10 个名额，固定
消耗 100 点。连续 5 天包含活动当天，当天只计算 20:00 前完成的有效笔记。

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/v1/ai-points/balance` | 查询当月点数、冻结和可用余额 |
| `GET` | `/api/v1/ai-events/current` | 查询当前活动、资格和近似剩余名额 |
| `GET` | `/api/v1/ai-events/history` | 查询近期活动的完全匿名成功名单 |
| `GET` | `/api/v1/ai-events/{event_id}` | 查询指定活动及当前用户资格 |
| `POST` | `/api/v1/ai-events/{event_id}/claims` | 携带 UUID `Idempotency-Key` 领取并排队 |
| `GET` | `/api/v1/ai-events/{event_id}/claims/me` | 查询当前用户本场生成状态和报告 ID |

领取通过 Redis Lua 原子预扣，PostgreSQL 唯一约束与点数账本最终裁决。成功后自动写入月报并
保存 revision 和来源；最终失败返还点数、不返普通名额。

活动时间、持续分钟数、名额、固定点数、连续天数和月度赠送点数保存在
`ai_flash_event_settings`，默认分别为 `Asia/Shanghai` 20:00、10 分钟、10 名、100 点、5 天和
每月 1,000 点。scheduler 预热 Redis 后才开放领取；未预热时领取 fail-closed。
数据库字段 `reservation_ready` 记录本场预热状态；预热失败时活动详情返回 `paused`，恢复后由
worker 重新聚合资格和点数镜像并自动置为就绪。

## 内容导出

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/api/v1/exports/markdown` | 下载全部有效笔记的 Markdown ZIP |
| `GET` | `/api/v1/backups/full` | 下载版本化完整备份 ZIP |
| `POST` | `/api/v1/backups/full/restore` | 将完整备份 ZIP 恢复到空租户 |

Markdown ZIP 只用于内容交换。完整备份使用 `cortex-full-backup-v1` 格式，包含笔记、标签、
版本、附件、定时报告、私有模板、个人收藏以及研究来源、草稿和资产；模板恢复后统一为
`private`。公开快照、公共排名、举报和活动库存不进入备份。恢复旧备份时会忽略其中的个人知识数据及
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
| `DELETE` | `/api/v1/research/sources/{source_id}` | 软删除研究来源并删除关联资产文件 |
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
