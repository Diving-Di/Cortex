# 基础设施候选规划

> 本文均为目标或待验收事项，不代表当前 API 与实现。现状见[基础设施实现](../INFRASTRUCTURE_EVOLUTION.md)，生产阻断见[实现缺口](../IMPLEMENTATION_GAPS.md)。

## 大文件与续传

候选方案为服务端上传会话、MinIO Multipart、逐分片 checksum、Redis Bitmap 加速进度查询。
PostgreSQL 应保存会话和分片事实，Redis 丢失后可重建；完成提交需要幂等、配额结算和安全孤儿清理。
当前没有 `/api/v1/uploads/*` 分片接口，现有知识上传仍是单次 multipart/form-data 文件上传。

验收需覆盖断网、刷新、服务重启、重复/乱序分片、并发 complete、会话过期和跨租户访问。

## 检索韧性与投影修复

候选方案包括显式 ES 故障熔断、pgvector 降级、恢复探针与切回，以及活动文档与 ES 投影的定时对账。
当前两个 backend 通过配置选择；不能把“可以配置 postgres”描述为自动故障转移。
降级仍需 RLS、活动版本校验、精排、证据门控和引用验证；向量兜底阈值需独立校准。
中文 2-gram 仍在当前 PostgreSQL 路径使用，删除它必须有明确迁移和质量对照。

## 知识文件版本与引用

补齐知识上传对 Put 返回的 version/etag 的持久化，使上传、迁移、GC 和恢复清单具有一致定位。
PDF 页码、Word 段落、图片区域的结构化来源字段及 OCR 置信度门控，需要端到端验收后才能承诺。
Excel 和演示文稿仍不在当前摄取范围，不能只放开扩展名。

## 生产化

按目标环境完成 Kafka/ES 多节点、TLS、权限、容量、监控送达和联合恢复，不将本机性能当作 SLA。
质量评测区分 ES 与 PostgreSQL、线上与离线调用链；公开夹具与人工复核的私人 bad case 分别管理。
Step-back、HyDE、Auto-merging 等实验先做冻结集消融，达成质量、延迟和成本目标后再决定是否上线。

原始设计讨论见[2026-08-24 快照](../archive/INFRASTRUCTURE_DESIGN_20260824.md)。
