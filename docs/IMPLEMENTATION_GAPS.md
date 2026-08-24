# 未完成事项与生产风险

> 更新日期：2026-08-19
> 本文只记录真实缺口和不能对外承诺的事项。已实现能力见 `README.md`、`docs/BASELINE.md`、
> `docs/SDD.md` 和 `docs/RAG.md`；发布门禁统一见 `docs/RELEASE_CHECKLIST.md`。

## P0：发布前必须关闭

- 在目标部署环境运行完整非 AI、AI、研究、模板和活动验收；本地通过不能代替目标环境结果。
- 用新 PostgreSQL 空库验证 58 张表、全部迁移、FORCE RLS、低权限 `cortex_app`、ready、注册和登录。
- 为迁移 `000035_knowledge_index_progress` 与 `000036_knowledge_clarifications` 补目标数据库验收：
  多 worker 竞争、租约过期接管、进度不倒退，以及澄清正常/重复/过期/跨租户恢复。
- 确认联合备份与隔离恢复报告仍适用于当前 schema 和数据卷布局；过期时重新演练。

## P1：上线前需要真实容量或质量证据

- AI 活动本地正确性已通过，但单实例冷 Token 认证和数据库热点仍是容量瓶颈；生产目标规格、多 backend、
  真实入口与到达率模型尚未完成复测。详细证据见 `operations/AI_EVENT_LOAD_TEST_20260816.md`。
- 知识检索已完成 100/1,000/10,000 合成文档测试，但 HTTP、Embedding、Reranker、LiteLLM 并发饱和点和
  AI 成本尚未完整测量；10,000 文档候选扫描是已知边界。见 `operations/CAPACITY_20260813.md`。
- `RAG_PLANNER_ENABLED` 必须保持默认关闭，直到真实冻结的 comparison/trend/cross_period 数据集完成
  单查询对照，并记录 Hit@K、MRR、Context Recall/Precision、引用通过率、拒答准确率、P95、调用次数和成本。
- Prometheus/Grafana 资产已经提供，但实际采集、告警路由、值班负责人和生产阈值仍由部署环境完成。
- 真实私人 bad case 不随仓库分发；只有用户主动复核和脱敏后才可晋升评测集，因此持续质量闭环仍需
  在真实使用中积累证据。

## P2：候选实验，不直接上线

- Step-back、HyDE、三级分块和 Auto-merging 只允许在冻结集进行离线消融；没有可解释增益时保持现有
  child 召回、parent 聚合和查询改写，不替换线上数据模型。
- PDF、Word、Excel 摄取尚未实现。若未来纳入，必须同时设计隔离解析 worker、文件/页数/解压比/超时
  限制、表格与页码溯源、解析器/chunker 版本和配额回滚，不能只开放扩展名。
- 团队知识库、云盘同步、计费、桌面组件以及数据库与 Markdown 双向同步不在当前产品范围。

## 已知运维边界

- 本地恢复演练测得的 RPO/RTO、容量测试 wall time 和回环网络延迟都不是生产 SLA。
- 源卷历史孤儿文件需要正式保留策略；未授权不得直接批量删除。
- AI、Embedding、Reranker、OCR 或 Redis 不可用时必须保持非 AI 主链路可用；不得用提高可用性为由
  绕过来源、RLS、幂等、配额或引用核验。
