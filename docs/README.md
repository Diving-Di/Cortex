# Cortex 文档导航

> 校对日期：2026-09-07。当前实现以代码、迁移和 Compose 为依据，工程约束见 [AGENTS.md](../AGENTS.md)。

## 从哪里开始

| 任务 | 阅读入口 |
| --- | --- |
| 启动项目、配置依赖 | [项目 README](../README.md) |
| 了解产品与技术边界 | [工程基线](BASELINE.md)、[系统设计](SDD.md) |
| 调用后端接口 | [API 契约](api.md) |
| 理解检索与文件处理 | [RAG 当前链路](RAG.md)、[基础设施当前实现](INFRASTRUCTURE_EVOLUTION.md) |
| 运行数据库集成测试 | [本机开发与验收](guides/LOCAL_DEVELOPMENT.md) |
| 排查运行故障 | [基础设施 Runbook](runbooks/OPERATIONS.md)、[RAG Runbook](runbooks/RAG_FAILURES.md) |
| 准备发布或恢复 | [发布检查清单](RELEASE_CHECKLIST.md)、[外部基础设施运维](EXTERNAL_INFRA_OPERATIONS.md)、[SLO](SLO.md) |
| 查看尚未完成的工作 | [实现缺口](IMPLEMENTATION_GAPS.md)、[基础设施规划](plans/INFRASTRUCTURE_ROADMAP.md) |

## 文档分工

- 根目录文档：当前产品、架构、HTTP/SSE 契约与发布规范；[LLM 网关规范](LLM_GATEWAY.md) 单独维护 AI 接入边界。
- [page/](page/README.md)：页面、路由、数据流和交互说明。
- [guides/](guides/LOCAL_DEVELOPMENT.md)：可执行的本机开发、测试与环境配置步骤。
- [runbooks/](runbooks/OPERATIONS.md)：故障定位和恢复操作。
- [operations/](operations/README.md)：按日期保存验收证据；注明运行环境、配置与未覆盖项。
- [rag-baselines/](rag-baselines/README.md)：历史冻结质量基线，不能自动代表当前 Elasticsearch 路径。
- [plans/](plans/INFRASTRUCTURE_ROADMAP.md)：尚未实现或尚未验收的候选能力。
- [archive/](archive/README.md)：被当前说明替代的设计快照，仅供历史对照。

## 当前事实

当前 schema 迁移为 **42**：56 张业务表，加上 `schema_migrations` 共 57 张 public 表。
后端唯一服务入口是 `backend/cmd/server/main.go`，支持 `all/api/worker` 运行角色。
Compose 默认使用 MinIO、Kafka/Redpanda、Elasticsearch；本机直启默认配置与 Compose 不完全相同。

附件 GC 使用 PostgreSQL 任务和可回收租约，按任务后端与对象版本删除；它不是 Kafka 消费者。
断点续传、Redis Bitmap 分片状态和 Elasticsearch 故障自动切换 pgvector 尚未实现。
当前活动领取接口发放免费 AI 点数，不应将领取成功描述成已经生成深度月报。

最近的功能验收见 [2026-09-07 本机验收](operations/LOCAL_INTEGRATION_ACCEPTANCE_20260907.md)。
生产配置与发布流程的历史证据见 [2026-08-31 验收](operations/PRODUCTION_REMEDIATION_ACCEPTANCE_20260831.md)；
本机结果不替代目标环境发布、容量和灾备验收。

## 维护规则

1. 改接口先核对 `backend/internal/server/server.go`，改默认值先核对 config 与 Compose。
2. 修改数据结构时增加版本化迁移，同步工程基线和相关契约。
3. 当前说明只写已实现行为；规划单列，不使用历史方案宣称实现完成。
4. 日期报告保留原始环境与结论；新验收另建报告并更新目录索引。
5. 只提交脱敏文字结论与公开测试夹具；原始备份、日志、私有评测内容放在忽略目录或项目外。
