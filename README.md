# Cortex

[GitHub Actions](.github/workflows/ci.yml) 会在每次 push 和 pull request 时运行后端
`go vet`、测试与构建，前端格式检查、测试与构建，以及 Docker Compose 配置校验。

Cortex 是一个面向个人成长记录的 AI 工作台：用 Markdown 记录日常与周期笔记，并让 AI
在可追溯的个人资料范围内帮助整理、总结、回顾和回答知识问题。

> 当前产品以个人知识库为核心；部署前仍须完成本文列出的 Compose 与验收脚本。

生成模型、Embedding 或 Reranker 不可用时，账号、笔记、标签、附件、搜索、
版本历史和 Markdown 导出仍保持可用；依赖模型的整理、报告、回忆、知识库索引或问答返回明确错误。

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

### 个人知识库

- 上传 Markdown/ZIP、PDF、Word（DOC/DOCX）或图片（PNG/JPG/WebP），可创建知识集合并管理文档；扫描件与图片使用中英文 OCR，每租户容量上限 3 GiB。
- 个人笔记可单独开启参与知识问答，回答只使用当前租户上传资料与已开启笔记作为依据。
- 知识问答使用 512 维中文 GTE Embedding 与 BGE CrossEncoder 精排，回答保存当前租户来源引用。
- `/knowledge` 提供知识问答、历史会话、可折叠来源、质量反馈，以及版本化的检索阶段与耗时展示。
- 弱证据会区分问题歧义、资料范围冲突和确实无依据；前两类允许在 15 分钟内补充一次后恢复原请求。
- 无当前租户有效证据时返回 `KNOWLEDGE_NO_EVIDENCE`，不依赖模型常识生成。
- 比较、趋势和跨周期问题可通过默认关闭的实验开关进行最多 4 个子查询的受控并行召回；简单问题保持单查询快速路径。
- 索引任务持久化展示加载、解析、Embedding、写入和完成/失败阶段以及块级进度；Kafka 模式下解析/OCR、Embedding 和 Elasticsearch 投影由三个独立消费者分阶段驱动，旧活动索引在重建时继续服务。
- `/recipes` 与 `/assistant` 已重定向到 `/knowledge`；历史 HowToCook 语料已迁移到用户 `Diving` 的运行时私有知识库。

PDF、Word 和图片由无业务凭据的隔离 `document-parser` 服务解析为统一文本块。解析结果通过 PostgreSQL 中间结果与 Transactional Outbox 进入独立 Embedding Kafka 阶段，完成后再触发 Elasticsearch 投影；消息仅携带事件和文档标识。Excel 摄取仍不在当前范围。

### 小红书研究

- 在 `/research` 通过关键词或公开笔记链接创建异步研究任务。
- 对公开正文和图片执行受控采集，图片 OCR 可通过 `RESEARCH_OCR_URL` 接入内部服务。
- 通过 LiteLLM 生成摘要、关键观点、分类和标签研究草稿。
- 任务、来源和草稿使用 PostgreSQL 持久化并受 RLS 隔离，不依赖 Redis。
- 支持按个人租户扫码授权；会话使用 AES-256-GCM 加密保存，二维码和 Chromium Profile 仅临时存在。
- 采集授权、平台限流或 AI/OCR 不可用时按能力降级，不影响笔记和个人知识库。

### 数据自主与隔离

- 将有效笔记导出为 Markdown ZIP。
- 每个账号自动对应一个个人空间，租户身份只由服务端根据 Token 解析。
- PostgreSQL RLS 与显式 `tenant_id` 条件共同约束业务查询。
- 附件保存在受控数据目录中，不作为公开静态资源暴露。

### 模板与限量 AI 活动

- 创建私有 Markdown 模板，并由作者自主上架或下架公开快照。
- 从公开模板幂等创建笔记，支持点赞、收藏和使用统计。
- 每天 20:00 开放 10 分钟的免费 AI 点数活动，限 10 个名额，每次赠送 100 点。
- Redis 负责高峰原子预扣，PostgreSQL 保存点数和领取的最终事实。
- AI 活动投影通过版本键离线分批构建并原子切换；可用 `AI_EVENT_PROJECTION_BUILD_BATCH_SIZE`（默认 250）和 `AI_EVENT_PROJECTION_BUILD_LEASE_SECONDS`（默认 60）调节批次与构建租约。
- 模板排行使用版本键双缓冲；Marketplace Outbox 按类型隔离、自动续租并以数据库 fencing 完成。
  daily/匿名 UV 保留 8 天，Redis 使用最大 64 条连接的有界复用池；本地容量验收记录见
  [RAG 链路与 AI 活动负载复验](docs/operations/RAG_AND_K6_RERUN_20260825.md)。

Markdown ZIP 只用于内容交换；生产灾备应覆盖 PostgreSQL 数据库与应用数据卷。笔记版本恢复继续保留。

## 技术架构

| 层级 | 技术 |
| --- | --- |
| 前端 | React 18、TypeScript、Webpack 5、Ant Design、TanStack Query、CodeMirror |
| 后端 | Go、Gin、pgx/v5 |
| 数据 | PostgreSQL 16、pgvector、RLS、Redis 7（活动协调与公共缓存） |
| AI 网关 | LiteLLM、OpenAI 兼容 Chat Completions / Embeddings、SSE |
| RAG | 父子切块、PostgreSQL FTS、向量召回、BGE CrossEncoder Reranker |
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
│   │   ├── application/         # 业务用例、编排及其所需端口
│   │   ├── domain/              # 与传输和存储无关的领域模型
│   │   ├── infrastructure/      # Redis 等外部能力的端口适配器
│   │   ├── knowledge/           # 个人知识库上传、解析与切块
│   │   ├── server/              # HTTP 契约和后台 worker 入口
│   │   └── store/               # PostgreSQL 端口实现、事务与 RLS
│   └── scripts/                 # 数据库初始化与验收脚本
├── frontend/src/
│   ├── api/                     # API 请求封装
│   ├── features/                # 工作台、笔记、知识库、研究、模板等
│   └── routes/                  # 路由保护
├── docs/                        # API、设计、网关与验收文档
├── docker-compose.yml
└── litellm-config.yaml
```

## 快速启动

### 前置条件

- Docker Engine 和 Docker Compose
- 可用的上游生成模型配置
- 首次构建内部 `embedding-service` / `reranker-service` 需要访问模型仓库并预留足够内存和磁盘

### 使用 Docker Compose

知识库向量由 Compose 内部 `embedding-service` 与 `reranker-service` 提供，二者不暴露宿主机
端口。如需在本地用 Ollama 调试 Embedding，可参照 `.env.example` 的 `LOCAL_EMBEDDING_*`
配置；Compose 默认不使用宿主机 Ollama。

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

知识库 Embedding 与 Reranker 是 Compose 的必备内部服务：

```powershell
docker compose up -d --build
```

模型镜像在构建时从 ModelScope 固定 revision 下载
`iic/nlp_gte_sentence-embedding_chinese-small` 与 `BAAI/bge-reranker-v2-m3`，
运行时离线加载。`db`、`llm-gateway` 和两个模型服务都不暴露宿主机端口。

> `.env` 只用于本地部署且已被 Git 忽略。修改数据库密码不会更新已有 `db_data` 卷中的角色密码；复用旧卷时应继续使用初始化该卷时的密码，或在确认数据已备份且不再需要后重新初始化。

## 本地开发

需要 Go、Node.js 20+、PostgreSQL 16 + pgvector，以及权限分离的 `cortex_migrator` 和 `cortex_app` 数据库角色。

启动后端：

```powershell
Set-Location backend
$env:DATABASE_URL = "postgresql://cortex_app:<app-password>@127.0.0.1:5432/cortex"
$env:MIGRATION_DATABASE_URL = "postgresql://cortex_migrator:<migrator-password>@127.0.0.1:5432/cortex"
$env:CORTEX_DATA_DIR = ".\data"
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
| `DATABASE_URL` | 业务连接，必须使用低权限 `cortex_app` |
| `MIGRATION_DATABASE_URL` | 迁移与 scheduler claim 使用的管理连接 |
| `CORTEX_DATA_DIR` | 附件、知识文件和导出的数据根目录；旧 `DIARY_DATA_DIR` 仅作兼容回退 |
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

- 主业务接口使用 `/api/v1`。浏览器使用 HttpOnly 会话 Cookie；脚本通过
  `/api/v1/auth/token` 获取凭证并使用 `Authorization: Token <token>`。
- `/healthz` 只反映进程存活；`/readyz` 只验证数据库可用。
- `/health/dependencies` 返回对象存储、Redis、搜索和 AI 的能力状态；这些可选依赖不参与就绪判定。
- `CORTEX_RUNTIME_ROLE=all|api|worker` 控制进程职责。生产拆分部署时，`api` 不建立 migrator 管理连接，`worker` 不监听 HTTP；Compose 默认使用兼容的 `all`。
- 更新笔记使用乐观冲突保护；正文更新和 AI 覆盖前先创建 revision，删除默认软删除。
- 跨租户资源统一表现为 404；软删除空间不得通过登录或 Token 认证。
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

模板广场 HTTP 容量采样可运行：

```powershell
.\backend\scripts\marketplace_http_capacity.ps1 -Token <token> -Requests 1000 -Concurrency 32
```

脚本输出 QPS、p50/p95/p99 和失败数；生产指标必须在目标部署环境采集。

知识检索 100/1,000/10,000 合成容量测试可运行：

```powershell
.\backend\scripts\knowledge_capacity.ps1
```

联合备份和隔离恢复验证必须使用专用目录；恢复脚本只创建随机命名的隔离资源：

```powershell
.\backend\scripts\backup.ps1 -OutputDirectory .\.tmp-backup-<timestamp>
.\backend\scripts\restore_verify.ps1 -BackupDirectory .\.tmp-backup-<timestamp>
.\backend\scripts\validate_prometheus_rules.ps1
```

实测恢复与容量边界见 `docs/operations/`，发布前逐项检查 `docs/RELEASE_CHECKLIST.md`。备份不包含 `.env`
或供应商 Key；若数据库引用文件缺失、checksum 或双向一致性不通过，恢复会失败退出。
Prometheus 告警规则和 Grafana dashboard 分别位于 `deploy/prometheus/` 与 `deploy/grafana/`。

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
.\backend\scripts\research_acceptance.ps1
.\backend\scripts\template_ai_event_acceptance.ps1
```

知识库验收覆盖上传、索引、混合问答、来源保存与 3 GiB 配额；研究与模板验收覆盖各自流程。

## 文档

- [API 概览](docs/api.md)：认证、笔记、AI、知识库、调度和导出接口
- [工程基线](docs/BASELINE.md)：当前技术与安全基线
- [软件设计说明书](docs/SDD.md)：当前已实现的系统架构、数据、知识库、RAG、AI 工作流和部署设计
- [实现与生产验收待办](docs/IMPLEMENTATION_GAPS.md)：未实现、部分实现、待验证事项和发布阻断
- [大模型网关规范](docs/LLM_GATEWAY.md)：LiteLLM 路由、密钥、隐私和用量治理
- [个人知识库页](docs/page/KNOWLEDGE_PAGE_ARCHITECTURE.md)：上传、配额、文档管理与降级说明
- [模板广场页](docs/page/TEMPLATES_PAGE_ARCHITECTURE.md)：私有模板、公开快照、榜单与使用流程
- [AI 限量活动页](docs/page/AI_EVENTS_PAGE_ARCHITECTURE.md)：活动倒计时、资格、点数与领取
- [小红书研究页](docs/page/RESEARCH_PAGE_ARCHITECTURE.md)：研究采集、整理、保存、授权和验收说明
- [2026-08-25 基础设施验收](docs/operations/INFRASTRUCTURE_ACCEPTANCE_20260825.md)：当前 Compose 主路径、故障注入、备份恢复和可观测性证据
- [2026-08-25 RAG 与负载复验](docs/operations/RAG_AND_K6_RERUN_20260825.md)：当前 RAG 质量和 AI 活动负载结果
- [RAG 与基础设施演进技术方案](docs/INFRASTRUCTURE_EVOLUTION.md)：MinIO/Redis 大文件上传、Kafka 多格式文档处理、Elasticsearch 检索及生产验收方案
