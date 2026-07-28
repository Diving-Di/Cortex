# Cortex

Cortex 是一个面向个人成长记录的 AI 知识工作台：用 Markdown 记录日常与周期笔记，沉淀自己的知识文件，并让 AI 在可追溯的个人资料范围内帮助整理、总结和回顾。

> 当前状态：核心笔记功能与知识库/RAG MVP 已实现，但知识库链路尚未通过完整生产验收，**当前版本禁止直接发布到生产环境**。已知阻断项和复验要求见 [实现与生产验收待办](docs/IMPLEMENTATION_GAPS.md)。

AI 是可选能力。生成模型、Embedding 或 Reranker 不可用时，账号、笔记、标签、附件、搜索、版本历史、知识文件管理和 Markdown 导出仍应保持可用；依赖模型的整理、报告、回忆、索引或问答会返回明确错误。

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

### 个人知识库与成长助手

- 创建知识集合，上传 UTF-8 TXT、文本型 PDF 和 DOCX 文件。
- 异步执行内容提取、父子切块、向量索引和全文索引，并展示处理状态。
- 按集合、文件名和处理状态筛选文件，支持服务端分页、提取预览、鉴权下载和重新索引。
- 在集合或指定文件范围内进行向量与 PostgreSQL 全文混合召回，再经 Reranker 重排。
- 成长助手支持独立会话以及知识库、成长记录、全部来源三种范围，回答使用安全 Markdown
  和可追踪引用；没有足够证据时返回 `KNOWLEDGE_NO_EVIDENCE`。
- 原文件只能通过鉴权接口下载；删除后文件与索引立即对当前用户失效。

### 小红书研究

- 在 `/research` 通过关键词或公开笔记链接创建异步研究任务。
- 对公开正文和图片执行受控采集，图片 OCR 可通过 `RESEARCH_OCR_URL` 接入内部服务。
- 通过 LiteLLM 生成摘要、关键观点、分类和标签草稿，用户确认后才写入个人知识库。
- 任务、来源和草稿使用 PostgreSQL 持久化并受 RLS 隔离，不依赖 Redis。
- 支持按个人租户扫码授权；会话使用 AES-256-GCM 加密保存，二维码和 Chromium Profile 仅临时存在。
- 采集授权、平台限流或 AI/OCR 不可用时按能力降级，不影响笔记和知识库管理。

### 数据自主与隔离

- 将有效笔记导出为 Markdown ZIP，用于内容交换或迁移。
- 每个账号自动对应一个个人空间，租户身份只由服务端根据 Token 解析。
- PostgreSQL RLS 与显式 `tenant_id` 条件共同约束业务查询。
- 附件和知识原文件保存在受控数据目录中，不作为公开静态资源暴露。

Markdown ZIP 不是完整备份。Cortex 当前不提供应用级完整备份/恢复 API；生产灾备应覆盖 PostgreSQL 数据库与应用数据卷。个人空间软删除恢复和笔记版本恢复仍属于产品功能。

## 技术架构

| 层级 | 技术 |
| --- | --- |
| 前端 | React 18、TypeScript、Webpack 5、Ant Design、TanStack Query、CodeMirror |
| 后端 | Go、Gin、pgx/v5 |
| 数据 | PostgreSQL 16、pgvector、RLS |
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
│   │   ├── knowledge/           # 文档提取、切块与索引
│   │   ├── server/              # HTTP 契约和后台 worker
│   │   └── store/               # SQL、事务与 RLS 数据访问
│   └── scripts/                 # 数据库初始化与验收脚本
├── frontend/src/
│   ├── api/                     # API 请求封装
│   ├── features/                # 工作台、笔记、知识库、助手、报告等
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

可选的本地 `Qwen/Qwen3-Reranker-0.6B` 服务位于 `local-ai` Profile：

```powershell
docker compose --profile local-ai up -d --build
```

Backend 始终通过 LiteLLM 的 `cortex-embedding` 逻辑模型调用宿主机 Ollama
中的 `qwen3-embedding:0.6b`，不直接访问 Ollama。该模型固定返回 1024
维向量，与当前 pgvector Schema 一致。`local-ai` Profile 只启动可选
Reranker；模型在镜像构建时从 Qwen 官方 Hugging Face 仓库的固定 revision
下载，运行时离线加载，不使用 TEI；Embedding 模型不运行在 Docker 中。

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
| `DIARY_DATA_DIR` | 附件、知识原文件、导出和日志的数据根目录 |
| `MAX_ATTACHMENT_BYTES` | 单附件上限，默认 20 MiB |
| `KNOWLEDGE_MAX_FILE_BYTES` | 单知识文件上限，默认 50 MiB |
| `KNOWLEDGE_MAX_PDF_PAGES` | PDF 页数上限，默认 500 |
| `KNOWLEDGE_MAX_EXTRACTED_CHARS` | 单文件提取字符上限，默认 5,000,000 |
| `RAG_INDEX_WORKERS` | 知识索引 worker 数，默认 2 |
| `RAG_EMBEDDING_*` | Embedding 地址、凭据、逻辑模型和维度 |
| `RAG_RERANK_*` | Reranker 地址和模型 |
| `AI_BASE_URL` / `AI_API_KEY` / `AI_MODEL` | 后端访问 LiteLLM 的生成配置 |
| `LITELLM_MASTER_KEY` | LiteLLM 管理密钥，不注入业务前端 |
| `LITELLM_VIRTUAL_KEY` | 后端使用的限模型、限预算虚拟密钥 |
| `LOCAL_EMBEDDING_BASE_URL` | LiteLLM 访问宿主机 Ollama 的 OpenAI 兼容地址 |
| `LOCAL_EMBEDDING_API_KEY` | 本地接口占位凭据，不产生 API 费用 |
| `OPENAI_API_KEY` / `KIMI_API_KEY` | 仅由 LiteLLM 使用的生成模型供应商凭据 |

生产环境不得把真实 Key 写入仓库、URL、Cookie、普通日志、审计业务字段或导出文件。

## API 与安全约定

- 主业务接口使用 `/api/v1`，认证头为 `Authorization: Token <token>`。
- `/healthz` 只反映进程存活；`/readyz` 验证数据库可用，不依赖 AI。
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
.\backend\scripts\knowledge_acceptance.ps1
```

知识验收脚本使用固定合成 TXT/PDF/DOCX 与双租户数据，对跨租户 404、删除后不召回、
答案事实和租户泄漏进行断言，并硬性检查 Recall@8、MRR、nDCG、citation precision、
无答案准确率以及检索/端到端 P95 门槛。

## 文档

- [API 概览](docs/api.md)：认证、笔记、AI、知识库、调度和导出接口
- [工程基线](docs/BASELINE.md)：当前技术与安全基线
- [软件设计说明书](docs/SDD.md)：当前已实现的系统架构、数据、知识库、RAG、AI 工作流和部署设计
- [实现与生产验收待办](docs/IMPLEMENTATION_GAPS.md)：未实现、部分实现、待验证事项和发布阻断
- [大模型网关规范](docs/LLM_GATEWAY.md)：LiteLLM 路由、密钥、隐私和用量治理
- [小红书授权架构](docs/XHS_AUTHORIZATION_ARCHITECTURE.md)：功能页、扫码授权、会话加密、租户隔离与 API 数据流
- [研究页面架构](docs/page/RESEARCH_PAGE_ARCHITECTURE.md)：研究采集、整理、保存、授权和验收说明
