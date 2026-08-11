# Cortex 软件设计说明书

## 1. 产品边界

Cortex 是个人记录与回顾工作台，提供笔记、日报/周报/月报、标签、附件、历史版本、
中文搜索、Dashboard、AI 整理、报告、回忆、研究、模板广场、限量 AI 活动和
个人知识库。

知识库以个人知识库 v2 为当前主线：

- 用户上传单个 `.md` 或 Markdown `.zip`，可创建知识集合、开启个人笔记入库，并进行
  带来源保存的混合问答；每租户容量上限 3 GiB。
- 数据由迁移 `000017_personal_knowledge_v2` 的 `knowledge_*` 表承载，并启用 RLS。
- `/knowledge` 是知识库入口；`/recipes` 与 `/assistant` 已重定向到 `/knowledge`。
- HowToCook 固定语料已从仓库移除并一次性迁移到用户 `Diving` 的运行时私有知识库；
  菜谱接口（`/api/v1/recipes/*`）与忌口、时区偏好已一并移除，前端 `/recipes`、
  `/assistant` 重定向到 `/knowledge`。
- 研究结果、日报、周报、月报和个人笔记不会写入个人知识库；研究内容与知识库检索相互隔离。

## 2. 技术架构

- 前端：React 18、TypeScript、Webpack 5、Ant Design。
- 后端：Go、Gin、pgx/v5，唯一入口为 `backend/cmd/server/main.go`。
- 数据库：PostgreSQL 16、RLS、pgvector。
- AI：后端仅通过 LiteLLM 的 OpenAI 兼容接口访问逻辑模型。

PostgreSQL 是个人笔记正文的唯一权威来源。Markdown 只用于笔记交换、导出以及个人知识库
上传语料，不与数据库做双向同步。

## 3. 租户与数据安全

- 每个账号对应一个服务端解析的个人租户，客户端 `tenant_id` 不可信。
- 租户查询在 `Store.WithTx` 中设置 transaction-local RLS 上下文，并保留显式
  `tenant_id` 条件。
- 跨租户资源访问统一表现为 404。
- 密码使用 PBKDF2-SHA256；登录 Token 只保存 SHA-256 摘要。
- 笔记更新使用乐观锁，正文更新和 AI 覆盖前创建 revision，删除默认软删除。
- 附件和知识文件只保存 `CORTEX_DATA_DIR` 下的安全相对路径，不作为公开静态目录暴露；`DIARY_DATA_DIR` 仅为兼容别名。

## 4. 个人知识库

```mermaid
flowchart LR
    UPLOAD["上传 .md / .zip"] --> PREPARE["安全校验与落盘"]
    PREPARE --> DB[("knowledge_* 表<br/>RLS 隔离")]
    NOTES["个人笔记知识开关"] --> DB
    DB --> INDEX["Knowledge Indexer"]
    EMBED["固定 Embedding 服务"] --> INDEX
    INDEX --> DB
    API["/api/v1/knowledge/*"] --> DB
    UI["/knowledge"] --> API
```

- 上传经 `internal/knowledge` 校验类型、配额与 ZIP 路径安全后，保存到
  `CORTEX_DATA_DIR/knowledge/{tenant_id}/...` 下的安全相对路径。
- 后台 `RunKnowledgeIndexer` 对文档做父子切块，并使用 Compose 内部
  `embedding-service`（`iic/nlp_gte_sentence-embedding_chinese-small`，512 维）生成向量。
- 问答只检索当前租户 `knowledge_documents`（含开启知识问答的个人笔记），向量 + 全文混合召回，
  经 `reranker-service`（`BAAI/bge-reranker-v2-m3`）精排后取前 `RAG_CONTEXT_PARENT_TOP_K` 个
  parent；来源写入 `knowledge_message_sources`。无当前租户证据时返回 `KNOWLEDGE_NO_EVIDENCE`。

主要接口：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/api/v1/knowledge/uploads` | 上传 `.md` / `.zip`，安全落盘后返回 202 |
| `GET` | `/api/v1/knowledge/documents` | 列出当前租户文档与 3 GiB 配额 |
| `DELETE` | `/api/v1/knowledge/documents/{id}` | 删除文档并使其退出检索 |
| `GET` / `POST` | `/api/v1/knowledge/collections` | 查询或创建知识集合 |
| `POST` | `/api/v1/knowledge/chat/stream` | 混合检索、精排并 SSE 回答 |
| `PATCH` | `/api/v1/notes/{id}/knowledge` | 开启或关闭笔记知识索引 |

## 5. 笔记、报告和回忆

笔记、日报、周报和月报保存在租户业务表中。报告必须先生成草稿，确认后写入，并保存当前
租户的来源笔记。回忆问答只能使用可信 Principal 下检索到的个人笔记。用户可通过
`PATCH /api/v1/notes/{id}/knowledge` 开启个人笔记参与个人知识库问答；知识库检索与回忆
问答相互隔离。

## 6. 研究

研究任务和来源受个人租户 RLS 隔离，可生成可编辑草稿、忽略或删除。研究内容不会保存到
个人知识库，也不提供目标知识集合参数。图片资产保存在独立的 `research` 安全目录。

## 7. AI 与降级

AI 未配置或不可用时，认证、笔记、搜索、附件、导出和知识库文件管理仍可用。
个人知识库上传、删除不依赖 Embedding；Embedding 或 Reranker 不可用时，索引任务失败或
问答返回稳定错误（`KNOWLEDGE_EMBEDDING_UNAVAILABLE` / `KNOWLEDGE_RERANK_UNAVAILABLE`）。
后端只持有 LiteLLM 虚拟密钥；供应商真实 Key 不进入前端、业务数据库或日志。
流式响应已经输出内容后不得从头重试。

## 8. 部署与验证

Compose 下数据库、LiteLLM、Embedding 和 Reranker 服务不暴露宿主机端口。
`/healthz` 只反映进程存活，`/readyz` 只验证数据库可用。新实例由 `backend/db/schema.sql`
基线加版本化迁移初始化（当前共 51 张表）。

```powershell
Set-Location backend
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
.\backend\scripts\research_acceptance.ps1
.\backend\scripts\template_ai_event_acceptance.ps1
```

知识库验收覆盖上传、索引、混合问答、跨租户隔离与 3 GiB 配额。

## 9. 模板广场与限量 AI 活动

私有模板受租户 RLS 保护，作者明确上架时生成不含租户标识的公开快照；作者下架或删除租户时
立即使快照不可见。

每日活动配置保存在 PostgreSQL，Redis Lua 负责库存和重复领取预扣，数据库唯一约束、点数
账本与 claim/job 状态机保存最终事实。Worker 使用有限租约领取任务，成功后自动写入带来源的
月报；最终失败释放冻结点数但不返普通名额。Redis 不可用时只关闭领取，不影响核心笔记功能。

活动参数集中保存在 `ai_flash_event_settings`。scheduler 使用 PostgreSQL 剩余名额和既有领取记录
预热带时间窗的 Redis Key；领取请求先由 Redis `TIME` 裁决开放时间和库存，只有预扣成功的少量
请求进入 PostgreSQL。模板排行由 outbox 幂等投影到 ZSet/HLL，Redis 清空后从公开统计重建。
