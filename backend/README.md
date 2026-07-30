# Cortex Go Backend

Cortex 的唯一后端实现，使用 Gin、pgx/v5 和 PostgreSQL。Go module、环境变量、
数据库角色与数据目录继续保留旧技术标识，兼容策略见
[`docs/COMPATIBILITY.md`](../docs/COMPATIBILITY.md)。

主要能力：

- 认证、个人租户与 PostgreSQL RLS；
- 笔记、版本、标签、搜索和 dashboard；
- 附件和 Markdown 导出；
- LiteLLM Proxy 上的 SSE、AI 整理、报告、回忆与 HowToCook 菜谱问答；
- `/api/v1` 菜谱推荐、问答、来源和 Prometheus 文本指标；
- 并发安全的定时报表 scheduler。

## 本地验证

```powershell
go vet ./...
go test ./...
go build ./cmd/server
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
```

菜谱模型由 Compose 内部服务从 ModelScope 固定 revision 构建并离线运行；
Embedding 输出严格为 512 维，Reranker 使用 BGE CrossEncoder。

知识库唯一来源是仓库内 `resources/howtocook`。用户、研究任务和个人笔记没有写入入口，
后端也不提供 `/api/v1/knowledge/*` 文档管理接口。

今日菜谱语料固定存放在 `resources/howtocook`，服务启动时会按 `SOURCE.json` revision
幂等同步并排队生成 512 维向量。Compose 环境使用固定 revision 的
`iic/nlp_gte_sentence-embedding_chinese-small` 和 `BAAI/bge-reranker-v2-m3`。
完整环境就绪后运行 `scripts/recipe_sync_acceptance.ps1` 验证推荐稳定性、三个建议问题、
偏好乐观锁和语料 revision。
