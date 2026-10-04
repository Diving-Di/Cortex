# Cortex 运维与故障处置手册

校对日期：2026-10-05。发布门禁见 [发布清单](../RELEASE_CHECKLIST.md)，阈值和责任角色见 [SLO](../SLO.md)。

所有指标均不使用租户、邮箱、姓名、正文或 query 标签。处置记录只保存时间、服务、资源 ID、稳定错误码和聚合计数。

## readyz 持续失败

- 影响：需要数据库的 API 不可用；对象存储、Redis、Kafka、搜索和 AI 是否可用不影响 `/readyz` 判定。
- 定位：检查 `cortex_database_ready`、数据库容器健康、连接池和迁移状态，再按 request ID 检查稳定错误码。
- 缓解：阻止新版本继续发布；恢复数据库连接或回滚最近数据库配置。不得临时改用迁移高权限连接运行后端。
- 恢复：`/readyz` 连续成功，低权限连接、注册、登录和跨租户 404 smoke 通过。
- 升级：两分钟未恢复或出现数据一致性错误时进入恢复流程。

## 数据卷容量压力

附件已逻辑删除但物理空间尚未回收时，检查 `object_gc_jobs`。迁移 42 后 running 租约过期可重新 claim；
后端不匹配或 MinIO 未配置会进入失败重试。不要只改状态为 success，也不要根据当前上传后端判断历史对象位置。

- 影响：附件、知识文件上传或导出可能失败。
- 定位：按当前 `storage_backend` 确认 MinIO bucket 或本地兼容卷使用率、增长前缀和数据库记录计数；不得在工单复制私人文件名。
- 缓解：暂停非必要导入并扩容数据卷。只通过产品删除流程清理数据，禁止直接删除未知文件。
- 恢复：容量低于阈值，DB/文件双向孤儿校验通过，上传下载 smoke 成功。
- 升级：达到 95% 或出现写失败时按严重事件处理。

## 备份过期或恢复演练失败

- 影响：最近写入可能无法在灾难后恢复，或备份包未经可恢复性验证。
- 定位：检查 `cortex_backup_last_success_unixtime`、`cortex_restore_drill_last_success_unixtime`、manifest checksum 和隔离演练报告。
- 缓解：立即执行联合备份；只在新容器和新 volume 中做恢复验证，禁止覆盖当前 volume。
- 恢复：checksum、迁移、RLS、低权限连接、DB/文件一致性和 non-AI smoke 全部通过后更新时间指标。
- 升级：checksum 不一致或恢复数据不一致时保留证据并停止切流。

## 监控采集或告警路由失败

- 影响：服务可能仍在运行，但 SLO、容量、备份过期和队列异常无法可靠发现。
- 定位：检查 Prometheus Targets、规则加载、Alertmanager status 和 receiver 配置；指标缺失时先区分 exporter 故障与被监控服务故障。
- 缓解：冻结发布，恢复采集或通知链路；不得仅删除告警或放宽阈值来恢复绿色状态。
- 恢复：所有必需 target 连续 10 分钟为 up，并注入测试告警确认 primary、升级链路和恢复通知送达。
- 升级：critical 通知 15 分钟无人确认，升级 secondary 和服务负责人。

## API SLO 越界

- 影响：用户请求出现持续 5xx 或高延迟，正在快速消耗 30 天错误预算。
- 定位：按固定 route 模板查看请求率、5xx 和 P95，再关联数据库连接池、Kafka/Outbox、ES 和上游依赖；不得添加 tenant/user/query 标签。
- 缓解：发布观察窗口内立即停止切流并执行应用镜像回退；非发布事件优先隔离故障依赖，保持非 AI 主链路。
- 恢复：5xx 燃烧率和 P95 连续 15 分钟回到阈值内，ready、认证和 non-AI smoke 通过。
- 升级：涉及数据一致性、跨租户或 RLS 时直接按严重事件处理，不等待燃烧窗口。

## 外部基础设施切换与恢复

生产发布顺序固定为 MinIO、Kafka、Elasticsearch，禁止在同一次发布中同时切换三条主路径。所有凭据通过 Secret 注入；Compose 中的单节点 Kafka 与 Elasticsearch 仅用于本地开发，生产必须使用三节点、TLS、最小权限账号、显式 Topic 和快照仓库。

### 上线门禁

已有实例按版本顺序运行全部待应用迁移，当前最终版本为 42；下列编号用于说明各组件的迁移依赖，不是跳号执行顺序。
升级 GC worker 前应用 42，混合版本部署时应先停旧 worker，避免旧完成逻辑绕过 owner fencing。

1. 确认 `000037` 已应用，运行历史对象迁移与双向 checksum 对账；切换 `STORAGE_BACKEND=minio` 后验证新上传只写目标 MinIO，并保留旧后端直至历史对象核验完成。
2. 执行 `000038`、`000040` 与 `000041`，显式创建知识索引、文档解析、Embedding、搜索投影等
   `cortex.*.v1` Topic，再启动 relay 和分阶段消费者。停 Kafka 时业务事务应成功、Outbox 应积压，恢复后应清空。
3. 执行 `000039`，回放全部活动版本并比较冻结集；完成影子查询后才设置 `RAG_RETRIEVAL_BACKEND=elasticsearch`。
4. 观察一个完整发布周期后方可执行 contract 收敛。`/healthz` 只检查 API 进程；API `/readyz` 只检查 PostgreSQL；对象存储、Redis、搜索和 AI 状态由 `/health/dependencies` 独立报告。

### 故障与恢复

- GC 卡住：检查 `object_gc_jobs` 的 status、lease_owner、lease_expires_at 与 last_error_code；迁移 42 后过期 running 可重新 claim。任务后端为 MinIO 时应保留对应 bucket/凭据，即使当前新上传改用 local。

- MinIO 丢失对象：保持数据库行不变，从 PostgreSQL dump 对应的版本化对象清单恢复，再校验大小和 SHA-256。未校验前不得切换 `storage_backend`。
- Kafka 全部不可用：停止消费者，保留 Outbox；恢复 broker、Topic 配置和 schema 后启动 relay。重复事件由 `consumer_receipts` 消除。
- Elasticsearch 不可用：RAG 返回 `KNOWLEDGE_RETRIEVAL_UNAVAILABLE`；普通笔记搜索继续走 PostgreSQL。由活动 `index_version` 全量重建投影并原子切换别名。
- 联合恢复：先 PostgreSQL，再 MinIO，最后重建 Kafka 配置与 Elasticsearch 投影。Kafka offset 和 ES 数据均不作为业务完成事实。

验收至少执行 `go vet ./...`、`go test ./...`、`go build ./cmd/server`、`go build ./cmd/blob-migrate`、
`docker compose config --quiet`，并完成跨租户、重复/乱序、broker 中断、对象丢失和 ES 断网故障注入。

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

Kafka 模式还需分别检查解析、Embedding、搜索投影 consumer group 的 lag、DLQ、receipt 与阶段更新时间。
不得通过手工跳过中间阶段把任务标为成功；broker 恢复后由 PostgreSQL 中的任务版本和 lease/fencing
继续推进，Kafka offset 不作为业务完成事实。

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
