# Diary Listener

Diary Listener 是一个本地优先、AI 辅助的个人笔记与回忆工作台。它将随手记录沉淀为可检索的 Markdown 笔记，并基于真实笔记生成日报、周报、月报和带来源引用的回忆回答。

AI 是可选能力：未配置模型或网络不可用时，笔记、标签、附件、搜索、版本历史、导出和备份仍可正常使用。AI 生成的内容必须经过用户预览和确认后才会写入笔记。

## 核心能力

- **快速记录**：在工作台记录文字，直接保存或交给 AI 整理为结构化草稿。
- **Markdown 笔记**：管理普通笔记、日报、周报和月报，支持编辑预览与历史版本恢复。
- **内容组织**：使用标签、附件、日期和笔记类型组织内容，并进行中文关键词检索。
- **周期报告**：从指定周期的笔记生成报告，保存报告与来源笔记之间的关系。
- **回忆书**：用自然语言询问过往经历，回答仅基于当前用户的笔记并保留引用。
- **数据自主**：支持 Markdown ZIP 导出、完整备份，以及向空个人空间受控恢复。
- **个人空间隔离**：每个账号自动拥有唯一的个人空间；后端解析租户，不接受客户端传入 `tenant_id`。

旧版 AI 聊天和图片轻日记接口暂时保留用于兼容，但不再是当前产品的核心入口。

## 技术栈

| 层级 | 技术 |
| --- | --- |
| 前端 | React 18、TypeScript、Webpack 5、Ant Design、TanStack Query、CodeMirror |
| 后端 | Go、Gin、pgx/v5 |
| 数据 | PostgreSQL 16、RLS；附件存储在受控本地目录 |
| AI | LiteLLM Proxy、OpenAI 兼容接口、SSE 流式输出 |
| 测试与格式 | Go test、Vitest、Prettier |
| 部署 | Docker、Docker Compose |

## 项目结构

```text
.
├── backend/
│   ├── cmd/server/          # 服务入口
│   ├── internal/            # Gin API、业务、AI、配置和 pgx 数据层
│   ├── db/                  # Schema 快照与 sqlc 查询
│   ├── scripts/             # 冒烟与真实 AI 验收脚本
│   └── Dockerfile
├── frontend/
│   └── src/
│       ├── api/             # 前端请求封装
│       ├── features/        # 工作台、笔记、报告、回忆、搜索、设置
│       └── routes/          # 路由与登录保护
├── docs/api.md              # API 概览
├── development-standards/   # 工程基线与设计说明
├── docker-compose.yml
└── README.md
```

## 快速启动

### Docker Compose（推荐）

需要 Docker 与 Docker Compose。首先创建环境文件，并为迁移角色和应用角色设置两个不同的强密码：

```powershell
Copy-Item .env.example .env
docker compose up --build
```

首次创建 `db_data` 卷时会依次创建低权限应用角色并应用 `backend/db/schema.sql` 基线。服务地址：

- Web 应用：<http://127.0.0.1:5173>
- 后端 API：<http://127.0.0.1:8000>
- 健康检查：<http://127.0.0.1:8000/healthz>

> 修改 `.env` 中的数据库密码后，如果复用了已有 `db_data` 卷，PostgreSQL 不会自动重建角色密码。请使用与该数据卷初始化时一致的密码，或在确认无需保留数据后重新初始化数据库卷。

### 本地开发

本地开发同样要求 PostgreSQL 16，以及权限分离的 `diary_migrator`、`diary_app` 角色。

后端（PowerShell）：

```powershell
Set-Location backend
$env:DATABASE_URL = "postgresql://diary_app:<app-password>@127.0.0.1:5432/diary_listener"
$env:MIGRATION_DATABASE_URL = "postgresql://diary_migrator:<migrator-password>@127.0.0.1:5432/diary_listener"
go run ./cmd/server
```

前端（另开一个 PowerShell）：

```powershell
Set-Location frontend
npm install
npm run dev
```

Webpack DevServer 默认将 `/api` 和 `/media` 代理到 `http://127.0.0.1:8000`。

## 配置

运行配置只读取服务端环境变量。

常用环境变量：

| 变量 | 说明 |
| --- | --- |
| `DATABASE_URL` | 应用运行时 PostgreSQL 连接，必须使用低权限角色 |
| `MIGRATION_DATABASE_URL` | 管理与调度连接，使用迁移角色 |
| `CORS_ORIGINS` | 逗号分隔的可信前端来源，不允许 `*` |
| `DIARY_DATA_DIR` | 附件、导出、备份和日志的数据根目录 |
| `MAX_ATTACHMENT_BYTES` | 单个附件大小上限，默认 20 MiB |
| `LITELLM_MASTER_KEY` | 本地应用访问 LiteLLM 的网关密钥 |
| `OPENAI_API_KEY` | LiteLLM 使用的 OpenAI 上游密钥 |
| `KIMI_API_KEY` | LiteLLM 使用的 Kimi 上游密钥 |

AI 密钥只保存在服务端。未配置 `AI_API_KEY` 时，AI 接口返回 `AI_NOT_CONFIGURED`，其余本地笔记能力不受影响。

## 数据与安全约定

- PostgreSQL 是笔记正文的唯一权威来源，Markdown 用于交换和导出。
- 全新数据库由 `backend/db/schema.sql` 初始化；后续结构升级必须增加版本化 Go/SQL 迁移。
- 登录 Token 仅以 SHA-256 摘要持久化，并具有有效期和撤销状态。
- 笔记、消息、附件和 AI 用量均绑定个人空间；数据库使用行级安全策略强化隔离。
- 附件通过鉴权接口访问，不作为公开静态目录暴露。
- AI 整理和报告遵循“生成草稿 → 用户确认 → 写入”，回忆回答和报告均保留来源。

## 开发与验证

```powershell
# 后端格式与测试
Set-Location backend
go vet ./...
go test ./...

# 前端格式、测试与生产构建
Set-Location ..\frontend
npm run format:check
npm test
npm run build
```

真实数据库冒烟可执行 `backend/scripts/non_ai_smoke.ps1` 和 `backend/scripts/ai_acceptance.ps1`。生产前端构建输出到 `frontend/dist/`。

## 文档

- [API 概览](docs/api.md)
- [Web 发布验收](docs/web-release-acceptance.md)
- [工程基线](development-standards/BASELINE.md)
- [软件设计说明书](development-standards/SDD.md)
