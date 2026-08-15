# SuperMew 项目借鉴分析报告

> 分析对象：`E:\Codebase\SuperMew`  
> 上游仓库：<https://github.com/icey1287/SuperMew.git>  
> 分析基线：提交 `f997821db835c9c080accaf1f78c2c4a602fe4b7`（2026-07-11）  
> 分析日期：2026-08-13  
> 对照项目：Cortex

## 1. 结论摘要

SuperMew 是一个以知识库问答为核心的 Python/FastAPI + Vue 3 + Milvus RAG 应用。它最值得 Cortex 借鉴的不是技术栈或存储架构，而是将 RAG 的内部决策转换为可理解、可继续交互的产品体验：

1. **检索过程可视化**：用结构化 SSE 事件展示检索、重写、精排、子问题与耗时，并在回答后提供可折叠的证据详情。
2. **低置信度 HITL 澄清与恢复**：证据不足或问题范围不清时，不立即终止，而是向用户提问；收到补充信息后做针对性检索并继续原会话。
3. **复杂问题分解与并行检索**：先进行复杂度判断，复杂问题拆成多个子问题并汇总证据，同时为简单问题设置快速路径。
4. **明确的延迟与降级护栏**：限制重写次数、知识工具调用次数和 rerank 等待时间，并用测试锁定短路行为。
5. **面向用户的索引步骤反馈**：上传处理被拆成“保存、清理、解析、父块入库、向量入库”等阶段，便于定位长任务卡点。

Cortex 已经具备更可靠的多租户 RLS、PostgreSQL 权威数据源、持久化索引任务、索引版本切换、多路 RRF、parent/child 检索、rerank、引用校验、脱敏 trace 和质量反馈链路。因此建议吸收 SuperMew 的**交互协议、状态模型和测试思路**，而不是迁移到 LangGraph、Milvus 或 Python。

## 2. 分析范围与方法

本次分析基于实际源码而非仅依据 README，重点检查了：

- RAG 编排：`backend/rag/pipeline.py`、`backend/rag/utils.py`
- 流式会话与请求隔离：`backend/chat/service.py`、`backend/chat/request_context.py`
- 索引与分块：`backend/indexing/document_loader.py`、`milvus_client.py`、`parent_chunk_store.py`
- 上传任务：`backend/api/routes/documents.py`、`backend/jobs/upload_jobs.py`
- 前端体验：`ThinkingTrace.vue`、`RetrievalTraceDetails.vue`、`References.vue`、`KnowledgeContextPanel.vue`、`stores/chat.ts`
- 自动化测试：`tests/test_chat_hitl_resume.py`、`test_rag_latency_guards.py`、`test_rag_short_circuit.py`、`test_request_context_di.py` 及前端 store 测试
- 安全与部署：认证、数据模型、Compose 与环境变量示例

同时与 Cortex 当前知识库链路进行了代码级对照，包括 `backend/internal/server/knowledge.go`、`backend/internal/store/knowledge_retrieval.go`、`backend/internal/knowledge/chunk.go`、知识索引 worker 和知识库前端页面。

## 3. SuperMew 架构概览

SuperMew 的主链路可以概括为：

```text
Vue 聊天界面
  -> FastAPI SSE
  -> Agent / 知识工具
  -> LangGraph RAG
       -> 复杂度分类
       -> 简单问题直接检索 / 复杂问题拆分并行检索
       -> Milvus 稠密向量 + 原生 BM25 + RRF
       -> 三级分块 Auto-merging
       -> 可选 rerank
       -> 证据评分与路由
       -> 回答 / 重写 / 澄清 / 范围选择 / 无知识
  -> PostgreSQL 保存会话与 RAG trace
```

文档使用三级块结构：L3 叶子块用于召回，多个同父叶子命中后向 L2/L1 合并。Milvus 保存叶子块及稀疏/稠密检索字段，PostgreSQL 保存父块。前端消费内容、检索步骤、完整 trace、HITL 请求和完成标记等 SSE 消息。

## 4. 值得借鉴的部分

### 4.1 P0：低置信度澄清与会话恢复

SuperMew 将证据判断结果路由为 `answer`、`rewrite`、`clarify`、`scope_select` 或 `no_knowledge`。其中 `clarify` 和 `scope_select` 会生成提示与候选选项，并保存最小恢复状态；用户回复后，系统组合原问题与补充信息，执行一次针对性检索，再继续生成回答。

值得借鉴的原因：

- Cortex 当前已通过候选数量和 rerank margin 拒绝低质量证据，这保障了可信度，但用户只能得到“无依据”结果。
- 对“问题本身含糊”与“知识库确实没有内容”进行区分，能减少本可挽救的拒答。
- 恢复状态让用户无需重复问题，也避免把不完整证据硬塞给模型。

建议在 Cortex 中实现为业务层显式状态机，不引入 LangGraph：

```text
retrieved -> sufficient -> generating
          -> ambiguous -> awaiting_clarification -> targeted_retrieval
          -> scope_conflict -> awaiting_scope_selection -> targeted_retrieval
          -> absent -> refused
```

建议新增命名 SSE 事件：

- `clarification_required`：仅返回服务端生成的 request ID、问题、有限候选项和原因枚举。
- `retrieval_progress`：恢复检索的阶段与安全摘要。
- 复用现有 `sources`、`error` 与完成事件，不发送原始 prompt、完整块正文、内部地址或模型响应。

恢复状态应持久化在当前租户、用户和会话下，设置过期时间并保证 request ID 幂等；客户端不得提交或覆盖 `tenant_id`。补充回答只应触发一次有限的针对性检索，避免无限澄清循环和配额绕过。

### 4.2 P0：面向用户的检索轨迹

SuperMew 的前端将实时步骤与最终 trace 分层展示：流式阶段显示“正在检索/合并/精排”等步骤及耗时，完成后可以展开查看检索模式、重写方式、候选数量、合并情况、精排分数和证据片段。复杂问题的子步骤会折叠分组，避免信息噪声。

Cortex 已保存脱敏 trace 并已发出 `retrieval`、`sources` 等 SSE 事件，后端基础比 SuperMew 更完整，但当前知识库页面还没有形成相应的会话与可视化体验。

建议：

- 将内部 trace 映射为版本化的公开 DTO，前端不得直接依赖数据库 trace JSON。
- 默认只显示用户能理解的阶段、耗时、来源数和结果状态；调试细节放在折叠面板。
- 不展示思维链、完整正文、原始 prompt、上游错误正文、内部 URL、模型密钥或可跨租户定位的标识。
- 证据卡片显示引用编号、标题、章节路径、来源类型和安全摘要；点击引用时定位对应卡片。
- 复杂检索按子问题分组，并明确标记“检索计划”而非“模型思维过程”。

建议事件结构示例：

```json
{
  "event": "retrieval_progress",
  "data": {
    "schema_version": 1,
    "stage": "rerank",
    "status": "completed",
    "elapsed_ms": 84,
    "candidate_count": 12,
    "qualified_count": 4
  }
}
```

### 4.3 P1：复杂问题分解，但先以受控实验落地

SuperMew 会用低延迟模型判断问题复杂度；复杂问题被拆分为多个子问题，通过 LangGraph fan-out 并行执行检索，再去重、汇总证据和状态。简单问题存在规则快速路径，避免每次调用规划模型。

该思路适用于跨日期、跨主题、比较型和趋势型的个人知识问答，例如“比较过去三个月工作重心的变化并给出证据”。但它也会增加成本、延迟和错误组合风险。

建议 Cortex 不直接加入通用 Agent 图，而是：

1. 先限定到明确的复杂查询类型，如比较、趋势、跨周期汇总。
2. 规则快速路径优先；仅在必要时调用 `cortex-default` 生成结构化子查询。
3. 子问题数量硬限制为 2–4 个，并设置总 token、总检索次数和总时限。
4. 每个子问题仍通过可信 Principal 与同一事务级 RLS 约束检索。
5. 合成前去重来源，最终答案逐项校验引用，部分证据只能生成明确标注的部分回答。
6. 先通过现有反馈/评测集与消融脚本验证，再逐步开放。

### 4.4 P1：延迟预算、调用预算和短路测试

SuperMew 对 rerank 设置明确超时，限制重写次数，并通过请求上下文限制知识工具调用；测试覆盖简单问题快速路径、无证据短路、重写后停止、rerank 超时和并发请求上下文隔离。

建议把这一思路扩展到 Cortex 已有链路：

- 为 rewrite、embedding、三路召回、rerank、引用验证和生成分别记录阶段耗时与结果枚举。
- 设置单阶段和全请求 deadline；客户端取消后停止后续工作。
- 生成已经输出内容后不得从头重试，保持现有 AI 网关安全边界。
- 为澄清、重写、子问题和针对性检索设置每请求硬上限。
- 增加确定性测试：无合格证据不得调用生成；简单问题不得调用规划；超时按稳定错误码降级；并发请求的 trace 与事件不得串线。

### 4.5 P1：索引任务阶段可见性

SuperMew 的上传 UI 展示各处理步骤和块进度，这种“长任务可解释性”值得借鉴。不过它的任务状态仅在进程内存中，不能直接采用。

Cortex 已有 PostgreSQL 持久化索引任务、租约、重试和 active index version，可靠性更高。建议只补充：

- 为持久化 job 增加稳定阶段枚举，如 `extracting`、`chunking`、`embedding`、`writing`、`activating`。
- 保存安全的进度计数与失败码，不保存完整正文或上游响应。
- 前端在文档列表中展示阶段、已处理/总块数、重试状态和“旧版本仍可用”提示。
- 多实例进度更新继续走数据库 claim/lease，不使用进程内 map。

### 4.6 P2：查询重写策略作为评测候选

SuperMew 在一次初检失败后，在 Step-back 与 HyDE 中选择一种重写方法，并限制最多重写一次。这一设计比无边界反复检索更稳健。

Cortex 已有查询重写。建议不要凭经验直接切换策略，而是在已有离线评测集上加入消融组：原查询、当前 rewrite、Step-back、HyDE。比较 Hit@K、MRR、Context Recall/Precision、引用通过率、P95 延迟和 token 成本。只有在中文个人笔记语料上有稳定收益时再上线，并记录 prompt/rewrite 版本。

### 4.7 P2：三级分块与 Auto-merging 仅作为实验项

SuperMew 的 L1/L2/L3 分块和命中阈值式向上合并，适合长 PDF、Word 和 Excel 文档。Cortex 已经采用 parent/child：child 多路召回后在 parent 级聚合并生成，因此核心思想已经覆盖。

可借鉴的是实验方向，而不是数据模型替换：

- 对超长 Markdown 文档评估可选的 section/subsection/leaf 三层结构。
- 比较当前 parent 聚合与“同一 parent 多个 child 命中后扩大上下文”的策略。
- 严格控制上下文 token 膨胀，使用评测集验证完整性收益。
- 若实施，必须新增版本化迁移、chunker version 和重建索引流程；不能用启动时 DDL。

## 5. 不应照搬的部分

| SuperMew 做法 | 风险或不适配原因 | Cortex 应保持的方案 |
|---|---|---|
| Python/FastAPI、SQLAlchemy、LangGraph | 违反 Cortex 固定 Go/Gin/pgx 架构边界，并增加另一套运行时 | 保持 `backend/cmd/server/main.go` 唯一入口，用显式 Go 状态机编排 |
| Milvus + etcd + MinIO | 增加三类基础设施与运维面；Cortex 当前 PostgreSQL pgvector/FTS 已有多路检索 | 继续以 PostgreSQL 16 为统一数据与检索底座，先用消融证明扩容必要性 |
| 任务状态保存在进程内 map | 重启丢失，不支持多实例一致 claim | 保持数据库 job、`FOR UPDATE SKIP LOCKED` 与有限租约 |
| JWT 有默认回退密钥且仅自包含校验 | 默认密钥存在安全风险，难以支持服务端撤销和最后使用时间 | 保持 Token 摘要持久化、过期、撤销与软删除租户校验 |
| 业务数据仅按 user 外键隔离，无 RLS | 不满足 Cortex 租户安全边界 | 所有租户查询继续使用 `Store.WithTx`、transaction-local RLS 与显式 tenant 条件 |
| 上传文件按原文件名直接落盘 | 存在覆盖、路径穿越、配额与隔离风险 | 保持 `CORTEX_DATA_DIR` 下安全相对路径、大小/配额/压缩比校验 |
| API 错误直接拼接异常字符串 | 可能泄漏内部路径、服务地址和上游响应 | 保持稳定 `code/message/details`，日志和响应均脱敏 |
| Compose 暴露 PostgreSQL、Redis、Milvus、MinIO 等端口 | 扩大攻击面，与 Cortex 部署规范冲突 | `db`、`redis`、`llm-gateway` 不暴露宿主端口，后端降权运行 |
| Agent 可选用通用外部模型配置 | 可能绕过 LiteLLM 网关和元数据限制 | 所有模型流继续只经 LiteLLM，后端仅持有虚拟密钥和逻辑模型 |
| 将详细 RAG trace 直接随会话返回 | 可能暴露块正文、内部端点、模型与调试信息 | 公开 DTO 白名单化，完整脱敏 trace 仅供租户内审计与评测 |

## 6. Cortex 当前能力对照

| 能力 | SuperMew | Cortex 当前状态 | 判断 |
|---|---|---|---|
| 混合召回 | Milvus dense + BM25 + RRF | PostgreSQL vector + FTS + title 三路 child RRF | Cortex 已覆盖且更易统一治理 |
| parent/child 上下文 | 三级块 + Auto-merging | child 召回、parent 聚合与上下文选择 | 核心能力已覆盖，三级结构仅需实验 |
| rerank | 可选，带超时与分数阈值 | 本地 rerank + margin 拒答 | Cortex 更接近可信生产方案，可补阶段可观测性 |
| 引用 | 展示检索片段 | 保存租户来源并验证引用 | Cortex 更严格 |
| RAG trace | 会话 JSON + 前端详情面板 | 脱敏持久化 trace，用户侧展示较弱 | 应借鉴前端与公开事件层 |
| 用户反馈 | 未见完整质量闭环 | 五类反馈、评测集晋升/冻结正在建设 | Cortex 更完整 |
| HITL | 澄清/范围选择/恢复检索 | 暂无同等交互闭环 | 最优先借鉴 |
| 复杂问题分解 | 分类、并行子问题、合成 | 暂无通用分解 | 受控实验后引入 |
| 索引任务 | 有步骤 UI，但状态在内存 | 持久化 job、租约、重试、索引版本 | 仅借鉴展示粒度 |
| 多租户安全 | 基础 user 隔离 | tenant + RLS + 显式条件 | 不可向 SuperMew 退化 |

## 7. 推荐实施路线图

### Phase 1：检索体验与可观测性（P0）

- 定义公开的 `retrieval_progress` DTO 与 schema version。
- 后端在 rewrite、retrieve、rerank、evidence gate、generation 阶段发送白名单事件。
- 前端新增折叠式检索进度、来源卡片和引用定位。
- 增加取消、断流、不完整回答和并发事件隔离测试。

验收标准：普通日志和 SSE 中无正文、密钥、内部 URL 或上游响应；同一 request ID 的阶段单调推进；跨租户访问仍统一 404；简单知识问答的首 token 延迟无显著回退。

### Phase 2：澄清与范围选择（P0）

- 定义 `ambiguous`、`scope_conflict`、`absent` 三类证据门控结果。
- 持久化短期 clarification state，并新增确认/恢复接口。
- 前端渲染自由输入和有限选项，恢复后仍展示原问题关联。
- 对过期、重复提交、跨用户、跨租户、循环澄清和配额进行测试。

验收标准：无证据仍返回 `KNOWLEDGE_NO_EVIDENCE`；只有可澄清的歧义进入 HITL；恢复检索最多一次；补充内容不能改变服务端租户和 collection scope。

### Phase 3：复杂问题计划器实验（P1）

- 建立包含跨周期、比较、趋势和多条件问题的冻结评测集。
- 实现规则快速路径与最多 2–4 个结构化子查询。
- 并行检索共享总 deadline 与配额，逐子问题保存脱敏 trace。
- 通过 feature flag 小流量开启，并与单查询基线做消融。

验收标准：引用通过率不下降；质量指标有显著收益；P95 延迟与 token 成本处于预设预算；任何子查询都保留相同 Principal/RLS 上下文。

### Phase 4：分块与重写优化实验（P2）

- 比较当前重写、Step-back 与 HyDE。
- 比较当前 parent 聚合与阈值式上下文扩展。
- 只基于冻结评测集和真实反馈晋升结果决定是否上线。

## 8. 建议新增的测试矩阵

1. **状态机**：充分证据直接回答；歧义进入澄清；范围冲突进入选择；无知识稳定拒答。
2. **恢复**：正常恢复、重复恢复、过期恢复、空答案、恶意超长答案、跨会话/跨租户恢复。
3. **预算**：简单问题不调用计划器；重写不超过一次；澄清不循环；总检索次数和总时限可控。
4. **流式**：首个内容前可发送进度；内容输出后上游失败标记 incomplete 且不重试；取消请求停止后续生成。
5. **并发**：request ID、trace、来源与阶段事件不串线。
6. **安全**：公开 trace 不含正文、邮箱、姓名、内部地址、模型密钥或上游响应；客户端 tenant ID 无效。
7. **质量**：Hit@K、MRR、Context Recall/Precision、引用通过率、无证据率、澄清挽救率、P50/P95 延迟和 token 成本。

## 9. 最终建议

建议将 SuperMew 视为 **RAG 交互体验和实验策略的参考实现**，而非 Cortex 的替代架构。近期最有投入产出比的工作是：

1. 先让 Cortex 已经具备的检索能力以安全、结构化的方式呈现给用户。
2. 将当前低证据拒答细分为“可澄清”与“确实无知识”，建立一次性恢复闭环。
3. 在现有质量反馈和评测集基础上验证复杂问题分解、Step-back/HyDE 和上下文扩展，而不是直接上线。

这样既能获得 SuperMew 最成熟的产品体验，又不会牺牲 Cortex 已建立的多租户安全、可靠任务、引用验证和单一 PostgreSQL 运维边界。
