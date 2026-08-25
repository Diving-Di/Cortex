# 个人知识库页

`/knowledge` 是个人知识库 v2 的入口，用于上传 Markdown / Markdown ZIP 资料、查看文档索引
状态与容量配额、删除不再需要的知识文档，并提供知识问答、历史会话、检索过程、来源与反馈。
问答使用 `POST /api/v1/knowledge/chat/stream`；`/recipes` 与 `/assistant` 已重定向到本页。

## 页面目标、范围与非目标

- 目标：让用户把个人 Markdown 资料变成可检索、可问答的私有知识库；提供上传、状态跟踪、
  容量管理和删除能力；页面与 API 只面向当前租户。
- 范围：`.md` 单文件与包含 Markdown 的 `.zip`（仅处理 `.md`、`.png`、`.jpg`，其他类型条目跳过）；个人笔记
  可通过 `PATCH /api/v1/notes/{id}/knowledge` 加入问答；每租户容量上限 3 GiB。
- 非目标：PDF/Word 解析、OCR、图片向量检索、团队共享、外部对象存储、压缩包在线预览、
  数据库与 Markdown 双向同步。

## 页面区域与交互

- 上传区：拖放或选择单个 `.md` / `.zip`，上传前展示格式与配额说明；成功后返回 202 并异步
  建立索引。
- 配额区：展示已用、剩余容量和进度条，容量判断以后端返回为准。
- 文档列表：显示标题、来源类型（上传资料 / 个人笔记）、大小、索引状态与失败摘要，支持删除。
- 问答区：支持新会话或选择历史 knowledge 会话、停止当前请求、展示完整或 incomplete 回答，
  完成后可提交“答案不正确”或“引用无依据”反馈。
- 检索过程：默认以标签展示公开阶段与耗时，来源以折叠列表展示引用编号、标题和章节路径。
- 澄清恢复：收到 `KNOWLEDGE_CLARIFICATION_REQUIRED` 后展示服务端安全提示；用户最多补充
  1000 字并继续原请求，前端不得修改原问题、集合或 tenant。
- 状态：首次索引使用 `uploaded`、`parsing`、`indexing`、`ready`、`failed`，删除使用
  `deleting`；已有活动版本的文档重建时保持 `ready`，另由 `index_job_status` 展示后台任务状态，
  重建失败记录 `last_index_failure_code` 且旧版本继续服务。任务详情用 `index_stage` 展示
  `queued/loading/parsing/embedding/persisting/completed/failed`，并用
  `processed_chunks/total_chunks` 计算进度。

## 前端数据流

- `useQuery(['knowledge'])` 每 5 秒轮询 `GET /api/v1/knowledge/documents` 刷新列表与配额。
- `useQuery(['knowledge-conversations'])` 加载 `source_scope=knowledge` 的历史会话；切换会话时只读取
  当前用户可见消息。
- POST SSE 使用 `fetch` + `ReadableStream` 解析命名事件；组件卸载和用户停止时通过
  `AbortController` 取消。只有 `done` 表示完成，`error.incomplete=true` 保留已收到的部分回答且不
  自动从头重试。
- 上传使用 `multipart/form-data` 的 mutation；删除使用确认弹窗（Popconfirm）后调用 DELETE。
- 上传/删除成功或失败均展示稳定提示；失败只显示后端返回的 `code`/`message`，不含内部路径。

## HTTP API

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/api/v1/knowledge/uploads` | 上传 `.md` / `.zip`（multipart，支持幂等键），安全落盘后返回 202 |
| `GET` | `/api/v1/knowledge/uploads/{id}` | 查询上传与索引状态 |
| `GET` | `/api/v1/knowledge/documents` | 列出当前租户文档与 3 GiB 配额 |
| `DELETE` | `/api/v1/knowledge/documents/{id}` | 删除文档并使其退出检索 |
| `GET` | `/api/v1/knowledge/documents/{id}/assets/{asset_id}` | 鉴权读取文档图片 |
| `POST` | `/api/v1/knowledge/documents/{id}/retry` | 重试失败的解析或索引任务 |
| `GET` / `POST` | `/api/v1/knowledge/collections` | 查询或创建知识集合 |
| `POST` | `/api/v1/knowledge/chat/stream` | 混合检索、精排并 SSE 回答 |
| `PATCH` | `/api/v1/notes/{id}/knowledge` | 开启或关闭笔记知识索引 |

## 后端组件与持久化模型

- 上传校验在 `backend/internal/knowledge`（archive/chunker）：校验类型、配额、ZIP 路径安全；ZIP 中仅解析 `.md`、`.png`、`.jpg`，其余类型条目跳过
  （拒绝绝对路径、盘符、`..`、符号链接、超高压缩比等），文件保存到
  `CORTEX_DATA_DIR/knowledge/{tenant_id}/{upload_id}/source` 安全相对路径。
- 数据库迁移 `000017_personal_knowledge_v2` 新增九张表并启用 RLS：
  `knowledge_quotas`、`knowledge_collections`、`knowledge_uploads`、`knowledge_documents`、
  `knowledge_assets`、`knowledge_parent_chunks`、`knowledge_child_chunks`、
  `knowledge_index_jobs`、`knowledge_message_sources`。
- `000035_knowledge_index_progress` 为索引 job 增加阶段、已处理块数与总块数；更新必须持有当前未
  过期 lease，同阶段数值只能前进。
- `000036_knowledge_clarifications` 新增 RLS 隔离的一次性恢复状态；状态绑定 tenant、user、
  conversation 和原 request ID，collection scope 只读取服务端保存值。
- 后台 `RunKnowledgeIndexer`（`backend/internal/server/knowledge_worker.go`）按 Markdown 标题
  切分 parent 与不超过 500 字的 child，经 Compose 内部 `embedding-service`
  （`iic/nlp_gte_sentence-embedding_chinese-small`，512 维）生成向量写入
  `knowledge_child_chunks`（pgvector + 全文 tsvector）。
- 新索引在单个事务中写完后切换 `active_index_version`；成功激活 N 后保留 N 与 N-1，清理更早
  chunk，历史消息来源中的标题、摘要和版本元数据继续保留。
- 限额配置：`KNOWLEDGE_MAX_UPLOAD_BYTES`（256 MiB）、`KNOWLEDGE_MAX_EXTRACTED_BYTES`（1 GiB）、
  `KNOWLEDGE_MAX_FILE_BYTES`（64 MiB）、`KNOWLEDGE_MAX_FILES`（5000）、
  `KNOWLEDGE_MAX_DEPTH`（16）、`KNOWLEDGE_MAX_COMPRESSION_RATIO`（100）。

## 检索与问答链路

检索实现由 `RAG_RETRIEVAL_BACKEND` 选择：Compose 默认使用 Elasticsearch BM25 + KNN 投影；
`postgres` 回退使用下述 pgvector + 中文全文通道。两种后端的候选都必须回 PostgreSQL 做租户、
活动索引版本与有效状态校验，再进入 rerank、证据门控和来源保存。

`POST /api/v1/knowledge/chat/stream`：

1. 有会话历史时先生成独立检索 Query；普通问题走单查询快速路径。默认关闭的实验计划器只为
   明确比较/趋势/跨周期问题生成最多 4 个查询；
2. `LocalEmbeddingClient` 对检索 Query 批量生成 512 维向量；
3. `Store.SearchKnowledge` 在每个子查询的 `pgx.Tx` 内设置同一 RLS 上下文，只检索当前租户
   `status='ready'`、`knowledge_enabled`、`index_version=active_index_version` 的文档
   （可选 `collection_ids` 过滤），向量 + 全文召回用 RRF 融合；
4. `reranker-service`（`BAAI/bge-reranker-v2-m3`）精排，取前 `RAG_CONTEXT_PARENT_TOP_K` 个
   parent；
5. 证据门控不足时区分歧义、范围冲突和确实无知识。前两者返回一次性澄清状态，恢复只执行一次
   定向检索；无证据或恢复仍失败不调用生成；
6. `AnswerKnowledge` 经 LiteLLM 生成并核验，SSE 先发送版本化 `retrieval_progress`，随后发送
   `retrieval`、核验状态、正文、`sources` 与 `done`；来源写入 `knowledge_message_sources`；
7. 无当前租户证据返回 `KNOWLEDGE_NO_EVIDENCE`；Embedding / Reranker 不可用分别返回
   `KNOWLEDGE_EMBEDDING_UNAVAILABLE` / `KNOWLEDGE_RERANK_UNAVAILABLE`。

## 租户、安全、降级与删除

- 知识数据属于当前租户并受 RLS 保护；客户端提交的 `tenant_id` 始终被忽略，跨租户访问统一
  404。
- 文件不通过公开静态目录暴露；上传/下载/删除均需认证，下载图片带 `private` 缓存头。
- AI / Embedding 不可用时，上传与删除仍可用；索引任务标记失败，问答返回稳定错误码。
- 删除文档立即退出检索。

## 测试与验收

- 覆盖：`.md` / `.zip` 上传与配额（含并发预占）、跨租户 404 隔离、文档删除后退出检索、
  笔记知识开关、公开 progress DTO、混合问答来源与 incomplete、澄清正常/重复/过期/跨租户恢复、
  简单问题不规划、子查询/恢复次数上限、索引进度 fencing、`KNOWLEDGE_NO_EVIDENCE` 和模型降级。
- 端到端：`non_ai_smoke.ps1`、`ai_acceptance.ps1`。
