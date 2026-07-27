# Cortex 软件设计说明书

> 状态：当前实现基线
>
> 更新日期：2026-07-27
>
> 未完成、部分完成和待验收事项统一见 [实现与生产验收待办](IMPLEMENTATION_GAPS.md)。

## 1. 产品目标与范围

Cortex 是一个记录、连接和回顾个人成长的 AI 知识工作台。系统保存个人笔记、日报、
周报、月报、标签、附件和知识文件，并基于当前租户的笔记或知识来源提供整理、报告、
回忆和带引用的知识问答。

当前实现包含：

- Markdown 笔记、周期笔记、标签、附件、历史版本、中文搜索和 Dashboard。
- AI 整理草稿、周期报告、回忆问答、来源引用和报告调度。
- UTF-8 TXT、文本型 PDF、标准 DOCX 知识文件管理。
- 异步提取、父子切块、PostgreSQL FTS、pgvector 向量召回和可选 reranker。
- 带筛选、预览和重建索引的知识库页面，以及支持会话、统一来源引用和具名 SSE 的成长助手。
- Markdown ZIP 内容导出。

当前不提供扫描 PDF OCR、XLSX/PPTX/音视频解析、团队共享知识库、云盘同步、用户画像、
模型训练、数据库与 Markdown 双向同步或应用级完整备份包。

核心不变量：

1. PostgreSQL 是业务正文和知识元数据的唯一权威来源。
2. Markdown 只用于内容交换和导出。
3. AI 不可用时，账号、笔记、搜索、附件、导出和知识文件管理仍可用。
4. AI 整理和报告先生成草稿，确认后才能写入笔记。
5. 报告、回忆和知识问答保存可追溯来源；无证据时拒绝生成。
6. 每个账号只有一个由服务端解析的个人租户，客户端不能控制可信租户身份。
7. 附件和知识原文件不作为公开静态目录暴露。

## 2. 系统架构

```text
Browser
  |
  | HTTP / Token / SSE
  v
React 18 / TypeScript / Ant Design
  |
  v
Go / Gin Backend
  |-- Auth / Principal / RLS
  |-- Notes / Tags / Search / Dashboard
  |-- Attachments / Markdown Export
  |-- Knowledge Document API / Safe File Storage
  |-- Extraction / Parent-Child Chunking / Index Worker
  |-- FTS + Vector + RRF + Optional Rerank
  |-- AI Workflows / Knowledge Chat
  `-- Scheduled Report Worker
       |                         |
       v                         v
PostgreSQL 16 + pgvector      LiteLLM
  |-- business metadata         |-- diary-default
  |-- parent/child chunks       `-- cortex-embedding
  |-- jobs and citations                 |
  `-- RLS                              Host Ollama
                               qwen3-embedding:0.6b

Optional internal service
  `-- Qwen/Qwen3-Reranker-0.6B

DIARY_DATA_DIR
  |-- attachments/<tenant>/...
  `-- knowledge/<tenant>/...
```

`backend/cmd/server/main.go` 是唯一后端服务入口。后端 HTTP 进程无状态；持久数据位于
PostgreSQL、Docker 数据卷和 `DIARY_DATA_DIR`。多个后端实例通过数据库租约争抢
scheduler 和知识索引任务。

## 3. 后端边界

### 3.1 HTTP、中间件与错误

- Gin 是唯一 Web 框架，当前业务接口位于 `/api/v1`。
- 旧认证、聊天和轻日记路径为兼容保留。
- 中间件负责 CORS、panic recovery、请求追踪、Token 认证和 Principal 注入。
- 合法的 `X-Request-ID` 可继续使用，否则服务端生成 UUID，并在响应中回传。
- handler 负责 HTTP/SSE 契约；持久化 SQL 和事务位于 `backend/internal/store`。
- 业务错误返回稳定的 `code`、`message` 和可选 `details`。
- 响应和普通日志不得包含密钥、内部地址、上游响应正文、完整笔记或知识正文。

### 3.2 认证与租户

- 密码使用 PBKDF2-SHA256。
- 登录 Token 只持久化 SHA-256 摘要，并记录过期、撤销和最后使用时间。
- 服务端根据 Token 查找用户唯一租户，不接受客户端 `tenant_id`。
- 软删除租户的普通业务请求返回 403，恢复接口除外。
- 跨租户资源访问统一表现为 404。

### 3.3 数据访问

- 租户业务查询通过 `Store.WithTx` 执行。
- 同一个 `pgx.Tx` 中设置 transaction-local 用户和租户 RLS 上下文。
- SQL 同时保留显式 `tenant_id` 条件。
- 管理连接只用于迁移、scheduler claim 和知识索引任务 claim，不读取租户正文。

## 4. 数据设计

### 4.1 核心数据

现有核心表覆盖：

- 用户与租户：`users`、`auth_tokens`、`tenants`。
- 笔记：`notes`、`note_revisions`、`tags`、`note_tags`、`attachments`。
- AI 与来源：`ai_providers`、`ai_usage_records`、`conversations`、`messages`、
  `message_sources`、`report_sources`。
- 兼容和调度：`diary_entries`、`scheduled_report_tasks`、`scheduled_report_runs`。
- 审计：`audit_logs`。

### 4.2 笔记、附件与导出

- 笔记类型为 `normal`、`daily`、`weekly`、`monthly`。
- 周报日期归一到周一，月报日期归一到月初；周期笔记按租户、类型和周期日期唯一。
- 正文更新和 AI 覆盖前创建 revision；删除默认软删除。
- 附件位于 `DIARY_DATA_DIR/attachments/<tenant>/<year>/<month>/`，数据库仅保存安全相对路径。
- 附件上传校验大小和租户配额；下载和删除需要认证并阻止目录穿越。
- Markdown 导出生成 ZIP，但不视为完整备份。
- 应用不提供 ZIP 完整备份/恢复，基础设施负责数据库和文件卷灾备。

### 4.3 知识库数据

版本化迁移 `000002_knowledge_base` 启用 `vector` 扩展并新增：

- `knowledge_collections`：租户内知识集合，支持描述、版本和软删除。
- `knowledge_documents`：原文件元数据、摘要、状态、解析统计和 active index version。
- `knowledge_parent_chunks`：用于生成上下文的结构完整父块。
- `knowledge_child_chunks`：用于 FTS/向量召回的细粒度子块，向量维度为 1024。
- `knowledge_index_jobs`：带租约、尝试次数和目标代次的异步索引任务。
- `knowledge_message_sources`：知识回答对应的文档、父块、子块、页码和最小引用片段。

新增租户表启用并强制 RLS，Policy 同时包含 `USING` 与 `WITH CHECK`。父块、子块、
文档和引用通过包含 `tenant_id` 的复合约束保持归属一致。子块包含 FTS GIN 索引；
当前小规模阶段使用带显式租户过滤的精确向量扫描，尚未创建 HNSW。

知识文档状态包括 `uploaded`、`extracting`、`indexing`、`ready`、`failed` 和
`deleting`。同一租户重复 SHA-256 返回 `DOCUMENT_DUPLICATE`，但响应不会泄露
其他租户是否上传相同内容。

## 5. AI 工作流

### 5.1 接口与网关

- `AIClient` 只负责 OpenAI 兼容生成模型流。
- `EmbeddingClient` 负责 OpenAI 兼容 embedding 调用、分批、超时、取消、有限重试、
  数量和维度校验。
- `RerankClient` 负责可选内部 reranker；不可用时检索回退到 RRF。
- `AIWorkflow` 编排现有整理、报告、回忆和生成流。
- LiteLLM 提供生成逻辑模型 `diary-default` 和 embedding 逻辑模型
  `cortex-embedding`。
- 后端只持有 LiteLLM 虚拟密钥；供应商 Key 和 master key 不进入前端或业务数据。
- 发送给网关的观测元数据只包含内部租户标识、请求类型、环境和请求 ID。
- 缓存默认关闭，禁止跨租户共享 Prompt、响应或 embedding 内容缓存。

### 5.2 整理、报告与成长助手

- 整理接口流式生成草稿，确认接口才可新建或更新笔记。
- 报告按类型和日期确定周期；无来源返回 `REPORT_NO_SOURCES`。
- 确认报告时重新校验来源属于当前租户和周期，并写入 `report_sources`。
- 成长助手选择笔记本时先检索当前用户笔记，回答完成后保存会话、消息和 `message_sources`。
- 流式响应已经输出内容后不得从头重试。

## 6. 知识文件生命周期

### 6.1 上传与安全存储

知识文件接口支持：

| 方法 | 路径 | 当前用途 |
| --- | --- | --- |
| `GET` / `POST` | `/api/v1/knowledge/collections` | 列表和创建集合 |
| `GET` / `POST` | `/api/v1/knowledge/documents` | 列表和 multipart 上传 |
| `GET` | `/api/v1/knowledge/documents/{id}` | 获取元数据和状态 |
| `GET` | `/api/v1/knowledge/documents/{id}/download` | 鉴权下载 |
| `DELETE` | `/api/v1/knowledge/documents/{id}` | 立即失效并清理 |

上传流程：

1. 使用请求体和 multipart 限制，不信任客户端声明大小。
2. 校验扩展名、MIME、magic bytes 和实际容器结构。
3. 清理展示文件名，使用服务端 UUID 生成落盘路径。
4. 写入同目录临时文件并流式计算 SHA-256，再原子 rename。
5. 在租户事务中锁定配额，写入文档元数据并投递索引任务。
6. HTTP 请求立即返回，解析和 embedding 由后台 worker 完成。

原文件位于：

```text
DIARY_DATA_DIR/knowledge/<tenant>/<year>/<month>/<uuid>.<ext>
```

数据库只保存 `DIARY_DATA_DIR` 下的安全相对路径。知识目录不作为静态目录暴露。

### 6.2 解析器

| 类型 | 当前实现 | 结构与页码 | 资源与安全边界 |
| --- | --- | --- | --- |
| TXT | Go 标准库 UTF-8 读取 | 段落和单页语义 | 拒绝 NUL、非法 UTF-8、二进制和字符超限 |
| PDF | Poppler `pdftotext -layout` 子进程 | 通过 form-feed 保留页界 | 独立进程超时、页数和字符上限；稳定处理加密、无文本和失败 |
| DOCX | Go `archive/zip` + `encoding/xml` 流式解析 | Heading、列表、表格和显式分页 | ZIP 条目、解压总量、压缩比、路径和 XML 读取限制 |

TXT 和 DOCX 使用 Go 标准库（BSD-3-Clause）；PDF 使用由基础镜像安全更新维护的
Poppler（GPL-2.0-or-later）。当前不引入 Apache Tika。

- 扫描 PDF 不执行 OCR，返回 `DOCUMENT_OCR_REQUIRED`。
- 加密 PDF 不绕过密码，返回 `DOCUMENT_ENCRYPTED`。
- `.docm` 不在允许类型中，不执行宏。
- DOCX 校验 `[Content_Types].xml`，阻止 Zip Slip、ZIP bomb 和外部关系解析。
- 文件名、原始路径和提取正文不进入普通日志。

### 6.3 父子切块

提取器输出带 block 类型、页码、标题路径和顺序的稳定文档结构。切块遵循：

- 父块优先保持 Heading 小节、段落组、列表、表格或代码等完整结构。
- 超长结构递归拆分，父块保存页码、标题路径、顺序、token 数、邻接关系和内容摘要。
- PDF 提取会去除跨页重复的页眉页脚，并合并能够确定为连续语义的跨页段落。
- 超长 Markdown 表格按行组切割，每组重复原始表头，避免孤立数据行。
- 子块只在父块内部切割，采用约 200～500 token 和 10%～15% overlap。
- 子块可使用受控标题增强文本生成 embedding，同时单独保存原始引用文本。
- 父子块共享同一 `index_version`，数据库约束禁止孤儿 child。
- 中文 token 硬上限和 overlap 终止条件有单元测试覆盖。

### 6.4 异步索引

后端内置 worker：

1. 管理连接使用 `FOR UPDATE SKIP LOCKED` claim 到期任务。
2. claim 后使用业务连接设置任务对应的可信租户 RLS 上下文。
3. 验证文档和目标版本仍有效，再读取安全路径中的原文件。
4. 提取文档、构建父子块并计算 `content_hash`。
5. 经 LiteLLM 的 `cortex-embedding` 批量调用宿主机 Ollama
   `qwen3-embedding:0.6b`。
6. 写入新代次父块、子块和向量后，原子切换文档 active index version。
7. 清理旧代次；失败使用最大尝试次数、指数退避和有限租约重试。

worker panic 不影响 HTTP 进程，持久化错误不包含正文。

### 6.5 删除

删除事务锁定文档，设置 `deleting` 和 `deleted_at`，取消索引任务，并通过状态与版本检查
立即阻止后续检索。实现会尝试将原文件改名为受控 `.deleting` tombstone 后删除，并清理
父块、子块、FTS、embedding 和引用 snippet。删除步骤幂等；磁盘清理失败时 tombstone
保留为持久重试标记，后台清理器启动时及之后每分钟重试。所有检索只读取
`ready + deleted_at IS NULL + active index version` 的内容。

## 7. 混合检索与知识问答

### 7.1 检索链路

当前检索链路为：

```text
问题
  -> PostgreSQL FTS Top-30
  -> pgvector 精确向量 Top-30
  -> Reciprocal Rank Fusion
  -> collection/document/status/active-version 过滤
  -> 可选 Qwen3 reranker 对 Top-20 重排
  -> child 去重与 parent 聚合
  -> 在统一预算内选择相邻 parent
  -> 统一 token 预算
  -> child 引用 + parent 生成上下文
```

- FTS 负责术语、专有名词和精确原词，向量负责同义表达。
- collection/document ID 逐一验证当前租户归属。
- SQL 同时使用 RLS、显式 `tenant_id` 和复合租户连接。
- 每个 parent 和每个文件的候选数量受限，避免单一来源挤占上下文。
- reranker 不可用时使用 RRF；embedding 不可用时退化为 FTS。
- 无可用证据时返回 `KNOWLEDGE_NO_EVIDENCE`。

### 7.2 统一来源知识 Chat

当前接口：

| 方法 | 路径 | 当前用途 |
| --- | --- | --- |
| `GET` / `POST` | `/api/v1/conversations` | 查询或创建知识/成长助手会话 |
| `GET` / `DELETE` | `/api/v1/conversations/{id}` | 读取含消息的会话或删除会话 |
| `POST` | `/api/v1/knowledge/chat` | 在选择的集合/文件范围内执行 SSE 问答 |
| `GET` | `/api/v1/knowledge/messages/{id}/sources` | 读取知识引用 |

Chat 的 `source_scope` 支持 `knowledge`、`growth` 和 `all`。知识文件与成长记录转换为统一
来源 DTO，仍分别在可信业务层验证租户归属并持久化引用。`request_id` 在租户内唯一；
已完成请求的重试重放已保存回答，进行中的同键请求返回 `REQUEST_IN_PROGRESS`。

SSE 使用 `retrieval`、`delta`、`sources`、`error` 和 `done` 具名事件。`done` 返回
`conversation_id` 与 `message_id`。服务端规则将知识材料视为不可信数据，要求回答只能
依据召回证据，材料不足必须拒答。流已经输出内容后不从头重试，客户端断线取消上游请求。
AI 未配置或不可用时，知识文件管理仍可使用。

## 8. Scheduler

- 支持 daily、weekly、monthly。
- 时间按任务 IANA 时区计算，数据库保存 UTC。
- worker 使用管理连接执行 `FOR UPDATE SKIP LOCKED` claim，并设置有限租约。
- 状态持久化为 running、success、failed；手动重试立即返回 queued。
- 多 worker 争抢同一任务只产生一条 run。

## 9. 前端

### 9.1 通用界面

- React 18、TypeScript、Webpack 5、Ant Design 和 TanStack Query。
- `/settings` 管理当前浏览器主题，不展示 AI 地址、模型或密钥。
- 主题为 `system`、`light` 或 `dark`，保存在
  `localStorage` 的 `diary-listener.theme`。
- 主题覆盖页面、Ant Design、Markdown 编辑器和浏览器 `theme-color`。

### 9.2 知识库与成长助手

- `/knowledge` 提供集合树选择、文件名搜索、状态筛选、服务端分页、TXT/PDF/DOCX
  拖拽上传、状态轮询、详情抽屉、提取预览、鉴权下载、重新索引和删除。
- 知识 API 类型与请求封装位于 `frontend/src/api/knowledge.ts`。
- 成长助手页面提供会话列表及 CRUD、三种来源范围、集合/ready 文件多选、安全 Markdown、
  SSE 增量渲染、停止生成、重复提交保护、引用卡片和已删除来源降级。
- 界面复用现有主题系统和路由保护。

## 9.3 可观测性

- `/metrics` 使用 Prometheus 文本格式输出知识索引队列、失败计数、累计处理耗时、
  检索请求计数和累计检索延迟。
- 指标只记录数量和时间，不记录问题、回答、正文、文件名、邮箱或租户标识。
- `/healthz` 仅表示进程存活；`/readyz` 独立验证数据库可用性。

## 10. 配置

Compose 必需的敏感配置：

- `DATABASE_URL`
- `MIGRATION_DATABASE_URL`
- `POSTGRES_APP_PASSWORD`
- `POSTGRES_MIGRATOR_PASSWORD`
- `LITELLM_DB_PASSWORD`
- `LITELLM_MASTER_KEY`
- `LITELLM_VIRTUAL_KEY`
- `KIMI_API_KEY`
- `OPENAI_API_KEY`

主要运行配置：

- `LISTEN_ADDRESS`、`CORS_ORIGINS`、`DIARY_DATA_DIR`
- `MAX_ATTACHMENT_BYTES`
- `KNOWLEDGE_MAX_FILE_BYTES`、`KNOWLEDGE_TENANT_QUOTA_BYTES`
- `KNOWLEDGE_MAX_PDF_PAGES`、`KNOWLEDGE_MAX_EXTRACTED_CHARS`
- 父块、子块、overlap、任务租约、重试和 `RAG_INDEX_WORKERS` 限制
- `RAG_EMBEDDING_BASE_URL`、`RAG_EMBEDDING_MODEL`、`RAG_EMBEDDING_DIMENSIONS`
- `RAG_RERANK_BASE_URL`、`RAG_RERANK_MODEL`
- `TOKEN_TTL_HOURS`、`DB_POOL_SIZE`、`DB_STATEMENT_TIMEOUT_MS`
- scheduler 开关和轮询周期

所有限制必须为安全正数，child overlap 必须小于 child size，child 上限必须小于 parent 上限。

## 11. 部署、健康与迁移

- Docker Compose 编排 PostgreSQL、LiteLLM、Go 后端、前端和可选本地 reranker。
- PostgreSQL 使用固定 PostgreSQL 16 + pgvector 镜像版本。
- `db` 和 `llm-gateway` 不暴露宿主机端口。
- 后端降权运行并挂载持久化 `app_data`。
- 宿主机 Ollama 提供 `qwen3-embedding:0.6b`；后端不得绕过 LiteLLM 直连。
- 可选 `local-ai` Profile 启动 Qwen3 reranker；模型从官方源的固定 revision
  构建到本地 CPU 镜像，运行时离线加载，并配置 CPU、内存和进程限制。
- `/healthz` 只反映进程存活且不依赖 AI。
- `/readyz` 验证数据库，不因 embedding 或 reranker 故障使整个服务不可用。

数据库升级规则：

- `backend/db/schema.sql` 是新实例初始化基线。
- 已部署数据库使用 `backend/cmd/migrate` 执行 `up`、`down` 和 `status`。
- 每个版本同时提供 `.up.sql` 和 `.down.sql` 并记录 SHA-256。
- 迁移持有 PostgreSQL advisory lock，每个版本在独立事务内执行。
- Compose 后端入口在启动服务前调用独立 `migrate` 二进制。
- 服务进程不执行临时 DDL。

## 12. 安全与隐私

- 原文件、提取文本、父子块、embedding、问题、回答和引用均视为私人数据。
- 上传目录不公开，落盘文件名由服务端生成。
- 下载和删除必须认证并验证租户。
- 文件处理阻止路径穿越、Zip Slip、ZIP bomb、XML 外部实体和解析超时。
- 普通日志和审计不记录正文、引用片段、原文件路径或用户文件名。
- embedding 只接收所需子块或查询文本；生成模型只接收最终选中的父块上下文。
- 文档内容不能覆盖服务端 system 规则。
- AI 不能自动创建、修改或删除知识文件。
- 管理连接不能执行普通 RAG 查询。

## 13. 测试基线

仓库当前包含：

- Go 单元测试、静态检查和 server/migrate 构建。
- 前端格式检查、Vitest 和生产构建。
- 认证、租户、笔记、附件、调度和 AI 工作流测试。
- 知识文件签名、DOCX 结构与损坏输入、提取字符限制测试。
- 父子切块、overlap、hash、token 上限和上下文预算测试。
- embedding 数量/维度、分批、超时、取消和重试测试。
- FTS/vector/RRF、过滤、降级和知识来源测试。
- Compose 配置检查、非 AI 冒烟和 AI 验收脚本。

构建或单元测试通过只证明代码基线可构建，不等同于生产验收通过。真实模型、双租户、
空库、PDF/DOCX、删除并发、性能和容器持久化等剩余验证统一记录在
[实现与生产验收待办](IMPLEMENTATION_GAPS.md)。
