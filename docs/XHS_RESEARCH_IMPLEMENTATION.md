# 小红书内容研究功能实现文档

## 1. 文档目的

本文定义 Cortex 新增“小红书研究”功能的完整实现范围、系统边界、数据模型、接口契约、前端交互、安全要求、测试要求和验收流程。

该功能面向个人租户，将用户授权提供或由系统受控采集的公开小红书内容进行解析、OCR、AI 整理、分类、筛选和确认，并保存到 Cortex 个人知识库。本文不按阶段拆分；交付时应完整实现本文列出的必需能力。

参考项目为 `jolekjoker-dot/xhs_research_and_classify`。只参考其“搜索、采集、OCR、格式化、分类、检索”的产品流程，不直接引入其 Python Web 服务、ChromaDB、Markdown 权威存储、供应商直连配置或无租户隔离的接口。

## 2. 目标与非目标

### 2.1 目标

- 在当前前端新增 `/research` 路由和“小红书研究”导航入口。
- 支持通过关键词创建研究任务。
- 支持粘贴一个或多个公开小红书笔记链接创建研究任务。
- 受控采集笔记的公开标题、作者显示名、正文、标签、发布时间、互动统计、图片和来源链接。
- 对图片执行 OCR，并保存可追溯的图片与 OCR 结果。
- 使用现有 LiteLLM 网关对内容生成摘要、关键观点、分类和标签建议。
- 所有 AI 整理结果先形成草稿，由用户确认后写入个人知识库。
- 提供任务进度、结果筛选、详情预览、失败原因、单条重试和批量操作。
- 对重复来源进行租户内去重，支持安全的重新采集。
- 将确认后的研究结果接入现有知识库索引与成长助手引用体系。
- 完整遵循现有认证、租户 RLS、附件、日志、AI 网关和部署安全要求。

### 2.2 非目标

- 不提供桌面客户端或浏览器扩展。
- 不提供团队共享研究任务。
- 不实现绕过验证码、风控、登录保护或访问限制的机制。
- 不采集非公开内容、私信、评论用户资料或其他与研究目标无关的个人数据。
- 不保证第三方平台页面结构永久稳定。
- 不将 Markdown、ChromaDB、Redis 或 Neo4j作为正文权威存储。
- 不通过 iframe 嵌入参考项目 Web UI。
- 不直接连接 DeepSeek、小米或其他模型供应商。

## 3. 总体方案

### 3.1 系统流程

```text
用户创建研究任务
  -> Go API 校验关键词、链接、数量和租户配额
  -> PostgreSQL 持久化任务
  -> Worker 使用租约认领任务
  -> 受控采集器搜索或读取公开笔记
  -> 下载并安全保存图片
  -> OCR 提取图片文字
  -> LiteLLM 生成整理草稿
  -> 用户在 /research 审核
  -> 用户确认写入个人知识库
  -> 现有知识库 Worker 建立索引
  -> 成长助手可在引用校验后检索该内容
```

### 3.2 架构约束

- `backend/cmd/server/main.go` 继续作为唯一后端入口。
- HTTP handler 只负责契约解析和响应；SQL、事务、租约、去重和写入逻辑放入 `backend/internal/store`。
- 新接口统一使用 `/api/v1`。
- PostgreSQL 是任务、来源、草稿和正文的唯一权威来源。
- 所有租户业务查询必须使用 `Store.WithTx`，在同一事务内设置 transaction-local 用户及租户上下文，并保留显式 `tenant_id` 条件。
- AI 请求只能通过现有 `AIClient` 和 LiteLLM 逻辑模型 `diary-default`。
- 研究功能不可用时，不得影响笔记、知识库、搜索、附件、导出、备份、认证、`/healthz` 或非 AI 能力。
- Redis 不是该功能的必需依赖，不在本次实现中引入。

## 4. 前端实现

### 4.1 路由与导航

- 新增受保护路由：`/research`。
- 侧边栏新增“小红书研究”入口，使用与现有 Ant Design 图标体系匹配的研究、搜索或采集图标。
- 路由组件使用懒加载。
- 未登录访问继续由 `ProtectedRoute` 重定向到登录页。
- 窄屏下沿用现有侧边栏折叠规则。

### 4.2 页面结构

页面必须沿用当前 Cortex 的主题变量、Ant Design token、间距、圆角、文字层级、卡片和表格风格，并同时适配浅色、深色和系统主题。

页面由以下区域组成：

1. 页面标题区
   - 标题：“小红书研究”。
   - 说明：“收集公开内容，提炼观点并保存到个人知识库。”
   - “新建研究”主按钮。

2. 新建研究表单
   - 研究方式：关键词搜索、笔记链接。
   - 关键词模式支持一个或多个关键词，显示最大数量限制。
   - 链接模式支持逐行粘贴多个链接。
   - 每个关键词的目标结果数。
   - 目标知识集合，可选；未选择时保存前再次确认。
   - “开始研究”和“取消”操作。
   - 提交前展示公开内容、平台访问和版权提示。

3. 任务区域
   - 展示任务状态：`queued`、`collecting`、`extracting`、`organizing`、`reviewing`、`completed`、`failed`、`cancelled`。
   - 展示已发现、已采集、已整理、失败、已保存数量。
   - 展示创建时间、完成时间、关键词或链接摘要。
   - 运行中任务自动刷新。
   - 支持取消尚未完成的任务。
   - 支持对失败任务执行手动重试；接口立即返回 `queued`。

4. 研究结果区域
   - 筛选：全部、待确认、已保存、已忽略、失败。
   - 支持标题、作者显示名和标签搜索。
   - 支持按采集时间、发布时间和相关度排序。
   - 表格或列表展示标题、来源、分类、状态、采集时间和图片数量。
   - 支持选择多条结果后批量保存或忽略。

5. 详情抽屉
   - 展示来源标题、作者显示名、公开发布时间、来源链接和采集时间。
   - 展示原始正文、OCR 文字、AI 摘要、关键观点、分类和建议标签。
   - 展示图片缩略图；点击后使用受认证的附件预览接口。
   - 明确区分“来源原文”和“AI 整理草稿”。
   - AI 字段允许用户编辑。
   - 支持选择知识集合。
   - 支持“保存到知识库”“忽略”“重新采集”和失败重试。

### 4.3 前端状态与错误处理

- 使用 React Query 管理任务、列表、详情和 mutation。
- 运行中的任务按后端建议间隔轮询，页面不可见时降低或暂停轮询。
- API 错误根据稳定 `code` 显示可执行提示，不展示上游响应正文或内部地址。
- 401 清理本地认证状态并跳转登录。
- 403 显示租户不可用提示。
- 404 统一表现为资源不存在。
- 409 显示内容已被其他操作更新，并刷新详情。
- AI 未配置时允许完成采集和人工整理，但页面应明确显示 AI 整理不可用。
- 页面需提供空状态、加载骨架、部分失败状态和移动端可用布局。

### 4.4 建议的前端文件

```text
frontend/src/features/research/ResearchPage.tsx
frontend/src/features/research/ResearchPage.css
frontend/src/features/research/ResearchPage.test.tsx
frontend/src/api/research.ts
```

同时更新：

```text
frontend/src/App.tsx
frontend/src/api/urls.ts
frontend/src/types/index.ts
```

## 5. 后端领域模型与数据库

### 5.1 `research_jobs`

保存研究任务及其可恢复状态，至少包含：

- `id`
- `tenant_id`
- `created_by_user_id`
- `mode`：`keyword` 或 `urls`
- `query_payload`：规范化后的关键词或链接列表
- `target_count`
- `target_collection_id`，可空
- `status`
- `found_count`
- `collected_count`
- `organized_count`
- `failed_count`
- `saved_count`
- `attempt_count`
- `max_attempts`
- `available_at`
- `lease_owner`
- `lease_until`
- `last_error_code`
- `last_error_summary`
- `cancel_requested_at`
- `started_at`
- `completed_at`
- `created_at`
- `updated_at`
- `version`

`query_payload` 不得包含 Cookie、Token 或浏览器登录态。

### 5.2 `research_sources`

保存租户内的来源记录，至少包含：

- `id`
- `tenant_id`
- `job_id`
- `platform`，当前固定为 `xiaohongshu`
- `platform_source_id`，可空
- `source_url`
- `normalized_url`
- `title`
- `author_display_name`
- `published_at`
- `raw_content`
- `public_tags`
- `like_count`、`collect_count`、`comment_count`，均可空
- `content_hash`
- `status`
- `failure_code`
- `failure_summary`
- `collected_at`
- `created_at`
- `updated_at`
- `version`

租户内按 `normalized_url` 建唯一约束。若平台 ID 可靠，还应增加租户内平台 ID 唯一约束。正文更新必须保留研究来源版本或审计记录。

### 5.3 `research_assets`

保存来源图片及 OCR 状态，至少包含：

- `id`
- `tenant_id`
- `source_id`
- `position`
- `storage_path`
- `original_url_hash`
- `mime_type`
- `byte_size`
- `sha256`
- `width`、`height`，可空
- `ocr_status`
- `ocr_text`
- `failure_code`
- `created_at`
- `updated_at`

`storage_path` 必须是 `DIARY_DATA_DIR` 下的安全相对路径。不得将第三方原始图片 URL 作为长期公开代理地址。

### 5.4 `research_drafts`

保存用户确认前的整理草稿，至少包含：

- `id`
- `tenant_id`
- `source_id`
- `summary`
- `key_points`
- `category`
- `suggested_tags`
- `edited_by_user`
- `status`：`pending`、`saved`、`ignored`
- `knowledge_document_id`，可空
- `model_name`
- `prompt_version`
- `source_snapshot_hash`
- `created_at`
- `updated_at`
- `version`

AI 草稿覆盖前必须创建 revision 或保留上一版本。保存到知识库必须校验草稿仍对应当前来源快照。

### 5.5 索引、迁移与 RLS

- 在 `backend/db/migrations` 增加版本化迁移，并更新 `backend/db/schema.sql` 初始化基线。
- 迁移通过现有 advisory lock 机制执行。
- 四张表均启用并强制 RLS。
- 增加任务 claim、状态筛选、来源去重和列表分页所需索引。
- 跨租户读取、修改、下载和删除统一返回 404。
- 软删除租户的普通请求返回 403。
- 不在应用启动时执行临时 DDL。

## 6. 采集器设计

### 6.1 运行边界

由于当前后端禁止重新引入 Python 后端，采集能力应通过以下方式之一实现：

- Go 后端内的独立采集 package；或
- 具有明确 HTTP 契约的受控采集服务，后端仍是唯一面向前端的业务入口。

若采用独立采集服务：

- 不得直接暴露宿主机端口。
- 只能由 backend 网络访问。
- 不得访问 PostgreSQL 业务账号或 LiteLLM 供应商密钥。
- 请求必须携带后端生成的短期任务凭证，且凭证不得进入日志。
- 返回内容必须有严格大小、类型、数量和超时限制。
- Compose 中需配置降权运行、只读根文件系统、资源限制和 healthcheck。

### 6.2 关键词搜索

- 只访问公开搜索结果。
- 输入必须限制关键词数量、单词长度和目标结果数量。
- 结果进入租户内去重流程。
- 搜索结果仅作为候选，不应在未成功读取详情时创建完整草稿。
- 页面结构变化应返回稳定错误 `XHS_LAYOUT_CHANGED`，不得把 HTML 或页面源码返回客户端。

### 6.3 链接采集

- 仅接受 `https`。
- 域名必须在显式白名单中。
- 阻止重定向到私网、环回、链路本地地址和非白名单域名，防止 SSRF。
- 规范化 URL 时移除无关追踪参数，但保留定位公开笔记所需参数。
- 限制单任务链接数。
- 不支持的、已删除的、需要额外权限的内容返回稳定错误。

### 6.4 登录态

- 不允许所有租户共享同一个浏览器 profile。
- 不允许将 Cookie、二维码内容或登录凭证写入普通日志、数据库正文、备份或 AI Prompt。
- 如果运行环境无法提供租户隔离的授权会话，关键词自动搜索必须明确标记为不可用，但公开链接导入和其他产品能力保持可用。
- 不实现验证码规避或自动化风控绕过。
- 会话过期应返回 `XHS_AUTH_REQUIRED`，由用户重新授权。

### 6.5 内容与图片限制

- 限制单来源正文字符数、图片数量、单图大小和单任务总下载量。
- 下载前后校验 Content-Type、文件签名和大小。
- 拒绝 SVG、HTML、脚本和未知可执行格式。
- 文件名由服务端生成，不使用第三方文件名作为路径。
- 防止目录穿越、压缩炸弹和超大尺寸图片。
- 采集器不得记录完整正文。

## 7. OCR

- OCR 是图片处理步骤，不影响已成功获取的正文。
- OCR 失败应按图片记录，不能导致整条来源永久丢失。
- OCR 服务不可用时，来源仍可进入人工审核，但状态需提示 OCR 部分失败。
- OCR 输入只允许当前租户已保存且通过校验的图片。
- OCR 输出限制最大字符数并进行 UTF-8 规范化。
- OCR 文本作为来源材料保存，不直接覆盖原始正文。
- OCR 服务不得持有 LiteLLM Key、数据库业务凭证或跨租户文件访问能力。

## 8. AI 整理与分类

### 8.1 输入

AI 输入可包含：

- 标题
- 公开正文
- OCR 文本
- 公开标签
- 用户选择的研究关键词

不得包含：

- 用户邮箱或姓名
- 登录 Cookie、Token 或浏览器 profile
- 内部存储路径
- 其他租户内容
- 无关评论用户信息

### 8.2 输出

模型必须输出经过结构化校验的数据：

- `summary`
- `key_points`
- `category`
- `suggested_tags`
- `warnings`，可选

后端必须限制字段长度、关键观点数量和标签数量。解析失败返回 `AI_OUTPUT_INVALID` 并允许重试。

### 8.3 网关与可靠性

- 统一使用 `AIClient`、LiteLLM 虚拟密钥和 `diary-default`。
- 观测元数据只能包含后端生成的非直接身份标识、请求类型、环境和追踪 ID。
- 默认不启用跨租户 Prompt 或响应缓存。
- AI 未配置或不可用时返回稳定错误，采集和人工编辑继续可用。
- 已开始输出内容后不得从头重试。
- 对同一来源快照和 Prompt 版本进行幂等保护，避免并发生成多个不一致草稿。

## 9. 知识库写入

- 保存动作必须由用户明确触发，支持单条和批量确认。
- 保存前校验来源和草稿属于当前租户、未被忽略、版本未冲突。
- 将标题、整理后的正文、来源链接、来源平台、公开作者显示名、发布时间、采集时间、分类和标签写入知识库文档。
- 原始正文和 OCR 内容是否进入知识文档正文应由固定模板决定，并在页面预览中明确展示。
- 来源图片通过受认证附件关系关联，不得公开为静态目录。
- 保存操作和草稿状态更新在同一事务内完成。
- 重复提交必须返回同一知识文档或明确的幂等结果，不得生成重复文档。
- 写入后使用现有知识库索引队列建立向量索引。
- 成长助手引用该内容时必须返回当前租户可访问的来源。

## 10. API 契约

所有成功响应和错误响应沿用现有稳定 `code`、`message`、可选 `details` 格式。错误不得暴露第三方响应正文、页面 HTML、内部地址、密钥或完整日记正文。

### 10.1 任务接口

```text
POST   /api/v1/research/jobs
GET    /api/v1/research/jobs
GET    /api/v1/research/jobs/:id
POST   /api/v1/research/jobs/:id/cancel
POST   /api/v1/research/jobs/:id/retry
```

创建请求示例：

```json
{
  "mode": "keyword",
  "keywords": ["Agent 面试", "RAG 实践"],
  "target_count": 20,
  "target_collection_id": 12,
  "idempotency_key": "client-generated-value"
}
```

链接模式使用 `urls` 替代 `keywords`。

### 10.2 结果接口

```text
GET    /api/v1/research/sources
GET    /api/v1/research/sources/:id
POST   /api/v1/research/sources/:id/retry
POST   /api/v1/research/sources/:id/recollect
PATCH  /api/v1/research/sources/:id/draft
POST   /api/v1/research/sources/:id/save
POST   /api/v1/research/sources/:id/ignore
POST   /api/v1/research/sources/batch-save
POST   /api/v1/research/sources/batch-ignore
```

列表接口支持：

- `job_id`
- `status`
- `search`
- `sort`
- `page`
- `page_size`

`page_size` 必须有服务端上限。

### 10.3 图片接口

```text
GET /api/v1/research/assets/:id
```

- 必须认证并通过 RLS 验证租户。
- 支持安全的 `Content-Type`、`Content-Length`、缓存控制和下载文件名。
- 不接受客户端提交的磁盘路径。

### 10.4 建议错误码

- `RESEARCH_INVALID_MODE`
- `RESEARCH_INVALID_KEYWORD`
- `RESEARCH_INVALID_URL`
- `RESEARCH_LIMIT_EXCEEDED`
- `RESEARCH_JOB_NOT_FOUND`
- `RESEARCH_JOB_NOT_RETRYABLE`
- `RESEARCH_SOURCE_NOT_FOUND`
- `RESEARCH_VERSION_CONFLICT`
- `RESEARCH_ALREADY_SAVED`
- `XHS_AUTH_REQUIRED`
- `XHS_RATE_LIMITED`
- `XHS_SOURCE_UNAVAILABLE`
- `XHS_LAYOUT_CHANGED`
- `XHS_COLLECTOR_UNAVAILABLE`
- `RESEARCH_ASSET_INVALID`
- `OCR_UNAVAILABLE`
- `AI_UNAVAILABLE`
- `AI_OUTPUT_INVALID`

## 11. Worker、调度与幂等

- Worker 使用管理连接池认领到期任务。
- 使用 `FOR UPDATE SKIP LOCKED` 和有限租约。
- 多实例争抢同一任务只能成功认领一次。
- Worker 定期续租；租约过期后任务可由其他实例恢复。
- 重试采用有限次数和退避策略。
- 平台限流时尊重服务端退避时间，不进行高频重试。
- 取消请求在安全检查点生效。
- 任务状态和计数持久化到 PostgreSQL。
- 单条来源失败不应终止整个任务。
- 创建任务、保存知识文档和重新采集均需要幂等保护。
- 进程重启后运行中任务不得永久卡死。

## 12. 缓存策略

本功能不引入 Redis。

- 权威任务状态、进度、草稿和去重信息全部保存于 PostgreSQL。
- 短生命周期、可重建的数据可使用有容量上限的进程内缓存。
- 不缓存完整正文、完整 Prompt、登录态或跨租户 AI 响应。
- 前端轮询使用 ETag、更新时间或版本号减少无效传输。
- 若未来监控证明多实例分布式限流或热点读取成为瓶颈，应单独提交架构变更，不得让 Redis 成为正文或任务唯一存储。

## 13. 安全、隐私与合规

- 客户端提交的 `tenant_id` 一律忽略。
- 所有资源 ID 必须在可信 Principal 和 RLS 下解析。
- 登录凭证、供应商 Key、Cookie、二维码内容和完整正文不得进入普通日志。
- 审计只记录操作类型、资源 ID、状态、非直接身份标识和追踪 ID。
- 第三方内容必须保留来源链接、采集时间和来源平台。
- UI 必须提示用户遵守平台规则、版权和适用法律，只处理其有权研究或保存的内容。
- 提供来源删除能力；删除默认软删除，并同步停止其在知识检索中的可见性。
- 完整备份可包含当前租户确认保存的内容和附件，但不得包含 API Key、Token、浏览器登录态或敏感审计信息。
- 空租户恢复时必须重映射研究来源、草稿、附件和知识文档 ID。
- 内容采集失败、平台限制或授权过期不得降低其他业务接口可用性。

## 14. 可观测性与运维

### 14.1 指标

至少提供：

- 研究任务创建、完成、失败和取消数量
- 各状态任务数量
- 任务端到端耗时
- 来源采集成功率和耗时
- OCR 成功率和耗时
- AI 整理成功率和耗时
- 知识库保存成功率
- 平台限流和授权过期次数
- Worker 租约过期恢复次数

指标标签不得包含邮箱、姓名、关键词原文、来源正文或 URL。

### 14.2 日志

- 使用结构化日志。
- 包含请求追踪 ID、任务 ID、来源 ID、状态和稳定错误码。
- URL 应记录散列或经过脱敏的主机及资源标识。
- 不记录第三方响应正文、页面源码、OCR 全文或 AI 完整输入输出。

### 14.3 健康检查

- `/healthz` 只反映主进程存活，不依赖小红书、OCR 或 AI。
- `/readyz` 继续验证数据库可用。
- 可选采集器和 OCR 的不可用状态应在功能配置接口或管理指标中体现，不应使主后端不健康。

## 15. 配置

配置名称应使用 `RESEARCH_` 或 `XHS_` 前缀，至少包括：

- 功能开关
- 关键词数量上限
- 单任务链接数量上限
- 单关键词结果数上限
- 单来源图片数量上限
- 单图大小上限
- 单任务下载总量上限
- 正文和 OCR 字符上限
- Worker 数量
- 租约时长
- 最大重试次数
- 采集超时
- 平台请求最小间隔

默认值必须偏保守。敏感配置通过环境变量或 secret 注入，不得提交到仓库。

## 16. 测试要求

### 16.1 后端单元测试

- URL 规范化、白名单、重定向和 SSRF 防护。
- 关键词、链接数量和长度校验。
- 状态机合法与非法转换。
- 租约认领、续租、过期恢复和并发 claim。
- 任务取消和手动重试。
- 来源去重和幂等创建。
- 草稿结构化输出校验。
- 草稿版本冲突。
- 保存知识库的事务与幂等。
- 图片路径、MIME、大小、签名和目录穿越防护。
- 稳定错误码与敏感错误脱敏。

### 16.2 后端集成测试

- 注册、登录后创建和查询研究任务。
- 租户 A 无法读取、修改或下载租户 B 的研究资源。
- 客户端伪造 `tenant_id` 无效。
- 多 Worker 竞争只产生一次有效认领。
- 部分来源失败时任务仍能完成并保留成功结果。
- AI 不可用时采集和人工编辑可用。
- OCR 不可用时文本来源仍可审核。
- 保存后生成知识文档并进入索引状态。
- 删除研究来源后不再被当前租户检索。
- 软删除租户访问返回 403。
- 数据库重启或 Worker 重启后任务可恢复。

第三方平台测试必须使用录制后脱敏的固定 fixture 或 mock server；常规测试不得依赖真实小红书账号和网络页面。

### 16.3 前端测试

- `/research` 受认证保护且导航选中正确。
- 新建关键词任务和链接任务。
- 表单边界和错误提示。
- 任务状态及进度展示。
- 自动刷新和页面不可见时的轮询行为。
- 筛选、搜索、排序和分页。
- 详情抽屉正确区分来源与 AI 草稿。
- 编辑草稿并保存到知识库。
- 批量保存、批量忽略、取消和重试。
- 401、403、404、409、AI 不可用、OCR 部分失败和采集器不可用状态。
- 浅色、深色和窄屏关键交互。

### 16.4 部署与安全测试

- `docker compose config --quiet` 通过。
- `db` 和 `llm-gateway` 不暴露宿主机端口。
- 可选采集器和 OCR 不暴露宿主机端口。
- backend 和新增服务以非 root 用户运行。
- 数据卷在容器重建后保留。
- 日志扫描确认无 Key、Cookie、完整正文和第三方响应正文。
- 尝试私网 URL、重定向私网 URL、超大图片、伪造 MIME 和路径穿越均被拒绝。

## 17. 验收流程

### 17.1 验收前准备

1. 确认工作树中的既有用户改动已被识别且未被覆盖。
2. 配置 PostgreSQL、LiteLLM 虚拟 Key、研究功能开关和必要的采集授权。
3. 准备两个独立测试账号：租户 A、租户 B。
4. 准备以下测试资料：
   - 可访问的公开笔记链接。
   - 包含正文和图片文字的公开笔记。
   - 重复链接和带追踪参数的同一链接。
   - 已删除或不可访问链接。
   - 非白名单链接、私网链接和重定向到私网的链接。
   - AI、OCR、采集器分别不可用的故障注入配置。
5. 在全新 PostgreSQL 16 空库执行初始化和迁移。

### 17.2 自动化验收

在仓库根目录依次执行：

```powershell
Set-Location backend
gofmt -w <本次修改的 Go 文件>
go vet ./...
go test ./...
go build ./cmd/server

Set-Location ..\frontend
npm run format:check
npm test
npm run build

Set-Location ..
docker compose config --quiet
.\backend\scripts\non_ai_smoke.ps1
.\backend\scripts\ai_acceptance.ps1
```

若新增研究专项验收脚本，还必须执行：

```powershell
.\backend\scripts\research_acceptance.ps1
```

所有命令必须成功。测试不得依赖开发者本机已有浏览器 profile、缓存、输出目录或未提交密钥。

### 17.3 功能验收

1. 使用租户 A 登录，确认侧边栏存在“小红书研究”，点击进入 `/research`。
2. 刷新 `/research`，确认仍停留在该页面；未登录访问时跳转登录。
3. 创建关键词研究任务，确认立即返回 queued 并展示进度。
4. 创建多链接研究任务，确认合法链接被处理，非法链接得到逐条稳定错误。
5. 确认任务依次进入采集、解析、整理和待审核状态。
6. 确认来源列表支持筛选、搜索、排序和分页。
7. 打开详情，确认原始正文、OCR、AI 摘要、关键观点、分类、标签、图片和来源链接显示正确且明确分区。
8. 编辑草稿并刷新页面，确认修改持久化。
9. 保存单条结果到指定知识集合，确认只产生一个知识文档。
10. 对同一结果重复点击保存，确认不会产生重复文档。
11. 批量保存和批量忽略各执行一次，确认结果状态和任务计数正确。
12. 对重复 URL 和带不同追踪参数的相同 URL 创建任务，确认租户内去重。
13. 对失败来源执行重试，确认立即返回 queued 且最终状态可恢复。
14. 取消运行中任务，确认在安全检查点停止，已成功结果仍可审核。
15. 在知识库中确认保存的文档进入索引完成状态。
16. 在成长助手中提问，确认回答可以引用已保存来源，且引用属于当前租户。
17. 删除或软删除来源后，确认它不再参与知识检索。

### 17.4 租户与安全验收

1. 使用租户 B 枚举租户 A 的任务、来源、草稿和图片 ID，全部应表现为 404。
2. 在请求中伪造租户 A 的 `tenant_id`，确认服务端仍使用租户 B。
3. 测试附件目录穿越、伪造路径、超大文件和非图片内容，确认请求被拒绝。
4. 提交环回、私网、链路本地、非白名单域名和恶意重定向 URL，确认 SSRF 防护有效。
5. 并发启动多个 Worker，确认同一任务只产生一次有效 claim。
6. 在任务运行中终止 Worker，等待租约过期后确认另一 Worker 能恢复任务。
7. 检查应用、采集器、OCR、LiteLLM 和数据库日志，确认不存在：
   - 供应商真实 Key
   - LiteLLM 虚拟 Key
   - 登录 Token 或 Cookie
   - 用户邮箱或姓名
   - 完整来源正文或 OCR 全文
   - 第三方响应正文或页面 HTML
8. 软删除租户 A，确认普通研究请求返回 403。

### 17.5 降级验收

1. 关闭 AI，确认任务仍可采集、人工编辑和保存，其他产品功能正常。
2. 关闭 OCR，确认正文来源仍可审核，图片标记 OCR 失败。
3. 关闭采集器，确认研究页面显示明确不可用提示，笔记、知识库、附件、导出和备份仍可用。
4. 触发第三方限流，确认任务退避，不进行无限重试。
5. 模拟平台 DOM 变化，确认返回 `XHS_LAYOUT_CHANGED`，客户端不显示页面源码。
6. 重启 backend、Worker 和数据库，确认任务状态不丢失，租约任务能够恢复。
7. 确认 `/healthz` 不依赖 AI、OCR 或小红书；`/readyz` 正确反映数据库状态。

### 17.6 UI 验收

1. 对比工作台、知识库和成长助手，确认标题、卡片、表格、按钮、间距、圆角和字体风格一致。
2. 切换浅色、深色和系统主题，确认没有不可读文字、硬编码亮色背景或错误边框。
3. 在桌面宽度和小于 800px 宽度下完成新建任务、查看详情、编辑草稿和保存。
4. 确认加载、空数据、部分失败、全失败、冲突和无权限状态均有清晰反馈。
5. 确认键盘可访问主要表单和操作，输入框有标签，图片有替代文本，焦点在抽屉关闭后正确返回。

### 17.7 数据库与部署验收

1. 全新空库初始化成功，研究表、索引、外键和 RLS 策略完整。
2. 从上一版本数据库执行迁移成功，重复执行迁移不会破坏数据。
3. 使用低权限 `diary_app` 运行 backend，确认不能绕过 RLS 或执行迁移权限操作。
4. 使用 `diary_migrator` 完成迁移和 scheduler claim 所需操作。
5. `docker compose` 中 `db`、`llm-gateway`、`backend` 及新增可选服务均达到预期健康状态。
6. 容器重建后数据库、研究图片和已保存知识文档仍存在。
7. 完整备份不包含 Key、Token、Cookie 或敏感审计信息。
8. 将备份恢复到空租户，确认研究来源、图片、草稿和知识文档 ID 正确重映射。

## 18. 完成定义

只有同时满足以下条件才视为功能完成：

- 本文所有目标功能已实现，不以占位页面或静态演示替代。
- `/research` 页面与当前 Cortex UI 风格一致。
- 关键词和链接两种研究方式均可工作。
- 采集、OCR、AI 整理、人工确认和知识库写入链路可完整运行。
- PostgreSQL 是唯一权威数据源，未引入 Redis 依赖。
- 租户隔离、RLS、附件安全、SSRF 防护、AI 网关和日志脱敏均通过验收。
- AI、OCR 或采集器不可用时，系统按本文要求降级。
- 后端、前端、部署和专项验收测试全部通过。
- README、API 文档、数据库基线、迁移说明和运行配置已同步更新。
- 没有修改或覆盖与本功能无关的用户工作区改动。
