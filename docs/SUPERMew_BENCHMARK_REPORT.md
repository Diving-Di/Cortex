# SuperMew 与 Cortex 重新对比分析

> 分析对象：`E:\Codebase\SuperMew` 与 `E:\Codebase\Cortex`
>
> SuperMew 基线：`f997821db835c9c080accaf1f78c2c4a602fe4b7`（2026-07-11）
>
> Cortex 基线：`0d76486`（2026-08-15）及 2026-08-19 当前工作树
>
> 重新分析日期：2026-08-19
>
> 方法：以源码、测试、Compose 和当前界面实现为准，不只依据 README

## 1. 结论

旧报告的方向仍然成立，但优先级需要收敛。

SuperMew 自旧报告使用的 `f997821` 基线后没有新提交；变化主要来自 Cortex。Cortex 现在已经明确具备会话持久化、请求 ID 幂等回放、流中断结果保存、来源校验、质量反馈与评测集晋升、中文 bigram FTS、索引版本切换和更完整的生产运维能力。因此，SuperMew 不再适合作为 Cortex 的整体架构参考，更适合作为三类局部设计参考：

1. **可恢复的低置信度交互**：把证据不足细分为歧义、范围冲突和确实无知识，并允许用户补充一次后继续原请求。
2. **用户可理解的 RAG 过程展示**：实时展示检索、重写、精排、证据门控和耗时，完成后提供折叠式来源详情。
3. **受预算约束的复杂问题检索**：简单问题走快速路径；比较、趋势和跨周期问题才进行有限分解与并行召回。

另有两个较小但现实的借鉴点：SuperMew 的 PDF/Word/Excel 摄取能力，以及上传任务的分步骤进度设计。

不应借鉴其 Python/LangGraph/Milvus 技术栈、进程内任务状态、原文件名落盘、默认 JWT 密钥、自包含 Token、直接返回异常字符串或较弱的数据隔离方式。这些会破坏 Cortex 已建立的架构和安全边界。

## 2. 项目定位不同

| 维度 | SuperMew | Cortex | 结论 |
|---|---|---|---|
| 产品中心 | 文档知识库聊天 | 个人笔记、周期报告、知识库、研究、模板与导出 | 不能按功能数量直接比较 |
| AI 形态 | Agent + RAG 是主产品 | AI 是可选增强，非 AI 功能独立可用 | 保持可降级边界 |
| 数据权威源 | PostgreSQL + Milvus + 本地文件分工 | PostgreSQL 是正文唯一权威源 | 不迁移到多权威存储 |
| 典型用户 | 管理员维护共享文档库，用户聊天 | 每账号独立个人租户 | 共享知识库假设不适用 |
| 文档类型 | PDF、Word、Excel | Markdown、Markdown ZIP、笔记 | Office/PDF 支持可独立评估 |

SuperMew 的优势集中在“知识库问答这一条链路的产品化深度”；Cortex 的优势是更完整的个人知识产品、更强的数据治理和生产可靠性。合理做法是移植交互模型和实验方法，而不是移植架构。

## 3. 当前架构对照

### 3.1 SuperMew

```text
Vue 3 聊天界面 -> FastAPI SSE -> Agent / knowledge tool -> LangGraph RAG
  -> 复杂度判断与简单问题快速路径
  -> 复杂问题分解和并行子检索
  -> Milvus dense + 原生 BM25 + RRF
  -> L1/L2/L3 Auto-merging -> rerank
  -> answer / rewrite / clarify / scope_select / no_knowledge
  -> PostgreSQL 保存用户、会话、消息、父块和 trace
```

### 3.2 Cortex

```text
React 18 -> Gin /api/v1 SSE -> 显式 Go 工作流
  -> 带会话上下文的查询重写
  -> PostgreSQL vector + 中文 FTS + title 多路召回
  -> RRF、parent 聚合、上下文选择
  -> 本地 rerank 与 margin 证据门控
  -> 基于来源的生成和引用校验
  -> PostgreSQL 保存会话、结果、来源、脱敏 trace、反馈和评测集
```

Cortex 的检索底座已经不弱于 SuperMew，而且在租户隔离、引用可信度、失败持久化和评测闭环上更完整。差距主要在用户交互层，不在召回组件数量。

## 4. 值得借鉴的部分

> 实施状态（2026-08-19）：4.1、4.2、4.4、4.5 已按 Cortex 的 Go/PostgreSQL/RLS 边界落地；
> 4.3 已作为默认关闭、最多 4 子查询的规则实验路径落地，仍需真实冻结集消融后才能启用；
> 4.7 继续沿用离线实验而不替换线上数据模型。按本轮范围，4.6 PDF/Word/Excel 摄取明确未实现。

### 4.1 P0：低置信度澄清与恢复

SuperMew 将证据决策路由为 `answer`、`rewrite`、`clarify`、`scope_select` 和 `no_knowledge`。`clarify`/`scope_select` 会持久化待恢复状态；用户补充后直接进行一次针对性检索，不重新进入完整 Agent 链路。相关行为有专门测试覆盖。

Cortex 当前会在候选不足或 rerank margin 过小时稳定返回 `KNOWLEDGE_NO_EVIDENCE`，但没有区分问题缺条件、多个范围冲突和知识确实不存在。

建议用 Go 业务状态机实现，不引入 LangGraph：

```text
retrieved -> sufficient -> generating
          -> ambiguous -> awaiting_clarification -> targeted_retrieval
          -> scope_conflict -> awaiting_scope_selection -> targeted_retrieval
          -> absent -> refused
```

实现边界：

- 待恢复状态绑定 tenant、user、conversation 和原 request ID，并设置过期时间。
- 客户端不能覆盖 `tenant_id`、collection scope 或原问题。
- 每个原请求最多恢复一次，且检索、token 和总时限有硬上限。
- 重复提交幂等回放；过期或跨租户访问表现为安全错误/404。
- 确实无知识时继续使用 `KNOWLEDGE_NO_EVIDENCE`。

### 4.2 P0：检索过程与来源的用户侧展示

SuperMew 的 `ThinkingTrace.vue` 和 `RetrievalTraceDetails.vue` 将实时步骤和最终 trace 分层展示。Cortex 后端已经发送 `retrieval`、`sources`、`done` 和带 `incomplete` 的 `error` 事件，并保存脱敏 trace；但当前 `KnowledgePage.tsx` 仍只是资料上传/列表页，`frontend/src/api/knowledge.ts` 也未暴露聊天流、会话、反馈或 trace 客户端。这是当前最高投入产出比的改进。

建议新增版本化公开事件 DTO，而不是把数据库 trace JSON 直接交给前端：

```json
{
  "schema_version": 1,
  "stage": "rerank",
  "status": "completed",
  "elapsed_ms": 84,
  "candidate_count": 12,
  "qualified_count": 4
}
```

界面建议分三层：默认展示用户能理解的阶段；完成后显示耗时、来源数、重写/降级和完整性；折叠详情显示引用、标题、章节路径、来源类型和安全摘要。

公开 DTO 不得包含思维链、完整块正文、原始 prompt、内部 URL、模型密钥、上游响应正文或可跨租户定位的标识。

### 4.3 P1：复杂问题分解与简单问题快速路径

SuperMew 对明显简单问题跳过复杂度模型；复杂问题拆成子问题后并行检索并合成。测试覆盖快速路径、部分子问题有证据、全部无知识和子问题触发 HITL。

该设计适合 Cortex 中的比较、趋势和跨周期问题，但不适合默认用于所有查询。建议先用规则识别明确类型，仅必要时让 `cortex-default` 生成 2～4 个结构化子查询。所有子查询继承同一 Principal、显式 tenant 条件和 RLS 上下文，并共享总 deadline、检索次数和 token 预算。用冻结评测集与单查询基线消融后再通过 feature flag 开放。

### 4.4 P1：延迟、调用与短路护栏

SuperMew 的测试特别值得借鉴：简单问题不调用复杂度模型，强证据初次评分后返回，弱证据最多重写一次，HyDE 只执行被选中的检索，HITL 恢复直接进入目标检索，请求上下文和工具计数不跨请求共享。

Cortex 应补齐这类“负行为断言”：无合格证据不得生成；简单问题不得规划；重写、恢复和子查询次数有上限；rerank 超时稳定降级；内容输出后只标记 incomplete、不从头重试；并发事件、trace、来源和对话不得串线。

### 4.5 P1：上传/索引任务的阶段反馈

SuperMew 把任务拆为上传、清理旧版本、解析与分块、父块入库、向量入库，并展示块级进度。Cortex 当前只展示 `uploaded/parsing/indexing/ready/failed/deleting` 和“旧版本可用”等粗状态。

只借鉴阶段和进度表达。SuperMew 的任务存在进程内字典，重启后丢失；Cortex 应继续使用持久化 job、claim/lease 和 active index version，只增加稳定阶段枚举、已处理/总块数、安全失败码、重试状态，以及多实例下单调幂等的进度更新。

### 4.6 P2：PDF、Word、Excel 摄取

SuperMew 已集成 PDF、DOCX、Excel 和加密 Office 文件相关解析依赖，Cortex 当前只接受 `.md`/`.zip`。若真实用户资料大量来自办公文档，这比更换向量库更可能提升采用率。

若纳入 Cortex，必须同时设计文件大小、页数、解压比、解析时间限制；隔离解析 worker；表格、标题层级和页码的可追溯来源；安全相对路径和配额；解析器/chunker 版本化。不能只放开扩展名。

### 4.7 P2：三级分块、Auto-merging 与重写策略只做实验

SuperMew 的 L1/L2/L3、Auto-merging、Step-back 和 HyDE 适合做实验候选。Cortex 已有 child 召回、parent 聚合和查询重写，核心思想已经覆盖。应在中文个人笔记冻结集上比较当前方案与候选方案的 Hit@K、MRR、Context Recall/Precision、引用通过率、P95 延迟和 token 成本，不直接替换数据模型。

## 5. Cortex 已经领先、无需跟随的部分

| 能力 | SuperMew | Cortex | 判断 |
|---|---|---|---|
| 多租户隔离 | user/role 和应用查询 | tenant + transaction-local RLS + 显式条件 | 保持 Cortex |
| Token | JWT 自包含校验，默认密钥回退 | SHA-256 摘要、过期、撤销、最后使用 | 保持 Cortex |
| 数据权威源 | PostgreSQL、Milvus、本地文件 | PostgreSQL 正文单一权威源 | 保持 Cortex |
| 索引任务 | 进程内状态 | 持久化、租约、重试、版本切换 | 保持 Cortex |
| 失败流 | 以聊天流为主 | incomplete、持久化、请求回放 | 保持 Cortex |
| 引用 | 展示召回片段 | 保存并校验当前租户来源 | 保持 Cortex |
| 质量治理 | RAG 单测 | 反馈、评测集晋升/冻结、基线 | 保持 Cortex |
| 中文搜索 | Milvus BM25 | 中文 bigram FTS + vector + title | 无需换库 |
| 运维 | 多基础设施 Compose | 健康检查、低权限角色、迁移和演练 | 保持 Cortex |
| AI 网关 | 可配置外部模型 | LiteLLM 逻辑模型和元数据边界 | 保持 Cortex |

## 6. 不应照搬的实现

| SuperMew 做法 | 风险 | Cortex 方案 |
|---|---|---|
| Python/FastAPI/SQLAlchemy/LangGraph | 违反固定技术栈，引入第二套运行时 | Go/Gin/pgx 与显式状态机 |
| Milvus + etcd + MinIO | 运维和一致性成本增加 | PostgreSQL 16 + pgvector/FTS |
| job 存进程内 map | 重启丢失，多实例不一致 | 数据库 job + lease/claim |
| 原文件名直接拼上传目录 | 覆盖和路径穿越风险 | 安全相对路径和服务端标识 |
| JWT 默认 `change-this-secret` | 配置遗漏即严重风险 | 必填密钥、摘要 Token、服务端撤销 |
| API 返回 `str(e)` | 泄漏路径、地址和实现 | 稳定 `code/message/details` |
| 详细 trace/块内容直达前端 | 泄漏正文和调试信息 | 白名单公开 DTO + 脱敏 trace |
| 通用 Agent 自由调用工具 | 成本、延迟和行为难控 | 业务层编排与硬预算 |

## 7. 更新后的实施顺序

### Phase A：把现有能力产品化（P0）

- 为知识聊天、会话、来源和反馈接口补齐前端 API 与聊天界面。
- 定义版本化 `retrieval_progress`，展示阶段、耗时和来源卡片。
- 正确处理取消、断流、幂等回放和 incomplete 回答。

### Phase B：一次性澄清恢复（P0）

- 增加 `ambiguous/scope_conflict/absent` 证据门控。
- 持久化短期 clarification state 和恢复接口。
- 测试过期、重复、跨用户/租户、超长补充和循环澄清。

### Phase C：复杂问题计划器实验（P1）

- 建立比较、趋势、跨周期冻结集。
- 实现规则快速路径和最多 2～4 个子查询。
- 用 feature flag 对比单查询基线。

### Phase D：文档与检索策略实验（P2）

- 先验证用户 PDF/Office 摄取需求，再决定隔离解析链路。
- 对 Auto-merging、Step-back、HyDE 做离线消融，不直接上线。

## 8. 建议测试矩阵

1. **事件协议**：schema 版本、未知字段、阶段顺序、取消、断流、重复 request ID。
2. **HITL**：直接回答、澄清、范围选择、无知识、正常/重复/过期恢复。
3. **预算与短路**：简单问题无计划器、强证据无重写、最多一次恢复、总时限受控。
4. **并发隔离**：request ID、对话、trace、来源、工具计数和保存结果不串线。
5. **安全**：客户端 tenant ID 无效；跨租户 404；公开 trace 无正文、身份、内部 URL 和上游响应。
6. **质量**：Hit@K、MRR、Context Recall/Precision、引用通过率、拒答准确率、澄清挽救率。
7. **性能**：首事件、首 token、P50/P95、阶段耗时、token 和模型调用次数。
8. **文档摄取**：恶意文件名、超大/加密/损坏文件、压缩炸弹、解析超时和配额回滚。

## 9. 最终判断

SuperMew 最有价值的不是“更复杂的 RAG”，而是把复杂检索变成用户可见、可理解、可恢复的交互。Cortex 当前最应该做的是把已经具备的可靠检索、会话、来源和失败语义呈现出来，然后补一次性澄清恢复；复杂问题分解和 Office/PDF 摄取应分别通过质量评测和用户需求验证后再投入。

**借鉴 SuperMew 的交互协议、状态机、短路测试和长任务反馈；保留 Cortex 的 Go/PostgreSQL 架构、多租户 RLS、可靠任务、引用验证、AI 网关和质量治理。**
