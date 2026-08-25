# Cortex 工程基线

> 状态：当前有效
> 更新日期：2026-08-25

## 产品范围

- 普通笔记、日报、周报和月报。
- 标签、附件、历史版本、中文搜索和 dashboard。
- AI 整理、报告、来源引用和回忆问答。
- 每个账号对应一个服务端解析的个人租户。
- Markdown ZIP 内容导出和笔记版本恢复。
- 用户自主上架的 Markdown 模板广场，以及每日限量免费 AI 点数活动。
- 个人知识库：上传 Markdown / Markdown ZIP、知识集合、个人笔记入库开关、带会话/来源/反馈的混合问答、
  可恢复的一次性澄清和持久化索引阶段进度（每租户 3 GiB 配额）。

桌面组件、团队协作、计费和数据库/Markdown 双向同步不属于当前范围。
PDF、Word、Excel 摄取仍不属于当前知识库摄取范围。

## 技术基线

| 层级 | 当前实践 |
| --- | --- |
| 前端 | React 18、TypeScript、Webpack 5、Ant Design |
| 后端 | Go 1.26、Gin、标准 `net/http`、`gofmt` |
| 数据访问 | pgx/v5、显式 SQL、显式事务 |
| 数据库 | PostgreSQL 16、RLS |
| AI | LiteLLM Proxy、OpenAI 兼容 Chat Completions、SSE |
| 知识检索 | GTE 中文 Embedding（512 维）、BGE CrossEncoder Reranker；PostgreSQL/pgvector + 中文 2-gram 为本地回退，Compose 默认使用 Elasticsearch BM25 + KNN 投影 |
| 部署 | Docker Compose、多阶段静态 Go 镜像 |
| 文件与事件 | Compose 默认使用私有 MinIO 与 Kafka 兼容的 Redpanda；PostgreSQL 保存对象定位、Outbox、任务和消费事实 |
| 活动协调 | Redis 7、Lua 原子预扣；PostgreSQL 保存最终事实 |
| 模板投影 | Outbox 类型隔离、租约续期/完成 fencing、排行版本键双缓冲、Redis 64 连接复用池 |

`backend/cmd/server/main.go` 是唯一部署后端入口，仓库不保留 Python 后端或 Alembic。外部基础设施 worker
由 `backend/internal/workers` 的受管 runner 启动，与 API 共享进程取消和优雅退出边界；迁移与评测命令仅作
显式运维工具，不作为部署服务入口。

## 数据规则

- PostgreSQL 是笔记正文的唯一权威来源。
- `DATABASE_URL` 使用低权限 `cortex_app`。
- `MIGRATION_DATABASE_URL` 使用 `cortex_migrator`，仅供版本化迁移和调度 claim。
- 每次租户业务查询必须在同一个 `pgx.Tx` 内设置 transaction-local RLS 上下文。
- 客户端不得提交或选择可信 `tenant_id`。
- `backend/db/schema.sql` 是经过空库验收的初始化基线。
- 后续 Schema 变化必须新增版本化迁移，不得直接修改已部署数据库。
- `000025_marketplace_hardening` 新增公开模板 trigram GIN 索引；`000026_remove_template_reports`
  删除模板举报表及整条举报链路。

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
- `RAG_PLANNER_ENABLED` 默认关闭；只有冻结评测集证明复杂查询优于单查询基线后才允许启用，
  `RAG_PLANNER_MAX_SUBQUERIES` 不得超过 4。

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
- `db`、`redis`、`llm-gateway`、`embedding-service`、`reranker-service`、`minio`、`kafka`、
  `elasticsearch` 和 `backend` 必须为 healthy；无 HTTP 健康检查的消费者须以进程、积压和任务状态验收。
- 固定 GTE Embedding 必须通过单条、批量、中英文、维度异常和不可用降级验收。
- 新 PostgreSQL 空库必须完成全部版本化迁移（当前 58 张表）、RLS、注册和登录验收。
- 模板广场还需验证 Outbox 类型隔离、排行 active pointer 原子切换、daily/HLL 8 天 TTL、匿名 UV
  读取和 Redis 故障回表降级。本地容量结果不能作为线上 QPS 或 p95/p99。
- 个人知识库上传、索引阶段/进度、混合问答、一次性澄清恢复、幂等回放、跨租户隔离和 3 GiB 配额验收通过。
