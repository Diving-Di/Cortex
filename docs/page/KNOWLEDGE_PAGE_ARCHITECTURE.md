# 个人知识库页

`/knowledge` 是个人知识库 v2 的入口，用于上传 Markdown / Markdown ZIP 资料、查看文档索引
状态与容量配额、删除不再需要的知识文档，并作为知识问答（`/api/v1/knowledge/chat/stream`）
的来源管理页面。`/recipes` 与 `/assistant` 已重定向到本页。

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
- 状态：`uploaded`、`parsing`、`indexing`、`ready`、`failed`、`deleting`；失败文档保留
  稳定错误摘要，不展示服务器路径。

## 前端数据流

- `useQuery(['knowledge'])` 每 5 秒轮询 `GET /api/v1/knowledge/documents` 刷新列表与配额。
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
- 后台 `RunKnowledgeIndexer`（`backend/internal/server/knowledge_worker.go`）按 Markdown 标题
  切分 parent 与不超过 500 字的 child，经 Compose 内部 `embedding-service`
  （`iic/nlp_gte_sentence-embedding_chinese-small`，512 维）生成向量写入
  `knowledge_child_chunks`（pgvector + 全文 tsvector）。
- 限额配置：`KNOWLEDGE_MAX_UPLOAD_BYTES`（256 MiB）、`KNOWLEDGE_MAX_EXTRACTED_BYTES`（1 GiB）、
  `KNOWLEDGE_MAX_FILE_BYTES`（64 MiB）、`KNOWLEDGE_MAX_FILES`（5000）、
  `KNOWLEDGE_MAX_DEPTH`（16）、`KNOWLEDGE_MAX_COMPRESSION_RATIO`（100）。

## 检索与问答链路

`POST /api/v1/knowledge/chat/stream`：

1. `LocalEmbeddingClient` 对问题生成 512 维向量；
2. `Store.SearchKnowledge` 在同一个 `pgx.Tx` 内设置 RLS 上下文，只检索当前租户
   `status='ready'`、`knowledge_enabled`、`index_version=active_index_version` 的文档
   （可选 `collection_ids` 过滤），向量 + 全文召回用 RRF 融合；
3. `reranker-service`（`BAAI/bge-reranker-v2-m3`）精排，取前 `RAG_CONTEXT_PARENT_TOP_K` 个
   parent；
4. `AnswerKnowledge` 经 LiteLLM 流式生成，SSE 事件为 `retrieval` → `delta` → `sources` →
   `done`；来源写入 `knowledge_message_sources`；
5. 无当前租户证据返回 `KNOWLEDGE_NO_EVIDENCE`；Embedding / Reranker 不可用分别返回
   `KNOWLEDGE_EMBEDDING_UNAVAILABLE` / `KNOWLEDGE_RERANK_UNAVAILABLE`。

## 租户、安全、降级与删除

- 知识数据属于当前租户并受 RLS 保护；客户端提交的 `tenant_id` 始终被忽略，跨租户访问统一
  404。
- 文件不通过公开静态目录暴露；上传/下载/删除均需认证，下载图片带 `private` 缓存头。
- AI / Embedding 不可用时，上传与删除仍可用；索引任务标记失败，问答返回稳定错误码。
- 删除文档立即退出检索；完整备份（`cortex-full-backup-v1`）不包含 `knowledge_*` 数据。

## 测试与验收

- 覆盖：`.md` / `.zip` 上传与配额（含并发预占）、跨租户 404 隔离、文档删除后退出检索、
  笔记知识开关、混合问答来源保存与 `KNOWLEDGE_NO_EVIDENCE`、Embedding/Reranker 降级。
- 端到端：`non_ai_smoke.ps1`、`ai_acceptance.ps1`。
