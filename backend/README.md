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
