# Diary Listener 软件设计说明书

> 状态：当前实现基线
> 更新日期：2026-07-26

## 1. 产品目标

Diary Listener 是本地优先、AI 辅助的个人笔记与回忆工作台。系统保存可检索的 Markdown 笔记，并仅依据用户自己的笔记生成整理草稿、周期报告和带来源引用的回忆回答。

核心不变量：

1. PostgreSQL 是正文唯一权威来源。
2. AI 不可用时非 AI 功能完整可用。
3. AI 整理和报告必须先生成、后确认写入。
4. 报告与回忆回答必须保留可追溯来源。
5. 每个用户只有一个服务端解析的个人租户。
6. 客户端不能控制可信租户身份。
7. 附件不作为公开静态目录暴露。

## 2. 系统架构

```text
Browser
  |
  v
React / TypeScript / Nginx
  |
  | /api + SSE
  v
Go HTTP Server (Gin)
  |-- Auth / Principal / RLS
  |-- Notes / Tags / Search / Dashboard
  |-- Attachments / Export / Backup
  |-- AI Workflows / Legacy Compatibility
  `-- Scheduled Report Worker
       |                  |
       v                  v
 PostgreSQL 16       LiteLLM Proxy
                           |
                    Kimi / OpenAI
```

后端是无状态 HTTP 服务；持久数据位于 PostgreSQL 和 `DIARY_DATA_DIR`。多个 Go 实例可共享数据库和对象存储，但 scheduler 必须使用数据库 claim 防止重复执行。

## 3. 后端设计

### 3.1 路由与中间件

- Gin 是唯一 Web 框架。
- `/api/v1` 承载当前产品接口。
- 旧认证、聊天和轻日记路径保持兼容。
- 中间件负责 CORS、panic recovery、请求追踪、Token 认证和 Principal 注入。
- 合法的客户端 `X-Request-ID` 可继续使用，否则服务端生成 UUID；响应始终回传该标识。
- handler 只处理 HTTP 契约，SQL 位于 `internal/store`。

### 3.2 错误

业务错误使用稳定错误码：

```json
{
    "code": "ERROR_CODE",
    "message": "用户可读消息",
    "details": null
}
```

鉴权失败、验证失败、冲突、资源不存在、AI 未配置和上游失败必须映射为明确 HTTP 状态。日志可以记录内部错误，但不得记录密钥和完整日记正文。

### 3.3 认证

- 密码使用 PBKDF2-SHA256。
- 登录 Token 只以 SHA-256 摘要持久化。
- Token 具有过期、撤销和最后使用时间。
- 认证后由服务端查找用户的唯一租户。
- 软删除租户的普通业务请求返回 403，恢复接口除外。

## 4. 数据设计

核心表：

- `users`、`auth_tokens`
- `tenants`
- `notes`、`note_revisions`
- `tags`、`note_tags`
- `attachments`
- `audit_logs`
- `ai_providers`、`ai_usage_records`
- `conversations`、`messages`、`message_sources`
- `report_sources`
- `diary_entries`
- `scheduled_report_tasks`、`scheduled_report_runs`

### 4.1 租户隔离

- 租户资源包含 `tenant_id`。
- PostgreSQL 对租户表启用 RLS。
- 业务请求开启 `pgx.Tx` 后，在同一事务设置 transaction-local 用户与租户上下文。
- 查询同时保留显式 `tenant_id` 条件，形成应用与数据库双重边界。
- 跨租户读取统一表现为 404。

### 4.2 笔记

- 类型为 `normal`、`daily`、`weekly`、`monthly`。
- 周报日期归一到周一，月报日期归一到月初。
- 周期笔记在租户、类型和周期日期上唯一。
- 正文更新创建 revision。
- 删除为软删除。
- AI 覆盖前保存 revision。

### 4.3 附件

- 文件位于 `DIARY_DATA_DIR/attachments/<tenant>/<year>/<month>/`。
- 数据库只保存受控相对路径、文件名、MIME、大小和摘要。
- 上传验证大小和租户配额。
- 下载与删除必须经过认证和租户校验。
- 路径解析必须阻止目录穿越。

## 5. AI 工作流

### 5.1 接口边界

- `AIClient` 只负责模型流。
- `Retriever` 只在可信 Principal/RLS 下检索来源。
- `AIWorkflow` 编排整理、报告和回忆。
- `StreamEvent` 统一增量内容和错误。

### 5.2 整理

输入自由文本和样式，流式生成草稿。只有确认接口可以新建或更新笔记；更新必须生成 revision。

### 5.3 报告

服务端按报告类型和日期确定周期并检索来源。没有来源时返回 `REPORT_NO_SOURCES`，不得要求模型虚构。确认报告时校验所有 `source_ids` 属于当前租户和周期，并写入 `report_sources`。

### 5.4 回忆

服务端提取问题关键词并检索候选笔记。没有证据时返回 `MEMORY_NO_EVIDENCE`。回答完成后保存会话、消息和 `message_sources`。

### 5.5 网关

LiteLLM 提供 `diary-default` 逻辑模型。Kimi `kimi-k2.5` 是主路由，OpenAI `gpt-5.6` 是备用。业务后端不持有供应商 Key。

- LiteLLM 使用独立的 `diary_litellm` 管理数据库记录准确 token、成本和路由指标。
- 业务后端只使用限制模型和预算的虚拟密钥，master key 仅用于网关管理。
- 应用向网关传递请求 ID、请求类型、环境和内部租户标识，不传递邮箱、姓名或其他直接身份信息。
- 缓存默认关闭，不允许跨租户共享 Prompt 或响应缓存。

## 6. Scheduler

- 支持 daily、weekly、monthly。
- 时间以任务 IANA 时区计算，数据库保存 UTC。
- worker 使用管理连接池执行 due-task claim。
- claim 使用 `FOR UPDATE SKIP LOCKED` 和有限租约。
- 执行状态持久化为 running/success/failed。
- 手动重试异步执行并返回 queued。
- 两个 worker 同时争抢同一任务只能产生一条 run。

## 7. 数据交换

- Markdown 导出生成 ZIP。
- 完整备份包含数据清单和附件完整性信息。
- 恢复只允许目标租户为空，避免合并歧义。
- 备份恢复必须重新映射笔记、标签和附件 ID。
- API Key、Token 和审计敏感信息不得进入备份。

## 8. 配置

Compose 部署必需：

- `DATABASE_URL`
- `MIGRATION_DATABASE_URL`
- `POSTGRES_APP_PASSWORD`
- `POSTGRES_MIGRATOR_PASSWORD`
- `LITELLM_DB_PASSWORD`
- `LITELLM_MASTER_KEY`
- `LITELLM_VIRTUAL_KEY`
- `KIMI_API_KEY`
- `OPENAI_API_KEY`

常用可选：

- `LISTEN_ADDRESS`
- `CORS_ORIGINS`
- `DIARY_DATA_DIR`
- `MAX_ATTACHMENT_BYTES`
- `TOKEN_TTL_HOURS`
- `DB_POOL_SIZE`
- `DB_STATEMENT_TIMEOUT_MS`
- `SCHEDULED_REPORTS_ENABLED`
- `SCHEDULED_REPORT_POLL_SECONDS`
- `APP_ENV`

## 9. 部署与健康

- Compose 编排 PostgreSQL、LiteLLM、Go 后端和前端。
- PostgreSQL 初始化 `diary_migrator`、`diary_app` 和独立的 `diary_litellm` 角色/数据库。
- 后端镜像使用 Go builder 和 Alpine runtime。
- runtime 安装 CA 与时区数据。
- 数据目录初始化后降权为 `diary` 用户。
- `/healthz` 不依赖外部 AI。
- `/readyz` 验证数据库。
- LiteLLM 只在 Compose 网络暴露 4000。
- 可选 Helm Chart 位于 `packaging/helm/diary-listener`，要求外部 PostgreSQL、LiteLLM、Secret 和支持 `ReadWriteMany` 的附件存储。

### 9.1 数据库升级

- `backend/db/schema.sql` 仅用于新数据库初始化。
- 已部署数据库使用 `backend/cmd/migrate` 执行 `up`、`down` 和 `status`。
- 每个版本必须同时提供 `.up.sql` 与 `.down.sql`，并记录 SHA-256。
- 迁移进程在单一数据库会话持有 PostgreSQL advisory lock，每个版本在独立事务内执行。
- 服务进程不得在启动时执行临时 DDL；Helm 在安装和升级前通过 Hook Job 运行迁移。

## 10. 前端设置

- `/settings` 只管理当前浏览器的主题，不展示 AI 地址、模型、Key 或连接测试。
- 主题值为 `system`、`light` 或 `dark`，保存于 `localStorage` 的 `diary-listener.theme`。
- 默认跟随 `prefers-color-scheme` 并监听系统变化；非法值回退到 `system`。
- 主题覆盖页面、Ant Design、Markdown 编辑器、弹窗及浏览器 `theme-color`，切换不刷新页面。
- 主题偏好不上传服务端、不进入备份，退出登录后继续保留。
- 控件须支持键盘、可见焦点和屏幕阅读器标签，正文颜色至少满足 WCAG AA。

## 11. 测试与发布策略

- 单元测试：密码、SSE、日期、关键词、路径和归档校验。
- 数据库冒烟：双租户隔离、附件、导出、备份恢复。
- AI 验收：通用 SSE、整理确认、报告引用、回忆引用。
- scheduler：无来源失败、成功生成和双 worker claim。
- 空库：18 张表、15 条 RLS Policy、ready、注册和登录。
- 每次发布运行 `gofmt` 检查、`go vet`、`go test`、Go build 和 Compose config。
- Helm 变更运行 `helm lint` 和 `helm template`。
- 生产发布前执行数据库备份、迁移演练、镜像安全扫描和至少 24 小时稳定性观察。

## 12. 待决事项

- OpenAI 备用路由需在账户额度恢复后重新验证。
- 应用内 `ai_usage_records` 仍使用字符估算 token，准确计费数据以 LiteLLM 为准。
- 数据库中的租户 AI Provider 元数据当前不控制实际路由；后续应决定删除该配置面或明确其产品用途。
- 语义检索仅在真实回忆问答 Top-8 召回率低于 85%、且同义表达是主要失败原因时重新评估，优先考虑 PostgreSQL `pgvector`。
- 自动生产发布待托管平台、Secret 管理和回滚策略确定后实施。
