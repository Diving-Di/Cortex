# Cortex 文档索引

> 更新日期：2026-08-28
> 维护原则：代码、数据库迁移与部署配置是实现事实；`AGENTS.md` 是不可削弱的工程约束；历史报告保留当时结论，不自动代表当前版本或生产环境。

## 当前基线与契约

| 文档 | 用途 |
|---|---|
| [工程基线](BASELINE.md) | 当前产品、技术、数据与验证基线；发现冲突时优先核对实现与仓库规范 |
| [软件设计说明书](SDD.md) | 当前系统边界、数据流、安全与主要组件 |
| [API 概览](api.md) | 当前 `/api/v1` HTTP/SSE 契约 |
| [RAG 链路](RAG.md) | 当前双检索 backend、证据门控与已冻结质量基线 |
| [LLM 网关](LLM_GATEWAY.md) | LiteLLM 接入、安全、缓存与可靠性规范 |
| [发布检查清单](RELEASE_CHECKLIST.md) | 发布门禁的唯一汇总入口 |
| [生产 SLO 与值班契约](SLO.md) | API、数据库与灾备目标，错误预算和告警升级责任 |
| [未完成事项](IMPLEMENTATION_GAPS.md) | 已确认的架构偏差、生产风险与不可承诺事项 |

## 架构演进与运维

| 文档/目录 | 用途 |
|---|---|
| [基础设施演进](INFRASTRUCTURE_EVOLUTION.md) | MinIO、Kafka/Redpanda、Elasticsearch 的当前落地状态与后续生产收敛 |
| [外部基础设施运维](EXTERNAL_INFRA_OPERATIONS.md) | 分阶段切换、故障与恢复要求 |
| [运行手册](runbooks/) | 常见基础设施与 RAG 故障处置 |
| [页面架构](page/README.md) | 已实现前端页面的专项说明与覆盖索引 |

## 历史证据

- `operations/` 保存带日期的容量、故障、恢复与验收记录。结论只适用于文档注明的 commit、配置和环境；涉及检索 backend 或基础设施变更时必须重新验收。
- 当前本地目标环境证据以 `operations/INFRASTRUCTURE_ACCEPTANCE_20260825.md` 和
  `operations/RAG_AND_K6_RERUN_20260825.md` 为准；`RAG_AND_PRODUCTION_ACCEPTANCE_20260824.md`
  仅作为成本、监控缺口和质量变化的历史对照。
- `rag-baselines/` 保存 RAG 历史冻结基线，用于回归对照，不表示当前 Compose 默认检索路径已经达到相同指标。

## 规划文档的使用边界

规划内容必须标明“目标/待实现”，不得覆盖当前实现描述。当前已知的重要分界包括：

- Compose 已默认启用 MinIO、Kafka 兼容的 Redpanda 和 Elasticsearch，但目标环境生产门禁尚需独立完成。
- Elasticsearch 当前实现同时执行 BM25 与 KNN；PostgreSQL/pgvector + 中文 2-gram 是独立配置回退 backend。
- `backend/cmd/server/main.go` 已是唯一部署后端入口；Outbox relay、知识索引、搜索投影和对象 GC
  均由 `backend/internal/workers` 在 server 进程内托管。`cmd/migrate`、`cmd/blob-migrate` 和评测命令
  仅用于显式运维或离线验证。
- PDF、DOC/DOCX 与 PNG/JPG/WebP OCR 摄取已经接入隔离的 `document-parser`；Excel、演示文稿和
  断点续传仍属于规划能力，不能写入当前 API 或页面能力清单。
- 当前数据库迁移版本为 41；迁移 41 把知识摄取拆成解析、Embedding、搜索投影三个 Kafka 阶段，
  不新增 public 表。历史报告中的迁移版本和表数只描述报告当时环境。

## 修改文档时的核对顺序

1. 先检查工作树，保留用户已有改动与历史证据。
2. 用 `backend/internal/server/server.go` 核对路由，用 `backend/internal/config` 与 Compose 核对运行配置。
3. 用 `backend/db/schema.sql` 和 `backend/internal/migrations/sql` 核对数据结构与迁移状态。
4. 当前实现变化时同步更新 `BASELINE.md`、`SDD.md`、`api.md` 和相关页面文档。
5. 仅当内容能被实现、迁移或验收证据明确证伪时删除；否则保留并补充适用时间、环境或规划状态。
