# RAG 故障处置 Runbook

本文只使用稳定错误码和低基数指标定位问题。禁止在工单、日志或指标标签中复制用户 query、回答、
上下文、邮箱、姓名或租户 ID。

## Reranker 故障

- 检测：`cortex_knowledge_rerank_failed_total` 持续增长，客户端收到 `KNOWLEDGE_RERANK_UNAVAILABLE`。
- 定位：按后端生成的 request ID 检查阶段化日志，确认超时、非 2xx、数量不一致、重复或越界 index。
- 缓解：检查 `reranker-service` 健康和资源使用；必要时回滚最近镜像。当前不允许静默绕过精排。
- 恢复：服务健康后用合成请求确认返回 index 完整且唯一，再观察错误计数停止增长。

## 索引租约丢失或积压

- 检测：`cortex_knowledge_index_lease_lost_total` 增长，或最老 queued/running job 超过运行阈值。
- 定位：使用 document/job ID 检查 `status`、`attempts`、`lease_owner`、`lease_until` 和稳定 failure code。
- 缓解：不要手工把旧 owner 的结果标为成功；等待租约到期由新 worker 接管，或通过认证重试接口入队。
- 恢复：确认 active index version 未被旧 owner 改写、旧版本仍能检索、新 job 最终 success。

自动化 fault test `TestKnowledgeChunksRollbackBeforeActivationKeepsOldVersion` 在 chunks 写入后、激活前
注入错误，验证新版本 chunks 回滚、旧 active version/status 不变，且 running job 可继续由租约机制处理。

## SSE 未完成与来源失效

- 检测：`cortex_knowledge_stream_incomplete_total` 或 `cortex_knowledge_source_invalid_total` 增长。
- 定位：按 request ID 查看 `upstream_stage` 和错误码，不读取普通日志中的正文（正文不应存在）。
- 缓解：已输出 delta 的流禁止从头自动重试；客户端展示 incomplete，并由用户发起新请求。来源失效时重新检索。
- 恢复：确认失败结果以 `failed` 持久化且没有 `done`，重放同一 request ID 不生成重复回答。

## 无证据率异常

- 检测：`cortex_knowledge_no_evidence_total` 相对请求量异常上升。
- 定位：比较索引队列、Embedding、关键词/标题候选数、Reranker 门槛和最近数据集版本。
- 缓解：优先回滚检索参数或索引版本；不得通过无上限增加上下文掩盖召回问题。
- 恢复：运行确定性资产校验和受控 retrieval-only 回归，确认既有门槛后再恢复发布。

## Scheduler 租约丢失与重复执行

- 检测：`cortex_scheduled_report_lease_lost_total` 或 `cortex_scheduled_report_runs_failed_total` 增长。
- 定位：检查 task/run ID、`lease_owner`、`lease_until`、run 状态与错误码；不要记录报告正文。
- 缓解：旧 owner 不得继续写报告或完成 run。等待未过期 owner 完成；确认进程已退出时才等待租约到期接管。
- 恢复：验证新 owner 能续租和完成，旧 owner 的 Start/ConfirmReport/Finish 均返回 fencing 错误；
  同一租户、类型、周期只能存在一份报告笔记。
