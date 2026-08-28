# 未完成事项与生产风险

> 更新日期：2026-08-28
> 本文只记录真实缺口和不能对外承诺的事项。已实现能力见 `README.md`、`docs/BASELINE.md`、
> `docs/SDD.md` 和 `docs/RAG.md`；发布门禁统一见 `docs/RELEASE_CHECKLIST.md`。

## P0：发布前必须关闭

- 在目标部署环境运行完整非 AI、AI、研究、模板和活动验收；本地通过不能代替目标环境结果。
- 用新 PostgreSQL 空库验证迁移版本 41、63 张 public 表、全部迁移、FORCE RLS、低权限 `cortex_app`、ready、注册和登录。
- 为迁移 `000035_knowledge_index_progress` 与 `000036_knowledge_clarifications` 补目标数据库验收：
  多 worker 竞争、租约过期接管、进度不倒退，以及澄清正常/重复/过期/跨租户恢复。
- 确认联合备份与隔离恢复报告仍适用于当前 schema 和数据卷布局；过期时重新演练。
- 在目标环境完成 MinIO、Kafka/Redpanda 与 Elasticsearch 的分阶段切换、回滚和联合恢复验收；
  Compose 默认启用不等于生产门禁已经通过。

## P1：上线前需要真实容量或质量证据

- AI 活动本地正确性已通过，但单实例冷 Token 认证和数据库热点仍是容量瓶颈；生产目标规格、多 backend、
  真实入口与到达率模型尚未完成复测。详细证据见 `operations/RAG_AND_K6_RERUN_20260825.md`。
- 知识检索已完成 PostgreSQL 路径的 100/1,000/10,000 合成文档测试，但当前 Elasticsearch 路径及
  HTTP、Embedding、Reranker、LiteLLM 的联合并发饱和点和 AI 成本尚未完整测量；历史的 10,000
  文档候选扫描结论不能直接代表当前 ES 投影性能。最新本地基线见
  `operations/INFRASTRUCTURE_ACCEPTANCE_20260825.md`。
- `RAG_PLANNER_ENABLED` 必须保持默认关闭，直到真实冻结的 comparison/trend/cross_period 数据集完成
  单查询对照，并记录 Hit@K、MRR、Context Recall/Precision、引用通过率、拒答准确率、P95、调用次数和成本。
- Prometheus/Grafana 资产已经提供，但实际采集、告警路由、值班负责人和生产阈值仍由部署环境完成。
- 真实私人 bad case 不随仓库分发；只有用户主动复核和脱敏后才可晋升评测集，因此持续质量闭环仍需
  在真实使用中积累证据。

## P2：候选实验，不直接上线

- Step-back、HyDE、三级分块和 Auto-merging 只允许在冻结集进行离线消融；没有可解释增益时保持现有
  child 召回、parent 聚合和查询改写，不替换线上数据模型。
- Excel 与演示文稿摄取尚未实现。若未来纳入，必须沿用隔离解析 worker、文件/页数/解压比/超时
  限制、表格与页码溯源、解析器/chunker 版本和配额回滚，不能只开放扩展名。
- 团队知识库、云盘同步、计费、桌面组件以及数据库与 Markdown 双向同步不在当前产品范围。
- Excel 与演示文稿的知识库摄取尚未实现。PDF、DOC/DOCX、PNG/JPG/WebP 已通过隔离解析/OCR 服务接入；页码当前作为生成的 Markdown 分节保留，结构化页码引用字段仍属于后续增强。

## 已知运维边界

- 本地恢复演练测得的 RPO/RTO、容量测试 wall time 和回环网络延迟都不是生产 SLA。
- 源卷历史孤儿文件需要正式保留策略；未授权不得直接批量删除。
- AI、Embedding、Reranker、OCR 或 Redis 不可用时必须保持非 AI 主链路可用；不得用提高可用性为由
  绕过来源、RLS、幂等、配额或引用核验。

## 本轮已关闭

- 2026-08-25：外部基础设施 worker 已迁入 `backend/internal/workers` 并由唯一的 `cmd/server` 托管；
  镜像与 Compose 已停止构建和部署四个额外 worker 二进制。生产环境门禁仍按上方 P0/P1 独立验收。
- 2026-08-27：迁移 `000041_kafka_knowledge_pipeline` 已把知识摄取拆为解析、Embedding、搜索投影
  三个 Kafka 阶段；阶段进度、租约、重试和最终状态继续以 PostgreSQL 为准。
- 2026-08-28：HTTP handler 到 `application` 用例服务的边界已覆盖认证、租户、笔记、附件、AI、
  报告、知识库、研究、模板、活动等现有领域，并由架构测试约束依赖方向。
