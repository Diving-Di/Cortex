# 模型网关与用量规范

校对日期：2026-10-05。本文维护当前配置与必守边界，检索细节见 [RAG](RAG.md)，验收见 [发布清单](RELEASE_CHECKLIST.md)。

## 生成与密钥

生成请求只通过 LiteLLM OpenAI 兼容 SSE；backend 只持有虚拟密钥，逻辑模型为 cortex-default。
供应商 Key 仅注入网关，不进入前端、URL、Cookie、业务数据、备份、审计或普通日志。
AIClient 负责模型流、Retriever 在可信 Principal/RLS 下检索、AIWorkflow 编排；配额、来源和确认仍属于业务层。

| 配置 | 用途 |
| --- | --- |
| AI_BASE_URL | 内部 LiteLLM `/v1` |
| AI_API_KEY / LITELLM_VIRTUAL_KEY | 受模型和预算限制的网关虚拟密钥 |
| AI_MODEL | cortex-default |
| LITELLM_MASTER_KEY | 网关管理密钥，业务后端不持有 |
| DEEPSEEK_API_KEY / KIMI_API_KEY / OPENAI_API_KEY | 仅网关使用的供应商凭据 |

当前 litellm-config.yaml 配置 DeepSeek 主路由及 Kimi/OpenAI 备用路由，cache:false，num_retries:1，request_timeout:60。
不同环境分别签发虚拟密钥和预算。使用 backend/scripts/provision-litellm-key.ps1 可限制模型、MaxBudget 和 BudgetDuration。
网关不发布宿主机端口；脚本须从能访问内部网关的受控位置执行，不为签发密钥开放公开管理端口。

## 内部检索模型

Compose 内部 embedding-service 加载固定 revision 的 GTE 中文模型，输出 512 维；backend 校验维度，默认不发送 dimensions。
Reranker 使用固定 BGE 模型；完整配置默认 GPU，轻量 profile 明确使用 CPU。
这两个内部模型服务以及隔离 document-parser 不暴露宿主机端口。
原文件、解析文本、向量与业务数据留在自部署环境；生成上下文按业务所需发送给网关。

## 可靠性与来源

整理/报告先生成草稿，确认才写入。报告无来源返回 REPORT_NO_SOURCES，知识问答无证据返回 KNOWLEDGE_NO_EVIDENCE。
报告 SSE sources 绑定本次实际检索来源；确认仍复核租户和周期。
模型、Embedding、精排或网关不可用时返回稳定错误，笔记、搜索、附件、导出继续可用。
已经输出内容的流式响应不得从头重试；断流明确标记未完成，不保存为完整回答，也不覆盖上次完整草稿。
不得绕过网关直连供应商，应用与网关不得叠加不受控重试。重试/备用路由的真实行为由目标环境故障注入验收。

## 隐私、缓存与指标

发送网关的观测元数据仅包括后端生成的非直接身份标识、请求类型、环境、追踪 ID；不含邮箱、姓名或完整正文。
缓存默认关闭，禁止跨租户共享 Prompt/响应。模板公共缓存不等于 AI 内容缓存。

应用 AI usage 当前按字符估算 Token，用于产品统计、状态和体验指标；工作台明确显示估算。
网关/供应商 usage 与价格是准确 Token/成本的依据。无法取得准确值时注明估算，不用于账单。
成本评测同时说明调用次数、重试、模型、价格日期、是否覆盖验证/Judge；方法见 [AI 质量回归](guides/QUALITY_EVALUATION.md)。
当前系统不提供计费功能。新增任务专用模型别名或语义缓存需要单独设计和验收，不作为现有能力宣称。
