# Cortex 个人知识库 RAG 当前链路

> 校对日期：2026-09-07。API 契约见 [api.md](api.md)，历史参数消融和质量数字见 [冻结基线](rag-baselines/README.md) 与 [旧链路快照](archive/RAG_DESIGN_20260825.md)。

## 范围与来源

知识问答入口为 `POST /api/v1/knowledge/chat/stream`，只使用当前租户启用的上传资料与开启知识索引的笔记。
上传支持 Markdown/ZIP、PDF、DOC/DOCX、PNG/JPG/WebP；二进制解析/OCR 由隔离的 document-parser 处理。
文档 ready、未删除、knowledge_enabled、活动索引版本和原笔记有效性由 PostgreSQL 决定。
旧菜谱 API 已移除；评测资源中保留 recipe 命名不代表仍有内置菜谱产品。

## 摄取与索引

Markdown 按标题章节构造 parent，跳过仅标题而无有效正文的章节；child 上限为 500 个 Unicode 字符。
Embedding 输入包含标题、来源、章节与清洗后的正文，模型输出 512 维向量。

Kafka 模式依次执行解析/切块、Embedding、ES 投影；PostgreSQL 先原子写入并激活索引，随后构建 ES 投影。
PostgreSQL 模式由索引 runner 轮询任务。索引阶段和块进度受租约 owner 保护；重建失败保留旧活动版本。
详见 [基础设施实现](INFRASTRUCTURE_EVOLUTION.md)。

## 查询流程

1. 认证后绑定 Principal、会话、request ID 与集合范围，检查幂等和并发状态。
2. 结合最近成功问答历史消解指代、改写检索 query；原问题保留给生成。历史不作为新的已验证来源。
3. 生成查询向量，按服务端配置选择检索 backend。
4. 对候选执行 PostgreSQL 租户/活动版本校验与 parent 聚合，使用 BGE CrossEncoder 精排。
5. 按证据分数、数量和可选 margin 门控，并优先选文档再选章节。
6. 弱证据分为 ambiguous、scope_conflict、absent。前两类可进入一次性澄清；无证据拒答。
7. 生成后执行引用与证据核验，来源再次校验后持久化当前租户的回答与来源。

| 配置 | 当前行为 |
| --- | --- |
| `RAG_RETRIEVAL_BACKEND=elasticsearch` | Compose 默认；ES BM25 + 512 维 KNN，可重建投影 |
| `RAG_RETRIEVAL_BACKEND=postgres` | 直启默认；全局向量、中文 2-gram、标题匹配文档内向量三路召回与 RRF |
| `RAG_VECTOR_TOP_K` / `RAG_TITLE_TOP_K` / `RAG_KEYWORD_TOP_K` | config 默认 15 / 10 / 5 |
| `RAG_FUSION_TOP_K` / `RAG_CONTEXT_PARENT_TOP_K` | config 默认 20 / 4 |
| `RAG_RERANK_MIN_SCORE` / `RAG_RERANK_MIN_MARGIN` | 可配置证据门槛，随模型和数据集校准 |
| `RAG_PLANNER_ENABLED` | 默认 false；比较、趋势、跨周期规则计划器需单独对照评测 |

Elasticsearch 故障当前返回稳定的检索不可用错误；没有自动切换 pgvector 的实现。
两种 backend 的质量指标必须分别记录，不能将 PostgreSQL 历史 Hit@K 当成当前 ES 结果。
Reranker 故障不能跳过精排直接生成；无证据返回 `KNOWLEDGE_NO_EVIDENCE`。

## 会话、SSE 与反馈

公开 `retrieval_progress`（schema_version=1）仅展示阶段和聚合统计，不携带 prompt、正文块、身份或内部地址。
流已输出内容后不得从头自动重试。断流、取消、来源失效与 request ID 重放遵循 [API 契约](api.md)。

澄清绑定租户、用户、会话、原 request ID 与服务端集合范围，15 分钟内只允许补充一次。
当前已有反馈、用户复核晋升、数据集冻结与脱敏 trace；不能再将“无反馈 API”作为现状。
反馈晋升须用户明确提交最小必要 query、期望答案与证据 hash，私人正文不自动进入仓库。

## 验证与证据

- `backend/internal/store/knowledge_*integration_test.go`：索引事务原子性、活动版本和旧 owner fencing。
- `backend/internal/server/knowledge_*test.go`：会话、SSE、证据决策、计划器、澄清和反馈。
- `go run ./cmd/rag-regression-check`：公开合成 fixture 的 schema、唯一性与 hash 完整性。
- `cmd/rag-eval`：受控环境真实检索/生成/Judge，前置条件是评测账号和授权语料已入库。

```powershell
# 仓库根目录
./backend/scripts/local_integration_test.ps1
# backend 目录
go run ./cmd/rag-regression-check
./scripts/rag_eval.ps1 -Workers 4
```

确定性 fixture 检查不等于真实 RAG 质量验收。离线 runner 与线上问答在历史改写、证据门控、核验等步骤上可能不同，
运行报告应注明 backend、模型、数据集、参数、索引版本、调用链和适用范围。

原始评测产物写入本地 `artifacts/rag-eval/` 并被 Git 忽略；只有脱敏的日期报告和公开冻结夹具进入仓库。
当前质量历史分别见 [2026-08-11 PostgreSQL 基线](rag-baselines/20260811-125254-chinese-bigram-fts.md)
和 [2026-08-25 复验](operations/RAG_AND_K6_RERUN_20260825.md)，后续模型/索引/检索变更必须重新评测。
