# Cortex 执行规范

本文件适用于整个仓库。实现、审查、测试和发布均须遵守；若子目录以后增加更具体的 `AGENTS.md`，以更具体的文件为补充，但不得削弱本文的安全边界。

## 1. 产品与架构边界

- 产品范围是个人笔记、日报/周报/月报、标签、附件、历史版本、中文搜索、dashboard、AI 整理/报告/回忆、Markdown 模板广场、每日限量 AI 深度月报和 Markdown ZIP 导出。
- 桌面组件、团队协作、计费及数据库与 Markdown 双向同步不在当前范围。
- 前端固定为 React 18、TypeScript、Webpack 5、Ant Design；后端固定为 Go、Gin、pgx/v5；数据库固定为 PostgreSQL 16；AI 经 LiteLLM 的 OpenAI 兼容 SSE 接口访问。
- `backend/cmd/server/main.go` 是唯一后端入口。不得重新引入 Python 后端、FastAPI、SQLAlchemy 或 Alembic。
- PostgreSQL 是笔记正文的唯一权威来源，Markdown 仅用于交换与导出。

## 2. 后端实现规则

- Go 源码遵循 `gofmt` 的标准格式；允许使用 Go 惯例中的 Tab 缩进。
- handler 只处理 HTTP 契约；SQL 和事务逻辑放在 `backend/internal/store`。
- 业务接口统一使用 `/api/v1`；不得重新引入无版本前缀的旧认证、聊天或轻日记路径。
- 统一返回稳定的 `code`、`message` 和可选 `details`；不得把内部地址、上游响应正文、密钥或完整日记正文返回客户端或写入普通日志。
- 密码使用 PBKDF2-SHA256；登录 Token 仅持久化 SHA-256 摘要，并支持过期、撤销和最后使用时间。
- 更新使用版本号或更新时间进行乐观冲突保护；笔记正文更新和 AI 覆盖前必须创建 revision；删除默认软删除。

## 3. 租户、数据库与附件安全

- 每个账号对应一个由服务端解析的个人租户。客户端提交的 `tenant_id` 不可信，不得用于选择租户。
- 租户业务查询必须通过 `Store.WithTx`，并在同一个 `pgx.Tx` 中设置 transaction-local RLS 用户与租户上下文，同时保留显式 `tenant_id` 条件。
- `DATABASE_URL` 必须使用低权限 `cortex_app`；`MIGRATION_DATABASE_URL` 仅供迁移和 scheduler claim，使用 `cortex_migrator`。
- 跨租户资源访问统一表现为 404；软删除租户不得通过登录或 Token 认证。
- `backend/db/schema.sql` 是新实例初始化基线。已部署结构的变化必须新增版本化迁移，使用 advisory lock；不得用应用启动时的临时 DDL 代替迁移。
- 附件只保存 `CORTEX_DATA_DIR` 下的安全相对路径，上传须校验大小和配额，下载/删除须认证并阻止目录穿越。附件不得作为公开静态目录暴露；`DIARY_DATA_DIR` 仅作旧部署升级兼容。
- 周报日期归一到周一，月报日期归一到月初；周期笔记按租户、类型和周期日期唯一。

## 4. AI 与网关边界

- AI 是可选能力；AI 未配置或不可用时，笔记、搜索、附件和导出必须保持可用。
- `AIClient` 只负责模型流，`Retriever` 只在可信 Principal/RLS 下检索，`AIWorkflow` 负责编排；Prompt、确认、引用校验、配额、审计和 RLS 保留在业务层。
- 整理与报告必须先生成草稿、后由确认接口写入；报告和成长助手回答必须校验并保存当前租户的来源。无来源报告返回 `REPORT_NO_SOURCES`，成长助手无依据时返回 `KNOWLEDGE_NO_EVIDENCE`。
- 后端仅持有 LiteLLM 虚拟密钥并使用逻辑模型 `cortex-default`；供应商真实 Key 只注入 LiteLLM，不得进入前端、URL、Cookie、日志、审计、数据库业务字段、备份或文档。
- 不得绕过网关直连供应商。流式响应已经输出内容后不得从头重试。缓存默认关闭，禁止跨租户共享 Prompt/响应缓存。
- 发送到网关的观测元数据只能包含后端生成的非直接身份标识、请求类型、环境和请求追踪 ID；不得包含邮箱、姓名或完整正文。

## 5. Scheduler、交换与部署

- scheduler 使用管理连接池，以 `FOR UPDATE SKIP LOCKED` 和有限租约 claim 到期任务；运行状态持久化为 running/success/failed，手动重试立即返回 queued。
- 多实例同时争抢同一任务只能产生一条 run；时间按任务 IANA 时区计算，数据库保存 UTC。
- Compose 下 `db` 与 `llm-gateway` 不暴露宿主机端口；后端运行时须降权，数据卷在容器重建时必须保留。
- `/healthz` 只反映进程存活且不依赖 AI；`/readyz` 验证数据库可用。

## 6. 变更与验证要求

- 修改前先检查工作树，保留用户已有改动；不得修改无关文件或用破坏性 Git 操作清理工作区。
- 新功能或缺陷修复必须增加与风险相称的单元/集成测试，并同步相关 README、API 或设计文档。
- 后端提交前至少运行：

```powershell
Set-Location backend
go vet ./...
go test ./...
go build ./cmd/server
```

- 前端变更至少运行：

```powershell
Set-Location frontend
npm run format:check
npm test
npm run build
```

- 部署变更至少运行 `docker compose config --quiet`。数据库、AI 或跨服务变更还应按条件运行：

```powershell
.\backend\scripts\non_ai_smoke.ps1
.\backend\scripts\ai_acceptance.ps1
```

- 发布前确认 Go 源码已经过 `gofmt`，并确认 Compose 的 `db`、`redis`、`llm-gateway`、`backend` healthy。新 PostgreSQL 空库须通过 56 张表、RLS、ready、注册和登录验收。
