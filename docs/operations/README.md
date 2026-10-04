# 按日期保存的验收证据

报告只证明当时的环境与测试范围，不代表当前全部生产门禁已完成。原始日志、数据库备份和私人评测产物不随仓库分发。

当前配置与命令应从 [文档导航](../README.md) 查阅。历史报告中的“当前”“本轮”均指报告日期；
保留旧测试数、表数和失败结果，不将它们更新为后来实现。已知行为变化以顶部说明和现行文档链接提示。

| 日期 | 报告 | 适用范围 |
| --- | --- | --- |
| 2026-10-05 | [个人工作流与轻量部署](PRODUCT_WORKFLOW_ACCEPTANCE_20261005.md) | 草稿/冲突/历史、报告来源、问答保存、48 项前端测试、3 项浏览器流程、Go 集成与无 AI 轻量栈 |
| 2026-09-07 | [本机数据库与功能验收](LOCAL_INTEGRATION_ACCEPTANCE_20260907.md) | 迁移 42、GC、MinIO 版本删除、全量 Go、前端、HTTP/AI 与隔离恢复 |
| 2026-08-31 | [上线整改](PRODUCTION_REMEDIATION_ACCEPTANCE_20260831.md) | 配置、镜像、浏览器与本地完整栈 |
| 2026-08-26 | [冷 Token 复验](AI_EVENT_COLD_TOKEN_RERUN_20260826.md) | AI 活动冷认证性能与正确性 |
| 2026-08-26 | [混合 Token 复验](AI_EVENT_MIXED_TOKEN_RERUN_20260826.md) | 热/冷 Token 混合负载 |
| 2026-08-25 | [基础设施验收](INFRASTRUCTURE_ACCEPTANCE_20260825.md) | 当时 Compose、故障、恢复与观测 |
| 2026-08-25 | [RAG 与负载](RAG_AND_K6_RERUN_20260825.md) | 特定 backend/配置下的检索质量与活动负载 |
| 2026-08-25 | [多格式摄取](MULTIFORMAT_KNOWLEDGE_INGESTION_20260825.md) | PDF/Word/图片解析与知识索引 |
| 2026-08-24 | [早期 RAG 与生产验收](RAG_AND_PRODUCTION_ACCEPTANCE_20260824.md) | 历史成本、质量与运维缺口对照 |

新增报告应注明日期、代码/镜像、环境、执行命令、结果、跳过项和不能外推的边界。
