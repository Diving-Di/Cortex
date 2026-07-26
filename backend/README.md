# Diary Listener Go Backend

Diary Listener 的唯一后端实现，使用 Gin、pgx/v5 和 PostgreSQL。

主要能力：

- 认证、个人租户与 PostgreSQL RLS；
- 笔记、版本、标签、搜索和 dashboard；
- 附件、Markdown 导出、备份与恢复；
- LiteLLM Proxy 上的 SSE、AI 整理、报告与回忆问答；
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
`diary-default` 且带预算的虚拟密钥，再将结果写入部署 Secret
`LITELLM_VIRTUAL_KEY`。应用不得使用 `LITELLM_MASTER_KEY`。

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
```
