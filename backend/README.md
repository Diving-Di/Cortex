# Cortex Go Backend

Cortex 的唯一后端实现，使用 Gin、pgx/v5 和 PostgreSQL。Go module、环境变量、
数据库角色与数据目录继续保留旧技术标识（`diary-*` / `CORTEX_DATA_DIR`）。

主要能力：

- 认证、个人租户与 PostgreSQL RLS；
- 笔记、版本、标签、搜索和 dashboard；
- 附件和 Markdown 导出；
- LiteLLM Proxy 上的 SSE、AI 整理、报告、回忆与个人知识库问答；
- `/api/v1` 个人知识库上传、集合、文档与问答，以及 Prometheus 文本指标；
- 并发安全的定时报表 scheduler。

## 本地验证

```powershell
go vet ./...
go test ./...
go build ./cmd/server
go build ./cmd/migrate
```

## 版本化数据库迁移

服务启动时不会自动修改数据库。发布前使用迁移角色显式执行：

```powershell
$env:MIGRATION_DATABASE_URL = "postgresql://diary_migrator:<password>@127.0.0.1:5432/diary_listener"
go run ./cmd/migrate status
go run ./cmd/migrate -steps=0 up
go run ./cmd/migrate -steps=1 down
```

迁移文件位于 `internal/migrations/sql`，每个版本必须同时提供 `.up.sql` 和
`.down.sql`。命令在单一数据库会话中持有 PostgreSQL advisory lock，每个版本在
独立事务中执行，并校验已应用迁移的 SHA-256。

LiteLLM 首次启动后，用 `scripts/provision-litellm-key.ps1` 签发仅允许
`diary-default`、`cortex-embedding` 且带预算的虚拟密钥。传入
`-EnvironmentFile ..\..\.env` 可在不显示 key 的情况下原子更新本地 Compose
Secret；生产环境应将值写入其 Secret 管理系统。应用不得使用 `LITELLM_MASTER_KEY`。

## Docker Compose

在仓库根目录执行：

```powershell
docker compose up --build
```

后端监听 `127.0.0.1:8000`，前端监听 `127.0.0.1:5173`。

运行时配置：

```text
DATABASE_URL=postgresql://diary_app:<password>@db:5432/diary_listener
MIGRATION_DATABASE_URL=postgresql://diary_migrator:<password>@db:5432/diary_listener
CORS_ORIGINS=http://127.0.0.1:5173,http://localhost:5173
LISTEN_ADDRESS=0.0.0.0:8000
AI_BASE_URL=http://llm-gateway:4000/v1
AI_API_KEY=<gateway-key>
AI_MODEL=diary-default
RAG_EMBEDDING_BASE_URL=http://embedding-service:4000/v1
RAG_EMBEDDING_MODEL=iic/nlp_gte_sentence-embedding_chinese-small
RAG_EMBEDDING_DIMENSIONS=512
RAG_RERANK_BASE_URL=http://reranker-service:8080
RAG_RERANK_MODEL=BAAI/bge-reranker-v2-m3
RAG_VECTOR_TOP_K=15
RAG_TITLE_TOP_K=10
RAG_KEYWORD_TOP_K=15
RAG_FUSION_TOP_K=20
RAG_CONTEXT_PARENT_TOP_K=5
```

知识库模型由 Compose 内部服务从 ModelScope 固定 revision 构建并离线运行；
Embedding 输出严格为 512 维，Reranker 使用 BGE CrossEncoder。
个人知识库按 Markdown 标题切分 parent 与不超过 500 字的 child，向量与全文检索使用 RRF
融合，rerank 后按 parent 展开供生成使用。

个人知识库来源是当前租户上传资料与主动开启的笔记。历史 HowToCook 内置语料已一次性迁移到
用户 `Diving` 的私有知识库，不再随应用镜像分发。运行时文件统一保存到
`CORTEX_DATA_DIR/knowledge/<tenant-uuid>/<upload-uuid>/source`，
并通过 `/api/v1/knowledge/*` 管理与问答接口访问。
Compose 环境使用固定 revision 的
`iic/nlp_gte_sentence-embedding_chinese-small` 和 `BAAI/bge-reranker-v2-m3`。
完整环境就绪后运行 `scripts/non_ai_smoke.ps1` 与 `scripts/ai_acceptance.ps1` 验证
知识库上传、索引、问答与降级。

## RAG 离线评测

个人知识库 v2 提供离线入口 `cmd/rag-eval`。它固定解析用户 `Diving` 的服务端 Principal，
在该用户全部启用的 ready 文档上运行生产 `SearchKnowledge`、Embedding、Reranker 和
`AnswerKnowledge` 链路，不接受客户端或命令行传入的 `tenant_id`，也不要求文档属于唯一集合。

评测复用 `testdata/rag/recipe_eval_v1.jsonl` 的 90 条问题、参考答案和 gold 文件名。启动前会确认
每个 gold 文件在 Diving 的知识库中存在、索引版本有效且至少有一个 embedding；同名文档均视为
有效 gold。运行：

```powershell
.\scripts\rag_eval.ps1 -Workers 4
# 快速检查指定样本
.\scripts\rag_eval.ps1 -CaseIDs "recipe-001,recipe-002"
```

结果写入 `artifacts/rag-eval/<timestamp>/`。候选正文只在进程内用于生成与 Judge，trace 默认只保存
文档 ID、标题、章节、索引版本、排名和分数，不写入完整检索正文。

个人知识库检索 v3 会跳过只有 Markdown heading、没有正文的 parent，并通过迁移
`000019_knowledge_retrieval_v3` 为现有 ready 文档排队 `active_index_version + 1`；旧索引会持续服务，
直到 worker 在同一事务中写完新 parent/child 和 embedding 后才切换。检索使用向量、正文 FTS、
标题命中文档内向量召回三条通道，在 parent 层去重并 RRF；rerank 后先确定文档，再选该文档内章节。
如果问题明确包含第一名文档的规范化标题，最终上下文只从该文档选择；否则最多保留三个文档。
