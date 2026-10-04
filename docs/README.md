# Cortex 文档导航

校对日期：2026-10-05。现状以代码、版本化迁移与对应 Compose 文件为依据；工程约束见 [AGENTS.md](../AGENTS.md)。

## 当前说明

| 任务 | 唯一主入口 |
| --- | --- |
| 产品、架构、权限与数据规则 | [系统设计](SDD.md) |
| 启动和配置 | [根 README](../README.md) |
| 个人轻量版与完整部署选择 | [个人部署](guides/PERSONAL_DEPLOYMENT.md) |
| 页面、草稿、冲突、报告和来源操作 | [页面与使用流程](page/README.md) |
| HTTP、SSE 与错误契约 | [API](api.md) |
| 检索、切块、引用核验 | [RAG](RAG.md) |
| 对象存储、队列、索引、GC | [基础设施](INFRASTRUCTURE_EVOLUTION.md) |
| 模型路由、密钥与用量 | [网关规范](LLM_GATEWAY.md) |
| 本机测试与浏览器 | [开发验收](guides/LOCAL_DEVELOPMENT.md) |
| 冻结集与质量/成本对照 | [AI 质量回归](guides/QUALITY_EVALUATION.md) |
| 基础设施、RAG、Scheduler 故障与恢复 | [统一运维手册](runbooks/OPERATIONS.md) |
| 发布与生产目标 | [发布清单](RELEASE_CHECKLIST.md)、[SLO](SLO.md) |
| 尚未完成和候选能力 | [实现缺口](IMPLEMENTATION_GAPS.md) |

## 证据与历史

[operations/](operations/README.md) 按日期保存实测环境、命令、结果与限制。
本轮结果见 [2026-10-05 工作流与轻量部署验收](operations/PRODUCT_WORKFLOW_ACCEPTANCE_20261005.md)。
[rag-baselines/](rag-baselines/README.md) 保留历史冻结集结果；不代表当前 ES 链路。
历史报告不改写成当前结果。原始日志、备份、私人语料和未脱敏评测产物不随仓库提交。

## 文档整理与维护

当前说明按产品设计、接口契约、使用流程、部署/开发、运维和未完成事项分工；同一事实只在主文档维护。
本次累计合并或删除 10 份重复/被替代文档：

| 原文档 | 处理与有效内容去向 |
| --- | --- |
| `BASELINE.md` | 工程与数据规则并入 SDD，执行规范仍以根 AGENTS.md 为准 |
| `EXTERNAL_INFRA_OPERATIONS.md` | 切换、故障与联合恢复并入统一运维手册 |
| `plans/INFRASTRUCTURE_ROADMAP.md` | 未实现方案与验收条件并入 IMPLEMENTATION_GAPS |
| `page/` 下三个 `*_PAGE_ARCHITECTURE.md` | 页面行为合并到 page/README，接口和架构分别归入 API 与 SDD |
| `archive/` 下两份旧设计和 README | 当前 RAG/基础设施文档已替代；原版本可从 Git 历史查阅 |
| `runbooks/RAG_FAILURES.md` | RAG、索引和 Scheduler 故障处理并入统一运维手册 |

API 的日期化补充已归入对应业务章节；SDD 不再重复接口表和测试命令。
operations 与 rag-baselines 保留不可替代的实测数字和环境条件，不因版本较旧而删除或改写为当前结果。
新增验收只更新证据索引；变更规范应直接修改对应主文档，避免再增加“补充版”“最终版”。

## 当前事实

- 唯一后端服务入口 `backend/cmd/server/main.go`，支持 all/api/worker。
- 当前迁移 42：56 张业务表与 schema_migrations，共 57 张 public 表；本轮未改变 schema。
- 完整 Compose 默认 MinIO、Redpanda、ES；轻量 Compose 默认本地存储、PostgreSQL 任务与 pgvector 检索。
- 个人笔记正文以 PostgreSQL 为准，浏览器草稿只保存当前标签页未提交内容；关闭标签页不保证恢复。
- AI/模型可选，不可用时非 AI 记录、搜索、附件和导出继续可用。
- 分片续传、ES 自动故障切换、结构化 PDF 坐标引用仍未实现。
- 活动领取发放免费 AI 点数，不等于生成月报。

改路由同步页面流程，改接口同步 API，改 schema 新增迁移，改部署同步指南与对应验收证据。
发布状态以当期目标环境实测为准，不从历史记录推断已上线。
