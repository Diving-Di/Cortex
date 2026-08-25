# 外部基础设施运行与恢复

> 当前状态（2026-08-25）：Compose 已默认启用 MinIO、Kafka 兼容的 Redpanda 与 Elasticsearch；
> 本文的分阶段门禁用于目标环境发布，不表示仅凭本地 Compose 配置即已完成生产验收。

生产发布顺序固定为 MinIO、Kafka、Elasticsearch，禁止在同一次发布中同时切换三条主路径。所有凭据通过 Secret 注入；Compose 中的单节点 Kafka 与 Elasticsearch 仅用于本地开发，生产必须使用三节点、TLS、最小权限账号、显式 Topic 和快照仓库。

## 上线门禁

1. 先执行 `000037`，运行历史对象迁移与双向 checksum 对账；确认新对象只写 MinIO 后再启用 `STORAGE_BACKEND=minio`。
2. 执行 `000038`，显式创建三个 `cortex.*.v1` Topic，再启动 relay 和消费者。停 Kafka 时业务事务应成功、Outbox 应积压，恢复后应清空。
3. 执行 `000039`，回放全部活动版本并比较冻结集；完成影子查询后才设置 `RAG_RETRIEVAL_BACKEND=elasticsearch`。
4. 观察一个完整发布周期后方可执行 contract 收敛。`/healthz` 只检查 API 进程；API `/readyz` 检查 PostgreSQL 与对象存储，不检查 Kafka、Elasticsearch 或 AI。

## 故障与恢复

- MinIO 丢失对象：保持数据库行不变，从 PostgreSQL dump 对应的版本化对象清单恢复，再校验大小和 SHA-256。未校验前不得切换 `storage_backend`。
- Kafka 全部不可用：停止消费者，保留 Outbox；恢复 broker、Topic 配置和 schema 后启动 relay。重复事件由 `consumer_receipts` 消除。
- Elasticsearch 不可用：RAG 返回 `KNOWLEDGE_RETRIEVAL_UNAVAILABLE`；普通笔记搜索继续走 PostgreSQL。由活动 `index_version` 全量重建投影并原子切换别名。
- 联合恢复：先 PostgreSQL，再 MinIO，最后重建 Kafka 配置与 Elasticsearch 投影。Kafka offset 和 ES 数据均不作为业务完成事实。

验收至少执行 `go vet ./...`、`go test ./...`、`go build ./cmd/server`、`go build ./cmd/blob-migrate`、
`docker compose config --quiet`，并完成跨租户、重复/乱序、broker 中断、对象丢失和 ES 断网故障注入。
