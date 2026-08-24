# RAG 质量与本地目标环境验收（2026-08-24）

> 环境：Windows + Docker Desktop，本地 Compose；提交基线：`7dd505e57b0ef0922f9b62af9f962fcb5532ba53`。
> 本报告中的容量、延迟、成本、RPO/RTO 只代表该本地环境，不是生产 SLA。

## 1. 真实 bad case 与脱敏边界

- `knowledge_rag_feedback`：0 条。
- `knowledge_eval_cases`：0 条。
- `knowledge_rag_traces`：0 条。
- 因此当前没有经过用户反馈、人工复核和脱敏晋升的真实私人 bad case，禁止把冻结测试集描述为真实用户数据。
- 现有 `feedback → review → promote` 链路会只保存人工提供的脱敏 query、expected answer、证据 hash 与 tags；原始私人正文不得进入仓库评测集。
- 本轮用真实模型网关和当前持久化索引执行冻结的 164 条非私人回归集，得到可复现的部署实测 bad case；它们用于工程回归，不替代真实用户反馈。

## 2. 全链路 RAG 评测

输出目录：`artifacts/rag-eval/20260824-050353`（本地忽略，不提交 query、answer 或上下文）。

| 指标 | 结果 |
|---|---:|
| 用例 | 164 |
| 失败 | 0 |
| 在线同口径门控通过 / 拒绝 | 147 / 17 |
| Hit@1 / Hit@10 | 0.9939 / 1.0000 |
| MRR（rerank 前 / 后） | 0.9557 / 0.9970 |
| Context Recall / Precision | 0.8433 / 0.9775 |
| Faithfulness / Answer Relevancy | 0.9440 / 0.9244 |
| P50 / P95 总延迟 | 5,455 ms / 9,546 ms |

高优先级实测 bad-case 分层：

| 标签 | 样本 | 门控拒绝 | Context Recall | Faithfulness | P95 |
|---|---:|---:|---:|---:|---:|
| personal | 9 | 6 | 0.4444 | 1.0000 | 7,765 ms |
| cross-document | 6 | 2 | 0.2500 | 0.7917 | 8,245 ms |
| substitution | 4 | 1 | 0.6111 | 0.5000 | 6,984 ms |
| analysis | 6 | 0 | 0.7056 | 0.9167 | 10,778 ms |
| table | 12 | 2 | 0.7683 | 0.8833 | 10,778 ms |
| hard | 26 | 3 | 0.7268 | 0.9029 | 11,268 ms |

`rag-eval` 已改为自动按全部 tags 输出样本、门控拒绝、检索、上下文、生成质量与 P95，避免只看总分掩盖弱分层。

## 3. 容量与成本

容量脚本：`backend/scripts/knowledge_capacity.ps1`，每个规模 100 次查询，失败均为 0。

| 文档 | 写入 docs/s | 检索 P50 | P95 | P99 | 索引关系大小 |
|---:|---:|---:|---:|---:|---:|
| 100 | 99.599 | 8.695 ms | 21.981 ms | 33.904 ms | 37,019,648 B |
| 1,000 | 419.426 | 32.900 ms | 52.713 ms | 67.380 ms | 40,640,512 B |
| 10,000 | 1,402.581 | 290.743 ms | 329.315 ms | 348.805 ms | 83,222,528 B |

10,000 文档 P95 已超过 300 ms，应继续把候选扫描作为扩容边界，而不是宣称无限扩展。

LiteLLM 只读取聚合账单字段，未读取 prompt、response、API key 或身份字段：

- 历史累计：8,362 次调用，19,012,122 tokens，网关记录成本 3.26477648。
- 本轮 164 条评测增量：296 次调用，392,519 tokens，成本 0.12571622。
- 平均每用例约 2,393 tokens、成本 0.00076656；门控拒绝的 17 条没有进入生成与裁判。

## 4. 监控与告警

- `promtool check rules`：通过，2 个 rule group、10 条规则。
- `/metrics`：数据库 ready、连接池、RAG 无证据、reranker 失败、SSE incomplete、索引积压/租约、scheduler 失败等在线指标均可抓取。
- 当前状态：`cortex_database_ready=1`，知识索引 queued/running/failed 均为 0。
- 未闭环：Compose 没有 Prometheus/Alertmanager 采集与通知路由；磁盘告警依赖 node exporter，备份/恢复告警依赖的时间戳指标也没有生产者。因此本轮只能验收规则语法和应用指标契约，不能宣称告警送达通过。

## 5. 数据库、备份与恢复

- 当前迁移版本：36。
- Public tables：58。
- RLS / FORCE RLS：48 / 48。
- Compose 的 db、redis、llm-gateway、embedding、reranker、backend 均 healthy；`/readyz` 通过。
- 非 AI smoke：通过；AI acceptance：通过。

迁移 36 后的联合备份：

| 项目 | 结果 |
|---|---:|
| 数据库备份 | 52,963,240 B |
| 应用数据备份 | 111,923,632 B |
| 备份耗时 | 69.352 s |
| 隔离恢复 RTO | 150.003 s |
| 观测 RPO | 12 s |
| 缺失文件 / 孤儿文件 | 0 / 0 |
| 恢复后非 AI smoke | 通过 |

备份保存在本机忽略目录 `.tmp-acceptance-current-backup-20260824-130644`，未包含 `.env` 或供应商密钥。

## 6. 验收结论

- 通过：冻结集 RAG 全链路、分层报告、SQL 容量、网关成本聚合、规则语法、应用指标、当前迁移、RLS、非 AI/AI smoke、联合备份和隔离恢复。
- 未通过：真实用户 bad case 库（当前无反馈库存）、Prometheus 实际采集、Alertmanager 通知送达、目标生产规格的 HTTP/Embedding/Reranker/LLM 并发饱和测试。
- 在上述未通过项补证前，本报告只能作为本地目标环境验收证据，不能作为生产发布签字或 SLA。
