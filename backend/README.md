# Cortex Go Backend

Cortex 的唯一后端实现，使用 Gin、pgx/v5 和 PostgreSQL。Go module、环境变量、
数据库角色与数据目录继续保留旧技术标识，兼容策略见
[`docs/COMPATIBILITY.md`](../docs/COMPATIBILITY.md)。

主要能力：

- 认证、个人租户与 PostgreSQL RLS；
- 笔记、版本、标签、搜索和 dashboard；
- 附件、知识原文件和 Markdown 导出；
- LiteLLM Proxy 上的 SSE、AI 整理、报告、回忆与统一来源知识问答；
- `/api/v1` 会话、知识文件预览/下载/重新索引和 Prometheus 文本指标；
- 旧聊天和轻日记兼容接口；
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
KNOWLEDGE_MAX_FILE_BYTES=52428800
KNOWLEDGE_MAX_PDF_PAGES=500
KNOWLEDGE_MAX_EXTRACTED_CHARS=5000000
RAG_INDEX_WORKERS=2
RAG_PARENT_TARGET_TOKENS=1800
RAG_PARENT_MAX_TOKENS=2500
RAG_CHILD_TARGET_TOKENS=350
RAG_CHILD_MAX_TOKENS=500
RAG_CHILD_OVERLAP_TOKENS=50
RAG_EMBEDDING_BASE_URL=http://llm-gateway:4000/v1
RAG_EMBEDDING_MODEL=cortex-embedding
RAG_EMBEDDING_DIMENSIONS=1024
RAG_RERANK_BASE_URL=http://reranker-service:8080
RAG_RERANK_MODEL=Qwen/Qwen3-Reranker-0.6B
```

`cortex-embedding` 默认由 LiteLLM 转发到宿主机 Ollama 的
`qwen3-embedding:0.6b`。模型输出固定为 1024 维，本地接口不需要付费
供应商 API Key。Reranker 使用同一 Qwen3 家族的
`Qwen/Qwen3-Reranker-0.6B`，由本地服务从官方模型源构建并离线运行。

知识文件上传、下载和删除不依赖生成模型。索引 worker 对 embedding 请求按 16 条分批，
对 429/502/503/504 和网络瞬时错误最多重试 2 次；embedding 不可用时文档保持失败状态并可
后续重建索引。知识问答的 embedding 不可用时退化为 FTS，reranker 不可用时退化为 RRF；
生成模型未配置时返回 `AI_NOT_CONFIGURED`，不影响知识文件管理。

父子块配置在启动时做强校验：target 不得大于 max，child max 必须小于 parent max，
overlap 必须小于 child target。PDF 提取受 45 秒 worker 超时、页数和字符数限制，
并处理重复页眉页脚与可判定的跨页续接；DOCX 还受 ZIP 条目、解压规模和压缩比限制。
表格过长时按行组切分并重复表头，检索命中父块边界时在统一预算内加入相邻 parent。

`GET /metrics` 输出知识索引队列、失败数、累计处理时间以及检索请求数和累计延迟。
指标不包含问题、正文、文件名或租户身份。知识文件删除失败留下的受控
`.deleting` tombstone 会由后台清理器持续重试。

可重复的知识库验收使用 `scripts/knowledge_acceptance.ps1`。它会创建两个临时租户，
上传 `testdata/knowledge` 中的 TXT/PDF/DOCX 合成资料，验证跨租户隔离、完整 RAG、
引用、无答案和删除后不可召回，并对 `testdata/rag/evaluation.jsonl` 的质量与延迟
门槛硬失败。实际 reranker 路径需先启动 Compose 的 `local-ai` profile。
