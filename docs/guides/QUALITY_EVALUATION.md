# AI 质量回归与实验对照

校对日期：2026-10-05。复用 Go `cmd/rag-eval`、`internal/rageval` 和现有 PowerShell runner；不增加另一套后端。

## 回归顺序

1. 在 backend 目录执行 `go run ./cmd/rag-regression-check`，验证公开夹具结构、唯一性和证据 hash。
2. 确认评测账号及授权语料已经入库，固定数据集版本、文件 SHA-256、模型 revision、阈值与索引版本。
3. 在 backend 目录使用 `./scripts/rag_eval.ps1` 做当前检索配置评测；需要真实生成/Judge 时采用 `./scripts/run_full_eval.ps1`。
4. 在 backend 目录使用 `./scripts/run_retrieval_ablation.ps1` 对照向量、全文、标题及其组合。该脚本只做检索/精排，不证明生成质量。
5. 将当前结果与相同配置的冻结基线比较，记录退化案例，再决定是否调整参数或打开实验开关。

## 每次报告要说明什么

| 维度 | 记录内容 |
| --- | --- |
| 可复现性 | commit/工作树状态、环境、数据集 hash、账号授权、索引版本、backend、模型和全部非默认参数 |
| 检索 | Hit@K、MRR、Context Recall/Precision；明确 runner 实际输出的指标和未采集项 |
| 回答 | 引用通过率、拒答准确率、无来源报告拒绝、歧义与范围冲突恢复 |
| 性能 | P50/P95、并发、阶段耗时、调用次数；区分单查询与复杂问题 |
| 用量 | 输入/输出 Token、供应商价格依据和估算方法；应用字符估算不能当作准确账单 |
| 降级 | 模型/精排/搜索不可用，流式中断，生成失败后保留上次完整结果 |

当前离线 runner 与线上知识问答链路可能不同，结果要注明是否覆盖历史改写、证据门控、核验与澄清。
PostgreSQL 和 Elasticsearch 的结果分开保存，不能引用历史 PostgreSQL 数据证明当前 ES 质量。
没有真实模型结果时记录“未测”，不填零或推断通过。

## 反馈与隐私

用户在知识问答提交 `incorrect_answer` 或 `unsupported_citation` 后，只有明确复核并脱敏的案例才能晋升评测集。
复用现有反馈晋升和数据集冻结接口，保留最小必要问题、期望答案与证据 hash；私人正文不自动进入仓库。
原始日志和评测结果写入被忽略的 `artifacts/rag-eval/`，脱敏结论放到 `docs/operations/`。

`RAG_PLANNER_ENABLED` 保持默认关闭。Step-back、HyDE、Auto-merging 等方案先离线消融，
同时达到质量、延迟和调用成本目标后再评审上线。历史基线索引见[冻结基线](../rag-baselines/README.md)。

## 自动比较既有产物

回到仓库根目录，使用 `python scripts/compare_rag_runs.py <基线目录> <当前目录> --output artifacts/rag-eval/comparison.md`。
目录需包含 Go runner 的 config.json、summary.json 和 cases.jsonl。脚本对案例内容计算冻结指纹，
拒绝不同案例集、重复 ID、样本数不匹配或 retrieval-only/full-generation 混比；不输出私人问题和答案。
默认质量指标允许下降 0.02，P95 最多增长到 1.25 倍，失败案例不得增加，超限退出码为 1，产物无效为 2。
可通过 quality-tolerance / max-latency-ratio 明确调整门槛；参数/模型变化仅列字段名供评审。
输出只包含 hash、聚合指标、配置变化字段和未通过项，并明确 Token/调用次数/成本未完整采集。
回归测试：`python -m unittest discover -s scripts -p test_compare_rag_runs.py`。
