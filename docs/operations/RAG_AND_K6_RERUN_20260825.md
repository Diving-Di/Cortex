# RAG 链路与 k6 压测复验（2026-08-25）

> 适用边界：Windows、Docker Desktop、本地单 backend Compose 环境。结果用于本次基础设施演进后的
> 回归对照，不是生产 SLA。原始 RAG case（可能包含上下文和模型回答）与 k6 机器结果保存在本地忽略的
> `artifacts`，不提交仓库。

## 1. 环境与范围

- 代码基线：`732cce9dcc043f5ed3e455ef00ae859060495f1a` 加当前基础设施演进工作树。
- 后端入口：唯一 `cmd/server`，内部托管 Outbox、知识索引、搜索投影和对象 GC runner。
- 基础设施：PostgreSQL 16、Redis 7、MinIO、Redpanda/Kafka、Elasticsearch、LiteLLM、
  GTE Embedding、BGE Reranker。
- RAG：冻结 `knowledge_eval_merged.jsonl` 164 用例，4 workers，包含召回、rerank、证据门控、
  `cortex-default` 生成和 judge。
- k6：v2.2.0，11,000 个独立冷 Token，10,000 合格用户、1,000 不合格用户、500 份库存、2,000 VU。

## 2. RAG 全链路结果

原始目录：`artifacts/rag-eval/20260825-infra-rerun/20260825-091019`。

| 指标 | 2026-08-24 基线 | 本轮 | 变化 |
|---|---:|---:|---:|
| 用例成功 / 失败 | 164 / 0 | 164 / 0 | 持平 |
| 门控通过 / 拒绝 | 147 / 17 | 147 / 17 | 持平 |
| Hit@1 | 0.9939 | 0.9939 | 持平 |
| Hit@10 | 1.0000 | 1.0000 | 持平 |
| MRR（rerank 前） | 0.9557 | 0.9557 | 持平 |
| MRR（rerank 后） | 0.9970 | 0.9970 | 持平 |
| Context Recall | 0.8433 | 0.8356 | -0.0077 |
| Context Precision | 0.9775 | 0.9656 | -0.0119 |
| Faithfulness | 0.9440 | 0.9549 | +0.0109 |
| Answer Relevancy | 0.9244 | 0.9251 | +0.0007 |
| 总延迟 P50 | 5,455 ms | 4,978 ms | -477 ms |
| 总延迟 P95 | 9,546 ms | 10,224 ms | +678 ms |

召回与排序硬门槛保持通过：Hit@10 为 1.0，164 条全部执行成功，没有不可解释的 case 失败。Context
Recall/Precision 有小幅波动，Faithfulness 提升；P50 改善但 P95 上升 7.1%，主要长尾仍来自 reranker、生成与
judge。当前最弱业务分层仍包括 `cross-document`（Recall 0.3542）、`personal`（Recall 0.4889，门控拒绝
6/9）和 `substitution`（Faithfulness 0.6667），应继续作为质量优化重点。

本轮 RAG 配置为 512 维 GTE、`RAG_CONTEXT_PARENT_TOP_K=4`、BGE reranker、证据阈值 0.5038954；
评测报告的 Vector/Fulltext/Title 分通道统计是评测器内部可解释指标，不能单独当作 Elasticsearch 线上流量占比。

## 3. k6 压测结果

正式结果：

| 指标 | 阈值/预期 | 本轮结果 |
|---|---:|---:|
| 总请求 | 11,000 | 11,000 |
| 成功领取 | 500 | 500 |
| 售罄 | 9,500 | 9,500 |
| 不合格 | 1,000 | 1,000 |
| 非预期响应 | 0 | 0 |
| HTTP 失败率 | < 0.1% | 0% |
| 吞吐 | — | 488.32 req/s |
| 平均延迟 | — | 3.628 s |
| 中位数 | — | 1.996 s |
| P90 | — | 8.793 s |
| P95 | < 10 s | 9.114 s |
| P99 | < 20 s | 约 9.41 s |
| 最大延迟 | — | 9.989 s |

k6 退出码为 0，全部响应计数、错误率、P95 和 P99 阈值通过。数据库最终一致性为：

```text
total_slots=500
claimed_slots=500
occupied_inventory_slots=500
distinct_claim_tenants=500
reward_grants=500
```

与 2026-08-19 相同业务口径的历史结果相比，吞吐从 639.47 降到 488.32 req/s，P95 从 5.49 秒上升到
9.114 秒，仍在既定 10 秒门槛内但余量只剩约 0.886 秒。本轮没有同步窗口级 CPU/内存采样，因此不能把
差异归因于 backend、数据库或 Docker Desktop；后续生产规格测试必须接入 Prometheus 时间序列后再定位。

正式原始文件：

- `artifacts/ai-event-k6-rerun-1787649126-final-summary.json`
- `artifacts/ai-event-k6-rerun-1787649126-final-console.txt`
- `artifacts/ai-event-k6-rerun-1787649126-final-facts.txt`

准备阶段曾有一轮 Token 摘要因 psql `-c` 变量未展开而全部返回认证失败。该轮没有 Claim、库存占用或奖励
变更，不计入性能结果；修正摘要生成并重置活动窗口后，从零执行了上述正式轮。

## 4. 清理与恢复

- 本轮 11,000 个短期 Token、50,000 条资格笔记、资格快照、Claim、reservation、点数账户和奖励账本
  已在单事务中清理。
- 测试用户残留为 0；活动恢复为 10 份库存、0 已领取、0 已占用槽位。
- `AI_EVENT_CLAIM_IP_LIMIT` 已从压测临时值 20,000 恢复为 60。
- backend 清理后重新创建并恢复 healthy。

## 5. 复现命令

```powershell
Set-Location backend
go run ./cmd/rag-regression-check
Set-Location ..
.\backend\scripts\rag_eval.ps1 -Workers 4 -Output artifacts/rag-eval/20260825-infra-rerun

$env:BASE_URL = "http://127.0.0.1:8000"
$env:TOTAL_REQUESTS = "11000"
$env:ELIGIBLE_USERS = "10000"
$env:EVENT_SLOTS = "500"
$env:VUS = "2000"
k6 run --summary-export artifacts/ai-event-k6-rerun-summary.json `
  .\backend\loadtest\ai_event_claim_100k.js
```

k6 命令还需要由隔离准备步骤注入 `EVENT_ID`、`RUN_ID` 和只存在于本轮进程中的 `RUN_SECRET`；不得把
Token 或运行密钥写入报告或提交仓库。

## 6. 结论

- RAG 正确性与核心检索门槛通过，但 P95 比上一基线上升，应继续观察长尾。
- k6 业务正确性、零超卖、零重复奖励及既定延迟阈值全部通过。
- k6 性能比 2026-08-19 历史最佳值回退，已接近 P95 门槛；在完成带资源时间序列的目标环境复测前，
  不应上调容量承诺或把本地 488 req/s 外推为生产 SLO。
