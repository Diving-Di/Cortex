# Cortex 工程基线

> 状态：当前有效
> 更新日期：2026-08-05

## 产品范围

- 普通笔记、日报、周报和月报。
- 标签、附件、历史版本、中文搜索和 dashboard。
- AI 整理、报告、来源引用和回忆问答。
- 每个账号对应一个服务端解析的个人租户。
- Markdown ZIP 内容导出和笔记版本恢复。
- 用户自主上架的 Markdown 模板广场，以及每日限量 AI 深度月报活动。
- 个人知识库：上传 Markdown / Markdown ZIP、知识集合、个人笔记入库开关与混合问答（每租户 3 GiB 配额）。

桌面组件、团队协作、计费和数据库/Markdown 双向同步不属于当前范围。

> 2026-08-05：HowToCook 固定语料与菜谱接口（`/api/v1/recipes/*`）已移除，语料一次性迁移到用户
> `Diving` 的运行时私有知识库；前端 `/recipes`、`/assistant` 已重定向到 `/knowledge`。

## 技术基线

| 层级 | 当前实践 |
| --- | --- |
| 前端 | React 18、TypeScript、Webpack 5、Ant Design |
| 后端 | Go 1.26、Gin、标准 `net/http`、`gofmt` |
| 数据访问 | pgx/v5、显式 SQL、显式事务 |
| 数据库 | PostgreSQL 16、RLS |
| AI | LiteLLM Proxy、OpenAI 兼容 Chat Completions、SSE |
| 知识检索 | 个人知识库 GTE 中文 Embedding（512 维）、BGE CrossEncoder Reranker、pgvector 混合召回 |
| 部署 | Docker Compose、多阶段静态 Go 镜像 |
| 活动协调 | Redis 7、Lua 原子预扣；PostgreSQL 保存最终事实 |

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
- 个人知识库使用 Compose 内部 `embedding-service` 加载固定 revision 的
  `iic/nlp_gte_sentence-embedding_chinese-small`（512 维）。
- 知识检索精排使用 Compose 内部 `reranker-service` 加载固定 revision 的
  `BAAI/bge-reranker-v2-m3`；两个模型服务均不暴露宿主机端口。
- 个人知识库上传限制由 `KNOWLEDGE_MAX_*` 环境变量控制；每租户容量上限 3 GiB。

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
docker compose up -d --build
.\backend\scripts\non_ai_smoke.ps1
.\backend\scripts\ai_acceptance.ps1
```

- 所有 Go 源码必须通过 `gofmt`；允许使用 Go 惯例中的 Tab 缩进。
- `db`、`llm-gateway` 和 `backend` 必须为 healthy。
- 固定 GTE Embedding 必须通过单条、批量、中英文、维度异常和不可用降级验收。
- 新 PostgreSQL 空库必须完成全部版本化迁移（当前 51 张表）、RLS、注册和登录验收。
- 个人知识库上传、索引、混合问答、跨租户隔离和 3 GiB 配额验收通过。
