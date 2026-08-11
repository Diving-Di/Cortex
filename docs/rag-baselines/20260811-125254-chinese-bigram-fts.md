# Unicode 2-gram 中文 FTS 新基线

- 评测时间：2026-08-11 12:52:54 UTC（20:52:54 +08:00）
- 基础提交：`a3b59a804e49bc10b8ca510e90c9d7355dd57437`，包含未提交的本次实现改动
- 数据集：`backend/testdata/rag/knowledge_eval_merged.jsonl`
- 数据集 SHA-256：`d2b9e87ee9285832f1206b0330aa0fa729e28a1eb472d71f09b3413f9b1c7dec`
- 数据集组成：164 条（90 条历史菜谱、74 条非菜谱）
- 原始产物：`artifacts/rag-eval/context-top4/20260811-125254`
- 参数：Vector TopK=15、Fulltext TopK=5、Title TopK=10、Fusion TopK=20、Context TopK=4
- Embedding：`iic/nlp_gte_sentence-embedding_chinese-small`
- Reranker：`BAAI/bge-reranker-v2-m3`

## 全链路指标

| 指标 | 新基线 |
|---|---:|
| Hit@1 | 0.993902 |
| Hit@3 / Hit@5 / Hit@10 | 1.000000 |
| MRR（rerank 前） | 0.943462 |
| MRR（rerank 后） | 0.996951 |
| Context Recall | 0.857885 |
| Context Precision | 0.864668 |
| Faithfulness | 0.966236 |
| Answer Relevancy | 0.939085 |
| Retrieval p50 / p95 | 182 ms / 336 ms |
| Total p50 / p95 | 6,533 ms / 9,347 ms |

自动门禁全部通过：Hit@10 ≥ 0.99、Context Recall ≥ 0.80、Context Precision ≥ 0.85、
Faithfulness ≥ 0.90、Answer Relevancy ≥ 0.88、失败数为 0。

## 数据集分层

| 子集 | 数量 | Hit@10 | Context Recall | Context Precision | Faithfulness | Answer Relevancy |
|---|---:|---:|---:|---:|---:|---:|
| 历史菜谱 | 90 | 1.0000 | 0.8614 | 0.8343 | 0.9800 | 0.9461 |
| 非菜谱 | 74 | 1.0000 | 0.8536 | 0.9017 | 0.9495 | 0.9305 |

## 检索消融

最终参数消融产物位于 `artifacts/rag-eval/ablations/20260811-205502`，Vector-only 来自
`20260811-195201`。标题局部相似度增强后的 Title-only 产物位于 `20260811-211212`，融合复测
位于 `20260811-211317` 和 `20260811-211805`。早期 Title-only 汇总受旧分母缺陷影响，应忽略。

| 路由 | Hit@1 | Hit@10 | MRR（rerank 后） | 结论 |
|---|---:|---:|---:|---|
| Vector only | 0.9451 | 0.9939 | 0.9660 | 漏掉 `recipe-010` 蛋炒饭精确材料问题 |
| Fulltext only（TopK=5） | 0.9573 | 0.9634 | 0.9604 | 从旧基线 0 命中提升到 158/164 |
| Title only（局部相似度增强） | 0.2927 | 0.2927 | 0.2927 | 48/164；其余 116 条无候选，没有错误标题候选 |
| Vector + Fulltext（Fulltext TopK=5） | 0.9939 | 1.0000 | 0.9970 | 补回 Vector 唯一漏例，实际增量 1/164 |
| Vector + Title | 0.9695 | 0.9939 | 0.9802 | 标题与向量覆盖重叠，Hit@10 无独立增量 |
| Fulltext + Title | 0.9573 | 0.9634 | 0.9604 | 标题与全文覆盖重叠，Hit@10 无独立增量 |
| Vector + Fulltext + Title | 0.9939 | 1.0000 | 0.9970 | 保持完整基线，无回退 |

现有 `route_metrics.fulltext_incremental` 衡量的是融合候选 Top10 中“仅 Fulltext provenance”的
gold，并不等价于路由消融增量；本次 Fulltext 的真实增量来自加入该路由后 RRF + rerank 将
`recipe-010` 从 Vector-only 未命中提升为 Top1。因此简历或报告应引用消融矩阵的 1/164，而不能
把 provenance 字段中的 0 解释为“没有增量”。

## 已知局限

- 菜谱子集 Context Precision 为 0.8343，低于非菜谱子集；整体门禁通过，但仍应继续分析同菜系、
  同食材文档之间的噪声。
- 164 条数据包含 90 条历史菜谱，不能包装成完全通用的真实用户分布。
- Title 通道通过局部 `word_similarity >= 0.45` 后可独立命中 48/164，且本轮没有错误标题候选；
  但这些命中均被向量或全文通道覆盖，融合消融的 Hit@10 独立增量仍为 0。可以说三路均能召回
  gold，不能说标题通道在当前数据集上带来独立增量。
- LLM Judge 指标存在非确定性，检索 Hit@K、MRR 和消融增量是更稳定的改造证据。
