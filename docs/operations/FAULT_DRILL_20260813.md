# 2026-08-13 知识索引原子切换故障演练

## 范围与安全边界

演练使用临时合成租户、文档和 512 维零向量，不读取或输出私人正文。故障点位于新 chunks 已写入、
`active_index_version` 尚未切换之间；生产入口不暴露该 fault point。测试仍使用 `diary_app`、`Store.WithTx`、
transaction-local RLS 和显式 `tenant_id` 条件。

## 实际时间线

| 阶段 | 实际结果 |
| --- | --- |
| 检测 | 注入 `crash before activation` 后，写事务立即返回错误，测试进程检测到预期故障。 |
| 定位 | 通过合成 document/job ID 检查 active version、document status、新版本 parent 数和 job status。 |
| 缓解 | 整个 chunks 写入事务回滚；没有手工修改 job，也没有让旧 owner 标记成功。 |
| 恢复验证 | 旧 active version 仍为 1、document 仍为 ready、新版本 parent 数为 0、job 保持 running，可由租约机制继续处理。 |

实际命令和墙钟耗时记录在本次验收日志中；报告不把 Go 编译/容器启动时间冒充故障恢复时间。自动化断言是
`TestKnowledgeChunksRollbackBeforeActivationKeepsOldVersion`。生产告警由
`CortexKnowledgeLeaseLoss` / `CortexKnowledgeIndexBacklog` 触发，处置步骤见 RAG runbook。

## 结论与限制

该演练证明索引激活窗口的事务回滚和旧版本继续服务，不证明进程级真实宕机的 MTTR，也不代表生产 SLA。
