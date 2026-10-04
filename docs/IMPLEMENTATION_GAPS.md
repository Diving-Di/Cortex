# 未完成事项与生产风险

> 更新日期：2026-10-05
> 本文只记录真实缺口和不能对外承诺的事项。已实现能力见[根 README](../README.md)、[系统设计](SDD.md)和[RAG](RAG.md)；发布门禁统一见[发布清单](RELEASE_CHECKLIST.md)。

## P0：发布前必须关闭

- 在目标部署环境运行完整非 AI、AI、模板和活动验收；本地通过不能代替目标环境结果。
- 用目标环境的新 PostgreSQL 空库验证迁移版本 42、56 张业务表（连同 `schema_migrations` 共 57 张 public 表）、
  全部迁移、FORCE RLS、低权限 `cortex_app`、ready、注册和登录。
- 为迁移 `000035_knowledge_index_progress` 与 `000036_knowledge_clarifications` 补目标数据库验收：
  多 worker 竞争、租约过期接管、进度不倒退，以及澄清正常/重复/过期/跨租户恢复。
- 确认联合备份与隔离恢复报告仍适用于当前 schema 和数据卷布局；过期时重新演练。
- 在目标环境完成 MinIO、Kafka/Redpanda 与 Elasticsearch 的分阶段切换、回滚和联合恢复验收；
  Compose 默认启用不等于生产门禁已经通过。

## P1：上线前需要真实容量或质量证据

- AI 活动本地并发正确性已通过；[冷 Token 复测](operations/AI_EVENT_COLD_TOKEN_RERUN_20260826.md)达到当时兼容门槛，
  但[冷热混合复测](operations/AI_EVENT_MIXED_TOKEN_RERUN_20260826.md)的读接口 P95 未达体验门槛。
  生产目标规格、多 backend、真实入口与到达率模型仍缺少容量证据，不能仅凭冷 Token 成绩宣称上线达标。
- 知识检索已完成 PostgreSQL 路径的 100/1,000/10,000 合成文档测试，但当前 Elasticsearch 路径及
  HTTP、Embedding、Reranker、LiteLLM 的联合并发饱和点和 AI 成本尚未完整测量；历史的 10,000
  文档候选扫描结论不能直接代表当前 ES 投影性能。历史容量证据见
  [2026-08-25 基础设施验收](operations/INFRASTRUCTURE_ACCEPTANCE_20260825.md)。
- `RAG_PLANNER_ENABLED` 必须保持默认关闭，直到真实冻结的 comparison/trend/cross_period 数据集完成
  单查询对照，并记录 Hit@K、MRR、Context Recall/Precision、引用通过率、拒答准确率、P95、调用次数和成本。
- Compose 已提供 Prometheus/Alertmanager/Grafana 实际采集、基础设施 exporter、HTTP SLI 和默认阈值；
  生产通知接收器、当期 primary/secondary、目标规格阈值和真实告警送达仍必须由部署环境完成。
- 真实私人 bad case 不随仓库分发；只有用户主动复核和脱敏后才可晋升评测集，因此持续质量闭环仍需
  在真实使用中积累证据。

## P2：候选实验，不直接上线

- 分片上传、Redis Bitmap 续传状态、ES 故障自动切换 pgvector 尚未实现；现有上传与检索行为见 [基础设施实现](INFRASTRUCTURE_EVOLUTION.md)，验收条件见下文“候选能力的验收边界”。
- GC 已使用任务对象版本删除，但知识上传常规入库尚未保存每个 Put 返回的版本；版本化桶中的知识文件需要补齐全链路元数据持久化与验收。

- Step-back、HyDE、三级分块和 Auto-merging 只允许在冻结集进行离线消融；没有可解释增益时保持现有
  child 召回、parent 聚合和查询改写，不替换线上数据模型。
- Excel 与演示文稿摄取尚未实现。若未来纳入，必须沿用隔离解析 worker、文件/页数/解压比/超时
  限制、表格与页码溯源、解析器/chunker 版本和配额回滚，不能只开放扩展名。
- 团队知识库、云盘同步、计费、桌面组件以及数据库与 Markdown 双向同步不在当前产品范围。
- PDF、DOC/DOCX、PNG/JPG/WebP 已通过隔离解析/OCR 服务接入；页码当前作为生成的 Markdown 分节保留，结构化页码引用字段仍属于后续增强。

## 已知运维边界

- 本地恢复演练测得的 RPO/RTO、容量测试 wall time 和回环网络延迟都不是生产 SLA。
- 源卷历史孤儿文件需要正式保留策略；未授权不得直接批量删除。
- AI、Embedding、Reranker、OCR 或 Redis 不可用时必须保持非 AI 主链路可用；不得用提高可用性为由
  绕过来源、RLS、幂等、配额或引用核验。


## 候选能力的验收边界

### 大文件与续传

候选方案为服务端上传会话、MinIO Multipart、逐分片 checksum、Redis Bitmap 加速进度查询。
PostgreSQL 应保存会话和分片事实，Redis 丢失后可重建；完成提交需要幂等、配额结算和安全孤儿清理。
当前没有 `/api/v1/uploads/*` 分片接口，现有知识上传仍是单次 multipart/form-data 文件上传。

验收需覆盖断网、刷新、服务重启、重复/乱序分片、并发 complete、会话过期和跨租户访问。

### 检索韧性与投影修复

候选方案包括显式 ES 故障熔断、pgvector 降级、恢复探针与切回，以及活动文档与 ES 投影的定时对账。
当前两个 backend 通过配置选择；不能把“可以配置 postgres”描述为自动故障转移。
降级仍需 RLS、活动版本校验、精排、证据门控和引用验证；向量兜底阈值需独立校准。
中文 2-gram 仍在当前 PostgreSQL 路径使用，删除它必须有明确迁移和质量对照。

### 知识文件版本与引用

补齐知识上传对 Put 返回的 version/etag 的持久化，使上传、迁移、GC 和恢复清单具有一致定位。
PDF 页码、Word 段落、图片区域的结构化来源字段及 OCR 置信度门控，需要端到端验收后才能承诺。
Excel 和演示文稿仍不在当前摄取范围，不能只放开扩展名。

### 生产化

按目标环境完成 Kafka/ES 多节点、TLS、权限、容量、监控送达和联合恢复，不将本机性能当作 SLA。
质量评测区分 ES 与 PostgreSQL、线上与离线调用链；公开夹具与人工复核的私人 bad case 分别管理。
Step-back、HyDE、Auto-merging 等实验先做冻结集消融，达成质量、延迟和成本目标后再决定是否上线。


## 当前体验能力的边界

当前已补齐笔记筛选、版本预览/恢复、浏览器草稿、冲突比较、报告来源绑定、报告正文历史、
来源片段、问答保存、待处理入口、引导和轻量部署文件，详见页面说明与当期验收记录。
浏览器 sessionStorage 草稿不跨标签页、设备同步，不是灾备；存储不可用时会提示。
报告正文历史不含旧来源快照，未完成生成尝试未新增服务端持久化表。
PDF 结构化页码/坐标高亮仍需解析模型与迁移支持；当前提供现有章节和有限片段。
待处理卡片最多检查前 20 个定时任务，其余从报告页查看。
轻量部署可选 CPU 模型 profile 的真实性能和完整 AI 链路需独立验收。
模板保留现有点赞/排行/推荐，暂缓新增社交能力；未来删减须依据真实使用数据。
