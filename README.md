# Cortex

[GitHub Actions](.github/workflows/ci.yml) 会在每次 push 和 pull request 时运行后端
`go vet`、测试与构建，前端格式检查、测试与构建，以及 Docker Compose 配置校验。

Cortex 是一个面向个人成长记录的 AI 工作台：用 Markdown 记录日常与周期笔记，并让 AI
在可追溯的个人资料范围内帮助整理、总结、回顾和回答烹饪问题。

> 当前版本正在进行“今日菜谱”生产验收；部署前仍须完成本文列出的 Compose 与验收脚本。

生成模型、Embedding 或 Reranker 不可用时，账号、笔记、标签、附件、搜索、
版本历史和 Markdown 导出仍保持可用；依赖模型的整理、报告、回忆、菜谱索引或问答返回明确错误。

## 能做什么

### 记录与整理

- 创建普通笔记、日报、周报和月报，支持 Markdown 编辑与预览。
- 使用标签、日期、笔记类型和附件组织内容，进行中文关键词搜索。
- 在 Dashboard 查看记录统计、活跃趋势、最近笔记和待处理周期报告。
- 每次正文更新保留历史版本，可查看并恢复指定 revision。
- 快速记录可以先交给 AI 整理为草稿，确认后才写入笔记。

### 报告与回忆

- 从指定周期内的笔记生成日报、周报或月报草稿。
- 为报告保存来源笔记，避免无来源生成；无来源时直接拒绝。
- 配置按 IANA 时区运行的周期报告任务，并查看运行记录或手动重试。
- 通过回忆问答检索自己的历史笔记，回答保留可追踪引用。

### 今日菜谱

- 从仓库内固定 revision 的 HowToCook 语料为每位用户确定性挑选每日菜谱。
- 保存跨设备忌口与时区，推荐不会命中规范化后的忌口词项。
- 首页提供三个与当日菜品相关的问题，也可输入任意烹饪问题。
- 菜谱问答使用 512 维中文 GTE Embedding 与 BGE CrossEncoder 精排，回答保存系统菜谱引用。
- 知识库仅由仓库内 `resources/howtocook` 静态语料构成，来源是 Github 上的一个高 star 项目，具体可以进入资源库查看。

### 小红书研究

- 在 `/research` 通过关键词或公开笔记链接创建异步研究任务。
- 对公开正文和图片执行受控采集，图片 OCR 可通过 `RESEARCH_OCR_URL` 接入内部服务。
- 通过 LiteLLM 生成摘要、关键观点、分类和标签研究草稿。
- 任务、来源和草稿使用 PostgreSQL 持久化并受 RLS 隔离，不依赖 Redis。
- 支持按个人租户扫码授权；会话使用 AES-256-GCM 加密保存，二维码和 Chromium Profile 仅临时存在。
- 采集授权、平台限流或 AI/OCR 不可用时按能力降级，不影响笔记和静态菜谱知识库。

### 数据自主与隔离

- 将有效笔记导出为 Markdown ZIP，或导出包含私有文件和资源关系的完整备份 ZIP。
- 每个账号自动对应一个个人空间，租户身份只由服务端根据 Token 解析。
- PostgreSQL RLS 与显式 `tenant_id` 条件共同约束业务查询。
- 附件保存在受控数据目录中，不作为公开静态资源暴露。

### 模板与限量 AI 活动

- 创建私有 Markdown 模板，并由作者自主上架或下架公开快照。
- 从公开模板幂等创建笔记，支持点赞、收藏、使用统计和举报反馈。
- 每天 20:00 开放 10 分钟的 AI 深度月报活动，限 10 个名额，固定消耗 100 点。
- Redis 负责高峰原子预扣，PostgreSQL 保存点数、领取和生成任务的最终事实。

Markdown ZIP 只用于内容交换。版本化完整备份可恢复到空的个人空间并重映射资源 ID，且排除
Token、密钥、小红书授权会话和敏感审计；生产灾备仍应覆盖 PostgreSQL 数据库与应用数据卷。
个人空间软删除恢复和笔记版本恢复继续保留。

## 技术架构

| 层级 | 技术 |
| --- | --- |
| 前端 | React 18、TypeScript、Webpack 5、Ant Design、TanStack Query、CodeMirror |
| 后端 | Go、Gin、pgx/v5 |
| 数据 | PostgreSQL 16、pgvector、RLS、Redis 7（活动协调与公共缓存） |
| AI 网关 | LiteLLM、OpenAI 兼容 Chat Completions / Embeddings、SSE |
| RAG | 父子切块、PostgreSQL FTS、向量召回、Qwen3 Reranker |
| 部署 | Docker Compose |

```text
Browser
   │
   ├── React UI ────────────────┐
   │                            │
   └────────────────────── Go / Gin API
                                ├── PostgreSQL + pgvector
                                ├── 受控文件存储
                                ├── LiteLLM ── 生成 / Embedding 模型
                                └── Reranker 服务（可选本地部署）
```

后端唯一入口是 `backend/cmd/server/main.go`。PostgreSQL 是笔记正文的唯一权威来源，Markdown 只用于交换与导出。

## 项目结构

```text
.
├── backend/
│   ├── cmd/
│   │   ├── server/              # 唯一后端服务入口
│   │   └── migrate/             # 显式数据库迁移工具
│   ├── db/schema.sql            # 新实例初始化基线
│   ├── internal/
│   │   ├── ai/                  # 生成、Embedding 与 Rerank 客户端
│   │   ├── recipe/              # HowToCook 同步、索引与检索
│   │   ├── server/              # HTTP 契约和后台 worker
│   │   └── store/               # SQL、事务与 RLS 数据访问
│   └── scripts/                 # 数据库初始化与验收脚本
├── frontend/src/
│   ├── api/                     # API 请求封装
│   ├── features/                # 工作台、笔记、菜谱、研究、报告等
│   └── routes/                  # 路由保护
├── docs/                        # API、设计、网关与验收文档
├── docker-compose.yml
└── litellm-config.yaml
```

## 快速启动

### 前置条件

- Docker Engine 和 Docker Compose
- 可用的上游生成模型配置
- 宿主机安装并启动 Ollama；知识向量统一使用本地 `qwen3-embedding:0.6b`
- 如启用可选的本地 Qwen3 Reranker，首次构建需要访问官方模型仓库并预留足够内存和磁盘

### 使用 Docker Compose

先在宿主机启动 Ollama 并拉取 0.6B Embedding 模型。Windows 上需让
Docker Desktop 能访问 Ollama 监听端口：

在第一个 PowerShell 窗口运行：

```powershell
$env:OLLAMA_HOST = "0.0.0.0:11434"
ollama serve
```

保持该进程运行，再在第二个 PowerShell 窗口执行：

```powershell
ollama pull qwen3-embedding:0.6b
```

`ollama serve` 需要保持运行。通过 Windows 防火墙将 `11434` 限制为本机和
Docker 私有网络可访问，不要将它暴露到公网。

然后复制环境变量模板，并替换其中所有 `replace-with-...` 占位值。数据库应用角色、迁移角色和 LiteLLM 数据库角色必须使用不同的强密码；生成模型供应商 Key 只提供给 LiteLLM。

```powershell
Copy-Item .env.example .env
docker compose config --quiet
docker compose up -d --build
docker compose ps
```

默认地址：

- Web：<http://127.0.0.1:5173>
- API：<http://127.0.0.1:8000>
- 存活检查：<http://127.0.0.1:8000/healthz>
- 就绪检查：<http://127.0.0.1:8000/readyz>

`db` 和 `llm-gateway` 不暴露宿主机端口。后端只监听本机映射的 `8000`，前端监听 `5173`。

菜谱 Embedding 与 Reranker 是 Compose 的必备内部服务：

```powershell
docker compose up -d --build
```

模型镜像在构建时从 ModelScope 固定 revision 下载
`iic/nlp_gte_sentence-embedding_chinese-small` 与 `BAAI/bge-reranker-v2-m3`，
运行时离线加载。`db`、`llm-gateway` 和两个模型服务都不暴露宿主机端口。

> `.env` 只用于本地部署且已被 Git 忽略。修改数据库密码不会更新已有 `db_data` 卷中的角色密码；复用旧卷时应继续使用初始化该卷时的密码，或在确认数据已备份且不再需要后重新初始化。

## 本地开发

需要 Go、Node.js 20+、PostgreSQL 16 + pgvector，以及权限分离的 `diary_migrator` 和 `diary_app` 数据库角色。

启动后端：

```powershell
Set-Location backend
$env:DATABASE_URL = "postgresql://diary_app:<app-password>@127.0.0.1:5432/diary_listener"
$env:MIGRATION_DATABASE_URL = "postgresql://diary_migrator:<migrator-password>@127.0.0.1:5432/diary_listener"
$env:DIARY_DATA_DIR = ".\data"
go run ./cmd/server
```

启动前端：

```powershell
Set-Location frontend
npm install
npm run dev
```

Webpack DevServer 会将 `/api` 和 `/media` 代理到 `http://127.0.0.1:8000`。

## 关键配置

完整示例见 [.env.example](.env.example)。

| 变量 | 用途 |
| --- | --- |
| `DATABASE_URL` | 业务连接，必须使用低权限 `diary_app` |
| `MIGRATION_DATABASE_URL` | 迁移与 scheduler claim 使用的管理连接 |
| `DIARY_DATA_DIR` | 附件、导出和日志的数据根目录 |
| `MAX_ATTACHMENT_BYTES` | 单附件上限，默认 20 MiB |
| `RAG_EMBEDDING_*` | Embedding 地址、凭据、逻辑模型和维度 |
| `RAG_RERANK_*` | Reranker 地址和模型 |
| `AI_BASE_URL` / `AI_API_KEY` / `AI_MODEL` | 后端访问 LiteLLM 的生成配置 |
| `LITELLM_MASTER_KEY` | LiteLLM 管理密钥，不注入业务前端 |
| `LITELLM_VIRTUAL_KEY` | 后端使用的限模型、限预算虚拟密钥 |
| `LOCAL_EMBEDDING_BASE_URL` | LiteLLM 访问宿主机 Ollama 的 OpenAI 兼容地址 |
| `LOCAL_EMBEDDING_API_KEY` | 本地接口占位凭据，不产生 API 费用 |
| `OPENAI_API_KEY` / `KIMI_API_KEY` / `DEEPSEEK_API_KEY` | 仅由 LiteLLM 使用的生成模型供应商凭据 |

生产环境不得把真实 Key 写入仓库、URL、Cookie、普通日志、审计业务字段或导出文件。

## API 与安全约定

- 主业务接口使用 `/api/v1`，认证头为 `Authorization: Token <token>`。
- `/healthz` 只反映进程存活；`/readyz` 只验证数据库可用。
- 更新笔记使用乐观冲突保护；正文更新和 AI 覆盖前先创建 revision，删除默认软删除。
- 跨租户资源统一表现为 404；软删除空间的普通业务请求返回 403。
- 周报日期归一到周一，月报日期归一到月初。
- AI 整理与报告遵循“生成草稿 → 用户确认 → 写入”，报告、回忆与知识问答保留来源。

接口列表、请求约定和错误语义见 [API 概览](docs/api.md)。

## 数据库升级

新数据库由 `backend/db/schema.sql` 初始化。已部署实例必须通过版本化迁移升级，不能依赖应用启动时临时修改 Schema。

```powershell
Set-Location backend
go run ./cmd/migrate status
go run ./cmd/migrate up
```

迁移工具使用 PostgreSQL advisory lock，并支持 `up`、`down` 和 `status`。Docker Compose 后端入口会在启动服务进程前执行待处理迁移。

## 开发验证

后端：

```powershell
Set-Location backend
gofmt -l .
go vet ./...
go test ./...
go build ./cmd/server
go build ./cmd/migrate
```

前端：

```powershell
Set-Location frontend
npm run format:check
npm test
npm run build
```

部署与端到端验收：

```powershell
docker compose config --quiet
.\backend\scripts\non_ai_smoke.ps1
.\backend\scripts\ai_acceptance.ps1
docker compose --profile local-ai up -d --build reranker-service
.\backend\scripts\recipe_sync_acceptance.ps1
.\backend\scripts\template_ai_event_acceptance.ps1
```

菜谱验收脚本验证 HowToCook 静态语料 revision、推荐稳定性、建议问题和问答来源。

## 文档

- [API 概览](docs/api.md)：认证、笔记、AI、知识库、调度和导出接口
- [工程基线](docs/BASELINE.md)：当前技术与安全基线
- [软件设计说明书](docs/SDD.md)：当前已实现的系统架构、数据、知识库、RAG、AI 工作流和部署设计
- [实现与生产验收待办](docs/IMPLEMENTATION_GAPS.md)：未实现、部分实现、待验证事项和发布阻断
- [大模型网关规范](docs/LLM_GATEWAY.md)：LiteLLM 路由、密钥、隐私和用量治理
- [小红书授权架构](docs/XHS_AUTHORIZATION_ARCHITECTURE.md)：功能页、扫码授权、会话加密、租户隔离与 API 数据流
- [研究页面架构](docs/page/RESEARCH_PAGE_ARCHITECTURE.md)：研究采集、整理、保存、授权和验收说明
