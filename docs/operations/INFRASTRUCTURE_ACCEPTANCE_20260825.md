# 基础设施演进本地目标环境验收（2026-08-25）

> 结论：当前 Compose 目标环境验收通过；本报告不替代生产三节点、真实告警通知和真实流量 SLA 证据。

## 部署与入口

- 唯一部署入口为 `cmd/server`；Outbox relay、知识索引、搜索投影和对象 GC 由
  `internal/workers` 托管。
- 新镜像只包含 `server`、`migrate`、`blob-migrate` 和 `rag-eval`；四个旧 worker 容器已移除。
- PostgreSQL、Redis、MinIO、Kafka、Elasticsearch、LiteLLM、Embedding、Reranker 和 backend
  均恢复 healthy；API `/healthz` 与 `/readyz` 正常。

## 功能与故障注入

- 非 AI smoke 通过：中文搜索、租户隔离、跨租户附件 404、导出和软删除认证失效均符合契约。
- AI acceptance 通过：`cortex-default` 聊天、整理草稿/确认、报告生成与来源保存均成功。
- Elasticsearch 中断时普通笔记、附件和导出保持可用；恢复后集群 healthy。
- Kafka 中断时业务主链路保持可用，runner 输出稳定错误码；恢复后自动重连且无持续提交错误。
- MinIO 中断时 `/healthz=200`、`/readyz=503`；恢复后 `/readyz=200`。

## 数据、恢复与可观测性

- 当前数据库迁移版本 40，共 63 张 public 表；50 张租户/业务表启用并强制 RLS。
- 联合备份包含 PostgreSQL dump、local 兼容数据和 MinIO 数据，全部记录 SHA-256。
- 隔离恢复通过：缺失文件 0、孤儿文件 0、非 AI smoke 通过；实测 RPO 92 秒、RTO 144.149 秒。
- 生产门禁脚本通过，扫描 1097 行日志未发现凭据模式；Prometheus 10 条规则通过 `promtool`。
- 合成 pgvector 容量基线无失败：10,000 文档、100 查询时 P50 300.968 ms、P95 373.32 ms、
  P99 416.782 ms。该结果不代表 Elasticsearch 或生产联合链路 SLA。

## 验证命令

- `gofmt -l .`、`go vet ./...`、`go test ./...`、`go build ./cmd/server`、`go build ./cmd/blob-migrate`
- `npm run format:check`、`npm test`、`npm run build`
- `docker compose config --quiet`
- `backend/scripts/non_ai_smoke.ps1`、`backend/scripts/ai_acceptance.ps1`
- `backend/scripts/production_acceptance.ps1`、`backend/scripts/validate_prometheus_rules.ps1`
- `backend/scripts/backup.ps1`、`backend/scripts/restore_verify.ps1`、`backend/scripts/knowledge_capacity.ps1`

生产发布仍须在真实三节点、TLS、告警通知目标和真实负载规格中复验，不得把本地数值描述为生产 SLA。
