# Diary Listener Go 后端与部署规范

> 状态：当前有效
> 更新日期：2026-07-26

## 1. 当前结论

Diary Listener 后端已经完成 Go 迁移。`backend/` 是唯一后端实现，使用 Gin、pgx/v5、PostgreSQL RLS 和 LiteLLM Proxy。Python、FastAPI、SQLAlchemy 和 Alembic 已从运行路径及仓库中删除。

当前固定选型：

| 能力 | 选型 |
| --- | --- |
| HTTP | Gin + 标准 `net/http` |
| PostgreSQL | pgx/v5、显式 SQL和事务 |
| 查询组织 | `backend/db/queries`，逐步由 sqlc 生成类型安全代码 |
| Schema 初始化 | `backend/db/schema.sql` |
| AI 客户端 | 项目自有接口 + OpenAI 兼容 SSE adapter |
| AI 网关 | LiteLLM Proxy `main-v1.77.7-stable`，锁定 OCI digest |
| 部署 | Docker Compose、多阶段 Alpine 静态镜像 |

## 2. 代码结构

```text
backend/
├── cmd/server/             # 进程入口
├── internal/
│   ├── ai/                 # AIClient、Workflow、SSE adapter
│   ├── apierror/           # 稳定业务错误
│   ├── auth/               # PBKDF2、Token
│   ├── config/             # 环境变量配置
│   ├── domain/             # Principal 等领域类型
│   ├── httpx/              # JSON 与错误响应
│   ├── server/             # Gin 路由和 HTTP handler
│   └── store/              # pgx SQL、RLS 事务
├── db/
│   ├── schema.sql          # 新实例完整基线
│   └── queries/            # sqlc 查询
├── scripts/                # 冒烟和 AI 验收
├── Dockerfile
└── docker-entrypoint.sh
```

Go 文件统一使用 4 空格，提交前必须检查行首 Tab 为 0。

## 3. HTTP 与错误契约

- 新业务接口使用 `/api/v1`。
- 旧聊天、认证和轻日记兼容路径按现有前端契约保留。
- Gin 负责路由、分组、中间件、CORS 和 panic recovery。
- handler 不直接信任租户字段；认证中间件生成 `domain.Principal`。
- 错误统一返回稳定的 `code`、`message` 和可选 `details`。
- `/healthz` 只反映进程存活；`/readyz` 验证数据库可用。

## 4. 数据与 RLS

- 应用连接使用 `diary_app`，管理连接使用 `diary_migrator`。
- 租户业务必须调用 `Store.WithTx`，并在同一事务执行 `set_config(..., true)`。
- 不能先设置租户上下文再从连接池取得另一连接。
- 附件路径只保存 `DIARY_DATA_DIR` 下的安全相对路径。
- 周报按周一、月报按月初归一，周期笔记保持唯一。
- 更新使用版本号或更新时间进行乐观冲突保护。
- 删除租户和笔记默认软删除，恢复行为必须写审计。

新数据库由以下顺序初始化：

1. `backend/scripts/init-db-roles.sh`
2. `backend/db/schema.sql`

基线已经在独立 PostgreSQL 16 卷验证：18 张表、15 条 RLS Policy、Go ready、注册与登录通过。

后续必须补充版本化增量迁移命令和 advisory lock；禁止用启动时的临时 DDL 替代迁移。

## 5. AI 边界

业务层保留项目自有接口：

```go
type AIClient interface {
    StreamChat(context.Context, ChatRequest) (<-chan StreamEvent, error)
}

type Retriever interface {
    Retrieve(context.Context, domain.Principal, string, int) ([]SourceNote, error)
}

type AIWorkflow interface {
    Organize(context.Context, string, string) (<-chan StreamEvent, error)
    GenerateReport(context.Context, string) (<-chan StreamEvent, error)
    AnswerMemory(context.Context, string) (<-chan StreamEvent, error)
}
```

Prompt、引用校验、确认、配额、审计和 RLS 属于 Diary Listener 领域逻辑，不得下沉到 LiteLLM 或未来 AI 框架。

当前不引入 Eino/LangChainGo。只有出现复杂工具调用、图式流程、长期状态或多路 RAG 时才单独做 PoC，并保留现有 OpenAI 兼容 adapter 作为回滚路径。

## 6. LiteLLM

- 应用固定使用 `AI_BASE_URL=http://llm-gateway:4000/v1`。
- 应用固定使用逻辑模型 `diary-default`。
- Kimi `kimi-k2.5` 为主路由。
- OpenAI `gpt-5.6` 为 fallback；当前本地 OpenAI 账户额度不足。
- 缓存关闭，网关不公开宿主机端口。
- 供应商 Key 只存在于 LiteLLM 容器环境。
- 本地透明代理可使用 master key；生产必须签发受预算和模型限制的虚拟密钥。

## 7. Scheduler

- due task 使用管理池 claim。
- claim 使用 `FOR UPDATE SKIP LOCKED`。
- claim 设置有限租约，执行结束写入真实 `next_run_at`。
- 手动重试立即返回 `queued`，运行状态写入 `scheduled_report_runs`。
- 多实例验收必须保证同一到期任务只生成一条 run。

## 8. 部署

Compose 服务：

```text
frontend -> backend -> db
                   `-> llm-gateway -> Kimi / OpenAI
```

- `backend` 对宿主机暴露 `127.0.0.1:8000`。
- `frontend` 暴露 `127.0.0.1:5173`。
- `db` 和 `llm-gateway` 不公开宿主机端口。
- Go 容器启动时修正共享数据目录权限，随后降权为 `diary` 用户。
- 所有依赖通过 healthcheck 后再启动下游服务。
- 删除或重建容器不得删除 `db_data` 和 `app_data` 卷。

## 9. 验收清单

- [x] Gin、pgx/v5、RLS 和租户隔离。
- [x] 认证、租户、笔记、版本、标签、搜索和 dashboard。
- [x] 附件、导出、备份和恢复。
- [x] AI 整理、报告、回忆、引用和 SSE。
- [x] 旧聊天与轻日记兼容接口。
- [x] scheduler 手动、失败和多实例 claim。
- [x] LiteLLM 主路由和真实 Kimi 生成。
- [x] 新 PostgreSQL 空库初始化。
- [x] Python 后端删除及 `backend/` 目录切换。
- [ ] 版本化增量迁移命令、advisory lock 和回滚。
- [ ] 正式 LiteLLM 虚拟密钥、预算和指标。
- [ ] 可选 Helm 与生产发布流程。
