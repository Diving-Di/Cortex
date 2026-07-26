# Diary Listener 工程基线

> 状态：当前有效
> 更新日期：2026-07-26

## 产品范围

- 普通笔记、日报、周报和月报。
- 标签、附件、历史版本、中文搜索和 dashboard。
- AI 整理、报告、来源引用和回忆问答。
- 每个账号对应一个服务端解析的个人租户。
- Markdown ZIP 导出、完整备份和空租户恢复。
- 旧聊天和图片轻日记兼容接口。

桌面组件、团队协作、计费和数据库/Markdown 双向同步不属于当前范围。

## 技术基线

| 层级 | 当前实践 |
| --- | --- |
| 前端 | React 18、TypeScript、Webpack 5、Ant Design |
| 后端 | Go 1.26、Gin、标准 `net/http`、`gofmt` |
| 数据访问 | pgx/v5、显式 SQL、显式事务 |
| 数据库 | PostgreSQL 16、RLS |
| AI | LiteLLM Proxy、OpenAI 兼容 Chat Completions、SSE |
| 部署 | Docker Compose、多阶段静态 Go 镜像 |

后端唯一入口为 `backend/cmd/server/main.go`。仓库不保留 Python 后端或 Alembic。

## 数据规则

- PostgreSQL 是笔记正文的唯一权威来源。
- `DATABASE_URL` 使用低权限 `diary_app`。
- `MIGRATION_DATABASE_URL` 使用 `diary_migrator`，仅供版本化迁移和调度 claim。
- 每次租户业务查询必须在同一个 `pgx.Tx` 内设置 transaction-local RLS 上下文。
- 客户端不得提交或选择可信 `tenant_id`。
- `backend/db/schema.sql` 是经过空库验收的初始化基线。
- 后续 Schema 变化必须新增版本化迁移，不得直接修改已部署数据库。

## 配置与密钥

- 服务只读取环境变量。
- `.env` 只用于本地运行并必须被 Git 忽略。
- 供应商 Key 仅注入 LiteLLM；业务后端只持有网关密钥。
- Key 不得进入前端、URL、日志、审计记录、备份或文档。

## 必须通过的验证

```powershell
Set-Location backend
gofmt -l .
go vet ./...
go test ./...
go build ./cmd/server
go build ./cmd/migrate

Set-Location ..
docker compose config --quiet
helm lint .\packaging\helm\diary-listener
helm template diary-listener .\packaging\helm\diary-listener | Out-Null
docker compose up -d --build
.\backend\scripts\non_ai_smoke.ps1
.\backend\scripts\ai_acceptance.ps1
```

- 所有 Go 源码必须通过 `gofmt`；允许使用 Go 惯例中的 Tab 缩进。
- `db`、`llm-gateway` 和 `backend` 必须为 healthy。
- 新 PostgreSQL 空库必须完成 18 张表、RLS、注册和登录验收。
