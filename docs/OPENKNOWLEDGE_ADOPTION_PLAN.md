# OpenKnowledge 可借鉴能力落地方案

> 文档状态：实施规划
> 更新日期：2026-07-28
> 参考项目：[JesstLe/OpenKnowledge](https://github.com/JesstLe/OpenKnowledge/tree/c5e2c49e2d69bcdfa03e78a350c76b75a2636658)
> 适用范围：Diary Listener（Cortex）个人知识库、成长助手和 Dashboard

## 1. 目的与结论

本文把 OpenKnowledge 中适合 Diary Listener 的产品思路转化为可实施任务，并标记当前仓库的
完成状态。参考项目仅用于产品和交互设计研究，不复制其 Python/FastAPI、SQLAlchemy、
LangChain 或直连模型供应商的实现。

总体结论：

- 当前项目的租户隔离、来源引用、混合检索、重排、上下文预算和 AI 网关边界已经优于参考项目，
  应继续作为实现基线。
- 文档处理状态、失败原因和重新索引已经落地，不需要重复建设。
- 文档提取可升级为独立、受限的 Python Markdown 解析 sidecar；Go 后端仍是唯一业务入口，
  并继续负责鉴权、租户、任务、切片、向量化和持久化。
- 下一阶段最有价值的能力是会话管理增强、长对话摘要，以及经过用户确认的结构化成长记忆。

## 2. 状态图例

| 标记 | 状态 | 定义 |
|---|---|---|
| ✅ | 已实现 | 前后端主链路已存在，能够完成声明行为 |
| 🟡 | 部分实现 | 已有可复用基础，但仍缺少接口、数据、交互或完整验收 |
| ⬜ | 未实现 | 仓库中尚无对应业务能力 |
| ⏸️ | 暂缓 | 有价值但不进入近期实施范围 |

状态以 2026-07-28 的仓库代码为准。后续每完成一个阶段，应同步更新本表、API 文档和软件设计
说明书。

## 3. 能力总览

| ID | 可借鉴能力 | 当前状态 | 目标状态 | 优先级 | 建议阶段 |
|---|---|---|---|---|---|
| OK-01 | 文档处理阶段、失败原因和重试 | ✅ 已实现 | 保持并补充验收 | P0 基线 | 持续 |
| OK-02 | 对话持久化、历史列表和删除 | ✅ 已实现 | 保持兼容 | P0 基线 | 持续 |
| OK-03 | 对话自动标题 | 🟡 部分实现 | 首问标题升级为可选 AI 标题 | P2 | 第二阶段 |
| OK-04 | 对话搜索和筛选 | ⬜ 未实现 | 支持标题、消息正文和来源范围检索 | P1 | 第一阶段 |
| OK-05 | 对话重命名 | ⬜ 未实现 | 支持乐观冲突保护的标题修改 | P1 | 第一阶段 |
| OK-06 | 长对话摘要与上下文压缩 | ⬜ 未实现 | 摘要加最近消息，同时每轮重新检索来源 | P2 | 第二阶段 |
| OK-07 | 结构化成长记忆 | ⬜ 未实现 | 可管理、可追溯、经用户确认的个人记忆 | P1 | 第三阶段 |
| OK-08 | AI 记忆建议与确认 | ⬜ 未实现 | AI 只生成建议，确认后写入 | P1 | 第三阶段 |
| OK-09 | 记忆提取策略设置 | ⬜ 未实现 | 类别、阈值、排除标签和保留策略 | P2 | 第四阶段 |
| OK-11 | 对话消息数和 token 用量展示 | 🟡 部分实现 | 会话级聚合并展示 | P3 | 第二阶段 |
| OK-12 | 通用 Agent 工具和网页搜索 | ⏸️ 暂缓 | 不纳入当前产品范围 | - | 不实施 |
| OK-13 | Python Markdown 文档解析服务 | ⬜ 未实现 | TXT、DOCX、PDF 统一转换为 Markdown | P1 | 基础设施阶段 |

## 4. 当前已经实现的基线

### 4.1 知识文件处理（OK-01）— ✅ 已实现

当前已有：

- `uploaded → extracting → indexing → ready/failed` 状态链路。
- 前端对处理中项目每 3 秒轮询。
- 文档列表展示状态、父块数、子块数、大小和失败原因。
- 失败文档支持重新索引。
- 文档详情支持页数、字符数和提取预览。
- 后端保存稳定 `error_code`，并通过安全的 `error_message` 面向用户展示。
- TXT、文本型 PDF 和 DOCX 上传、校验、配额、下载及安全删除。

保持要求：

- 不展示磁盘路径、上游响应正文、密钥或完整内部异常。
- 后续如增加更细进度，只返回离散阶段或整数百分比，不暴露任务租约和 worker 内部信息。
- OCR、XLSX、PPTX、音视频仍遵循 `IMPLEMENTATION_GAPS.md` 的非 MVP 定义。

### 4.2 对话和可信来源（OK-02）— ✅ 已实现

当前已有：

- `/api/v1/conversations` 列表、新建、读取和删除。
- `knowledge`、`growth`、`all` 来源范围。
- 首个问题截断后作为新会话标题。
- 消息持久化、幂等 `request_id` 和 SSE 重放。
- 知识文件与成长记录的统一来源展示。
- 文件、页码、章节、片段和来源删除状态。
- 无证据时返回 `KNOWLEDGE_NO_EVIDENCE`，不会无来源生成答案。

后续增强不得削弱这些行为。

### 4.3 Markdown 文档解析服务（OK-13）— ⬜ 未实现

#### 目标

引入独立的 Python 文档解析 sidecar，把支持的输入统一转换成 Markdown，再交回 Go worker
执行现有的结构化切片、索引和向量化流程。

支持范围：

| 输入 | 目标转换 |
|---|---|
| `.txt` | 规范化编码、换行和段落后输出 Markdown |
| `.docx` | 标题、段落、列表和表格转换为 Markdown |
| `.pdf` | 文本型 PDF 转换为带分页元数据的 Markdown |

明确不支持：

- 扫描 PDF OCR。
- 图片文字识别。
- 手写识别。
- 音频或视频转录。
- XLSX、PPTX 等新增格式。
- 公式、图片和任意嵌入对象的语义理解。

#### 架构边界

Python 组件不是新的业务后端。`backend/cmd/server/main.go` 仍是唯一对外后端入口，客户端不得
直接访问解析服务。

```text
客户端
  → Go API：上传、认证、配额和安全落盘
  → Go knowledge worker：领取租户索引任务
  → Python parser sidecar：文件转 Markdown
  ← Markdown + 页码/结构元数据
  → Go：Block 解析、父子切片、embedding、索引和状态持久化
```

Python sidecar 只负责纯解析：

- [ ] 不连接 PostgreSQL。
- [ ] 不读取 Token、LiteLLM Key 或供应商 Key。
- [ ] 不处理登录、租户、配额、审计或业务状态。
- [ ] 不执行切片、embedding、检索或生成式 AI。
- [ ] 不向宿主机暴露端口，只允许 Compose 内部网络访问。
- [ ] 不把上传目录作为公开静态目录。
- [ ] 普通日志不记录文件正文、Markdown 全文或原始文件路径。

#### 建议实现

建议新建独立目录，例如：

```text
document-parser/
  app.py
  requirements.txt
  Dockerfile
  test_app.py
```

建议解析器：

- TXT：Python 标准库，严格使用 UTF-8，允许去除 BOM。
- DOCX：`python-docx`，将标题、段落、列表和表格渲染为 Markdown。
- PDF：优先 `pymupdf4llm`；如依赖不可用或提取结果无有效文本，可使用 PyMuPDF 文本提取作为
  非 OCR 回退。

不配置 RapidOCR、Tesseract、ocrmac 或任何 OCR 模型。文本型 PDF 提取不到足够内容时返回
稳定错误 `DOCUMENT_OCR_REQUIRED`，不自动尝试识别图片。

#### 内部接口

建议使用仅在 Compose 内部可访问的 HTTP 接口：

```http
POST /v1/parse
Content-Type: multipart/form-data

file=<binary>
format=pdf
```

成功响应：

```json
{
  "markdown": "# 文档标题\n\n正文……",
  "page_count": 12,
  "character_count": 18342,
  "language": "zh",
  "pages": [
    {
      "page": 1,
      "markdown": "第一页内容……"
    }
  ],
  "warnings": []
}
```

约束：

- `markdown` 与 `pages` 是否同时返回应在实现前确定唯一主表示，避免双份大正文占用内存。
- 推荐以分页数组为主，Go 按页构建 Block；完整 Markdown 仅作为可选调试字段且生产环境关闭。
- 所有字符串、页数、数组长度和响应体必须设置硬上限。
- 页码来自服务端解析结果，客户端不能提交或覆盖。
- 警告只使用稳定代码，例如 `LAYOUT_PARTIALLY_PRESERVED`，不携带内部异常正文。

失败响应：

```json
{
  "code": "DOCUMENT_OCR_REQUIRED",
  "message": "PDF 不包含足够的可提取文本"
}
```

建议内部错误码：

| 错误码 | 含义 |
|---|---|
| `DOCUMENT_UNSUPPORTED_TYPE` | 不支持的输入格式 |
| `DOCUMENT_ENCRYPTED` | PDF 已加密或需要密码 |
| `DOCUMENT_OCR_REQUIRED` | 没有足够的可提取文本 |
| `DOCUMENT_PARSE_LIMIT` | 超出页数、字符数、时间或响应大小限制 |
| `DOCUMENT_PARSE_FAILED` | 已安全归一化的其他解析失败 |

Python sidecar 返回的错误不直接透传给客户端。Go handler/worker 必须将其映射为现有稳定错误契约。

#### Markdown 规范

为保证 Go 侧解析稳定，输出限定为以下子集：

- ATX 标题：`#`～`######`。
- 普通段落。
- `-` 无序列表和 `1.` 有序列表。
- fenced code block。
- blockquote。
- GitHub 风格表格。
- 分页元数据通过响应字段传递，不在正文中插入容易与用户内容冲突的伪标题。

解析器必须：

- [ ] 保留文档阅读顺序。
- [ ] 尽量保留标题层级、列表和表格。
- [ ] 规范为 LF 换行。
- [ ] 删除 NUL 等不允许的控制字符。
- [ ] 不渲染或执行宏、外部链接、嵌入脚本和远程资源。
- [ ] 不根据文档内指令访问网络。

#### Go 侧改造

当前 Go 提取器返回 `knowledge.Document` 和 `Block`。引入 sidecar 后应保留该领域模型：

```text
Python Markdown/Page JSON
  → Go Markdown 子集解析器
  → knowledge.Document / Block
  → BuildParentChildChunks
  → embedding 和 PostgreSQL
```

任务：

- [ ] 新增 `DocumentParser` 接口和 sidecar HTTP client，避免 worker 直接依赖具体传输实现。
- [ ] 设置连接、首字节、总处理和响应体大小超时。
- [ ] 使用流式 multipart 上传，避免在 Go 和 Python 两侧无界复制文件。
- [ ] Markdown 解析回现有 `BlockKind`、`HeadingPath`、`PageFrom` 和 `PageTo`。
- [ ] 保留现有 `BuildParentChildChunks`、token 限制、内容 hash 和 embedding 流程。
- [ ] 解析器不可用时将索引任务按有限次数重试；不得影响笔记、搜索、导出和备份。
- [ ] 通过配置启用 sidecar，并保留明确的部署失败提示。
- [ ] 不在应用启动时执行临时 DDL。

是否保留现有 Go/pdftotext 解析器作为降级路径，应在实现时通过固定评测集决定。建议第一版保留
为可配置回退，待 Python 解析服务通过兼容性和稳定性验收后再评估移除，避免部署切换导致所有
文档索引不可用。

#### 部署要求

- [ ] `document-parser` 使用固定依赖版本和固定基础镜像 revision。
- [ ] 容器以非 root 用户运行，只读根文件系统，临时目录设置容量限制。
- [ ] Compose 不映射宿主机端口。
- [ ] Go backend 与 parser 之间设置请求大小和并发上限。
- [ ] `/healthz` 仅表示解析进程存活；可另设内部 readiness 检查依赖是否加载。
- [ ] Parser 不影响 Go `/healthz`；是否影响 `/readyz` 应保持否，以保证非知识功能可用。
- [ ] 文档解析属于跨服务变更，必须运行 `docker compose config --quiet` 和知识库验收脚本。

#### 测试与验收

固定样本至少覆盖：

- [ ] UTF-8 TXT、带 BOM TXT、中英文混排 TXT。
- [ ] DOCX 标题、段落、嵌套列表、表格和分页。
- [ ] 普通 PDF、中文 PDF、英文 PDF、双栏 PDF 和表格 PDF。
- [ ] 加密 PDF、扫描 PDF、空文件、损坏文件和超限文件。
- [ ] 恶意 ZIP/DOCX 解压膨胀、路径名异常和超大 XML。

质量指标：

- [ ] 标题层级保留率。
- [ ] 段落阅读顺序正确率。
- [ ] 表格单元格保留率。
- [ ] 页码映射准确率。
- [ ] 中文字符保留率。
- [ ] 解析成功率、P50/P95 延迟和峰值内存。

端到端验收：

- [ ] 同一文件经 Markdown 解析后可以完成父子切片、embedding、混合检索和来源引用。
- [ ] A 租户文档不会被 B 租户解析任务或查询访问。
- [ ] Parser 停止时任务有限重试，非 AI 和非知识功能继续可用。
- [ ] 扫描 PDF 稳定返回 `DOCUMENT_OCR_REQUIRED`，不触发 OCR。
- [ ] 日志、审计、数据库和 API 响应均不泄露内部路径或完整正文。
- [ ] 与当前 Go/pdftotext 基线进行固定样本 A/B 对比，达标后才能默认启用。

## 5. 第一阶段：对话管理增强

目标：在不引入 AI 依赖的情况下，使历史对话可查找、可整理。

### 5.1 对话搜索和筛选（OK-04）— ⬜ 未实现

后端任务：

- [ ] 扩展 `GET /api/v1/conversations`：
  - `search`：搜索标题和消息正文。
  - `source_scope`：`knowledge`、`growth`、`all`。
  - `limit`、`offset`：稳定分页。
- [ ] SQL 和事务逻辑放入 `backend/internal/store`。
- [ ] 查询必须通过 `Store.WithTx`，同时设置 RLS 上下文并保留显式 `tenant_id`。
- [ ] 搜索结果按 `updated_at DESC, id DESC` 稳定排序。
- [ ] 不在列表响应中返回完整消息正文；可返回长度受限的命中摘要。

前端任务：

- [ ] 成长助手会话栏增加搜索框。
- [ ] 增加来源范围筛选。
- [ ] 搜索输入防抖，清空时恢复最近会话。
- [ ] 提供加载、空结果和失败状态。

建议接口：

```http
GET /api/v1/conversations?search=季度目标&source_scope=growth&limit=20&offset=0
```

```json
{
  "items": [
    {
      "id": 12,
      "title": "第二季度成长目标",
      "source_scope": "growth",
      "matched_snippet": "……季度目标……",
      "created_at": "2026-07-28T08:00:00Z",
      "updated_at": "2026-07-28T09:00:00Z"
    }
  ],
  "total": 1
}
```

验收标准：

- [ ] 标题和消息正文均可命中。
- [ ] A 租户无法通过关键词、ID 或分页侧信道看到 B 租户会话。
- [ ] 命中摘要有固定长度上限，不返回无关完整日记正文。
- [ ] 空搜索、中文、特殊字符和超长搜索词有测试。

### 5.2 对话重命名（OK-05）— ⬜ 未实现

后端任务：

- [ ] 新增 `PATCH /api/v1/conversations/{conversation_id}`。
- [ ] 标题 trim 后长度限制为 1～255 个 Unicode 字符。
- [ ] 为 `conversations` 增加 `version` 字段，使用乐观冲突保护。
- [ ] 跨租户或不存在的会话统一返回 404。
- [ ] 更新成功写入不含标题正文的审计记录。

建议契约：

```json
{
  "title": "2026 年职业规划",
  "version": 3
}
```

冲突返回：

```json
{
  "code": "CONVERSATION_VERSION_CONFLICT",
  "message": "会话已被更新，请刷新后重试"
}
```

前端任务：

- [ ] 会话菜单增加“重命名”。
- [ ] 保存期间禁用重复提交。
- [ ] 409 时刷新最新数据并保留用户输入。

验收标准：

- [ ] 合法标题可以更新并立即反映到历史列表。
- [ ] 旧版本并发更新返回 409。
- [ ] 非法标题返回稳定校验错误。
- [ ] 租户隔离和软删除租户行为符合仓库规范。

## 6. 第二阶段：长对话和标题增强

### 6.1 对话摘要（OK-06）— ⬜ 未实现

原则：

- 摘要只压缩会话历史，不作为事实来源。
- 每次回答仍需在当前 Principal/RLS 下重新检索知识文件和成长记录。
- 摘要中的指令视为不可信内容，不得覆盖系统规则。
- AI 不可用时继续使用最近消息，不影响知识库管理和非 AI 功能。

建议数据变更：

```text
conversations
  summary                  text null
  summary_through_message_id integer null
  summary_version          integer not null default 0
  summary_model            varchar(120) null
  summary_updated_at       timestamptz null
```

实现任务：

- [ ] 新增版本化数据库迁移，并同步 `backend/db/schema.sql`。
- [ ] 当未摘要消息超过配置阈值时，将摘要任务入队。
- [ ] worker 在租户事务内读取旧摘要和增量消息。
- [ ] 生成新摘要时限制输入、输出 token，并记录 AI 用量与审计事件。
- [ ] 通过 `summary_through_message_id` 防止消息遗漏和重复覆盖。
- [ ] 组装对话上下文时使用“历史摘要 + 最近 N 条消息”。
- [ ] 并发生成通过版本号或行锁保证只有一个结果生效。

建议默认阈值：

- 至少 20 条消息后才触发。
- 保留最近 10 条原始消息。
- 摘要最多约 1,000 token。
- 阈值必须可由服务端配置，不接受客户端任意扩大。

验收标准：

- [ ] 摘要前后对同一问题的关键约束保持一致。
- [ ] 新消息不会被并发摘要遗漏。
- [ ] 删除来源后，摘要不能使已删除内容重新成为可引用来源。
- [ ] AI 未配置或摘要失败时，聊天主链路按预期降级。

### 6.2 自动标题（OK-03）— 🟡 部分实现

当前行为：新会话使用首个问题前 80 个字符作为标题。

目标行为：

- [ ] 保留当前确定性标题作为立即可用的默认值。
- [ ] 在首轮回答成功后，可选地异步生成 10～30 字的标题建议。
- [ ] 仅当用户未手动改名且会话版本未变化时自动应用。
- [ ] AI 标题失败不影响对话。
- [ ] 不向普通日志记录问题或生成标题正文。

### 6.3 消息数和 token 用量（OK-11）— 🟡 部分实现

当前已有全局 AI 用量记录，但会话列表没有可靠的会话级聚合。

任务：

- [ ] 会话列表返回 `message_count`。
- [ ] 评估为 `ai_usage_records` 增加可空的 `conversation_id`，或建立专用关联表。
- [ ] 只有具备可靠关联后才展示 `input_tokens`、`output_tokens`。
- [ ] 明确应用估算 token 与 LiteLLM 计费 token 的口径，禁止混为同一指标。

## 7. 第三阶段：结构化成长记忆

### 7.1 领域定义（OK-07）— ⬜ 未实现

“成长记忆”是用户确认保存、可检索和可管理的结构化信息，不等同于：

- 原始日记或笔记；
- 对话摘要；
- AI 临时上下文；
- 模型自行推断的用户画像。

建议类别：

| 值 | 含义 |
|---|---|
| `fact` | 用户确认的稳定事实 |
| `preference` | 用户确认的偏好 |
| `goal` | 有待推进的目标 |
| `habit` | 习惯或规律 |
| `milestone` | 已发生的重要节点 |

建议表：

```text
growth_memories
  id                    bigint identity
  tenant_id             uuid
  user_id               integer
  category              varchar(20)
  content               text
  importance            smallint
  source_type           varchar(30)
  source_note_id        integer null
  source_conversation_id integer null
  source_message_id     integer null
  creation_mode         varchar(20)  -- manual / ai_confirmed
  version               integer
  created_at            timestamptz
  updated_at            timestamptz
  deleted_at            timestamptz null
```

约束：

- [ ] `importance` 为 1～10。
- [ ] 内容有明确 Unicode 长度上限。
- [ ] 来源 ID 必须属于当前租户，跨租户统一表现为 404。
- [ ] 更新使用 `version` 乐观锁。
- [ ] 删除默认软删除。
- [ ] 表启用并强制 RLS，应用查询同时保留显式 `tenant_id`。
- [ ] 完整备份包含成长记忆及其安全来源映射，但不包含敏感审计正文。

建议接口：

```text
GET    /api/v1/growth-memories
POST   /api/v1/growth-memories
GET    /api/v1/growth-memories/{id}
PATCH  /api/v1/growth-memories/{id}
DELETE /api/v1/growth-memories/{id}
```

列表支持：

```text
category, min_importance, search, source_type, limit, offset
```

前端任务：

- [ ] 新增长期记忆页面或成长助手中的独立页签。
- [ ] 支持分类、重要度、来源、创建方式和更新时间展示。
- [ ] 支持手动新建、编辑、软删除和筛选。
- [ ] 来源仍存在时可跳转；来源删除后显示失效状态。

### 7.2 AI 记忆建议与确认（OK-08）— ⬜ 未实现

采用两阶段流程：

```text
用户主动请求提取
  → AI 生成结构化建议草稿
  → 服务端校验类别、重要度和来源
  → 用户逐条编辑、拒绝或确认
  → 确认接口写入成长记忆
```

禁止：

- AI 响应完成后自动写入正式记忆。
- 仅凭模型输出绑定来源 ID。
- 从其他租户或已经删除的来源建立记忆。
- 将邮箱、姓名、完整正文放入网关观测元数据。

建议接口：

```text
POST /api/v1/growth-memory-drafts
GET  /api/v1/growth-memory-drafts/{draft_id}
POST /api/v1/growth-memory-drafts/{draft_id}/confirm
```

建议草稿响应：

```json
{
  "draft_id": "server-generated-id",
  "expires_at": "2026-07-28T10:00:00Z",
  "items": [
    {
      "category": "goal",
      "content": "在九月底前完成 Go 并发课程",
      "importance": 8,
      "source_type": "note",
      "source_id": 42
    }
  ]
}
```

实现任务：

- [ ] Prompt 明确来源内容不可信并要求严格 JSON。
- [ ] 服务端不信任模型给出的类别、分数和来源 ID，全部重新校验。
- [ ] 草稿设置过期时间且只能确认一次。
- [ ] 确认前再次验证来源归属和状态。
- [ ] 重复建议检测使用规范化内容与来源组合，冲突时由用户选择。
- [ ] 记录生成、确认、拒绝审计事件，但普通日志不含记忆正文。
- [ ] AI 不可用返回稳定错误，手动记忆 CRUD 仍可用。

验收标准：

- [ ] 未确认草稿不会出现在正式记忆和成长助手检索中。
- [ ] 篡改草稿中的来源 ID 无法跨租户写入。
- [ ] 草稿过期、重复确认和来源删除均有稳定错误码。
- [ ] 用户可以在确认前编辑类别、内容和重要度。

## 8. 第四阶段：记忆提取策略

### 8.1 策略设置（OK-09）— ⬜ 未实现

不直接复制参考项目的自由文本黑白名单。优先采用结构化、可解释规则：

```text
memory_suggestion_enabled     boolean default false
allowed_categories           varchar[] default all
minimum_importance           smallint default 5
excluded_note_types          varchar[] default empty
excluded_tag_ids             bigint[] default empty
retention_days               integer null
```

产品默认值：

- 默认关闭自动建议。
- 即使开启，也只生成草稿，不自动写入。
- 用户可排除私密标签或特定笔记类型。
- 排除规则优先于允许规则。

任务：

- [ ] 设置存储在租户范围内，并纳入完整备份与恢复。
- [ ] 提议任务开始和确认时都重新读取策略。
- [ ] 被排除来源不得发送给 AI。
- [ ] 设置页面解释数据会发送到已配置的 LiteLLM 网关。
- [ ] 修改设置写入不含具体标签文本的审计记录。

## 9. 明确不实施或不照搬

### 9.1 通用 Agent 工具（OK-12）— ⏸️ 暂缓

网页搜索、计算器和任意工具调用不属于当前产品范围。若未来引入，必须单独设计工具权限、网络
访问、Prompt 注入防护、审计和来源可信度，不能直接复用参考项目的实现。

### 9.2 禁止照搬的实现

- [x] 不引入 Python 后端、FastAPI、SQLAlchemy、Alembic 或 LangChain。
- [x] 不让前端保存或提交供应商真实 API Key。
- [x] 不绕过 LiteLLM 直连供应商。
- [x] 不让 AI 自动写入正式记忆。
- [x] 不使用没有租户条件和 RLS 上下文的向量检索。
- [x] 不把异常、正文、上游响应或密钥打印到普通日志。
- [x] 不将上传文件、运行日志或本地数据库提交到 Git。
- [x] 不以 README 声明代替已通过测试和验收的实现。

这里的 `[x]` 表示已确定的架构决策，不表示新增功能已经实现。

## 10. 推荐实施顺序与依赖

```mermaid
flowchart LR
    A["现有安全与 RAG 基线"] --> P["基础设施：Markdown 解析服务"]
    P --> B["第一阶段：搜索与重命名"]
    B --> C["第二阶段：摘要、标题和统计"]
    C --> D["第三阶段：成长记忆与确认"]
    D --> E["第四阶段：提取策略"]
```

建议交付批次：

1. OK-13：独立实现并 A/B 验证 Markdown 解析服务，不改变现有切片和检索语义。
2. OK-04、OK-05：纯会话管理，风险较低；可与 OK-13 独立排期。
3. OK-06、OK-03、OK-11：涉及迁移、AI 成本和并发控制。
4. OK-07：先完成纯手动成长记忆 CRUD、来源和备份恢复。
5. OK-08：在 CRUD 稳定后接入 AI 草稿与确认。
6. OK-09：增加用户策略和后台建议。

## 11. 通用工程要求

每个阶段都必须满足：

- 新接口使用 `/api/v1`，保持既有兼容路径。
- handler 只处理 HTTP 契约；SQL 和事务放在 `backend/internal/store`。
- 数据库变更同时新增版本化迁移并更新 `backend/db/schema.sql`。
- 新租户表启用并强制 RLS；所有业务查询使用 `Store.WithTx` 和显式 `tenant_id`。
- 跨租户资源访问统一表现为 404。
- 更新使用版本号乐观冲突保护，删除默认软删除。
- AI 能力坚持“草稿—确认—写入”，并保存经过服务端验证的来源。
- AI 未配置或不可用时，非 AI 功能保持可用。
- 任何新数据纳入 Markdown ZIP、完整备份、空租户恢复和配额影响评估。
- API 变更同步 `docs/api.md`，架构变更同步 `docs/SDD.md`。
- 单元、集成、RLS、并发、降级和前端交互测试与风险相称。

## 12. 每阶段完成检查

实现任务只有同时满足以下条件，才能从 ⬜/🟡 更新为 ✅：

- [ ] 数据库迁移、基线 schema、RLS、索引和回滚路径完整。
- [ ] Store、handler、前端 API 和 UI 主链路完成。
- [ ] 稳定错误码和跨租户 404 行为有测试。
- [ ] AI 降级、幂等、并发冲突和来源失效按适用范围测试。
- [ ] 备份、恢复、导出和删除语义已评估并实现。
- [ ] `docs/api.md`、`docs/SDD.md` 和本状态文档已同步。
- [ ] 后端通过 `go vet ./...`、`go test ./...`、`go build ./cmd/server`。
- [ ] 前端通过 `npm run format:check`、`npm test`、`npm run build`。
- [ ] 数据库、AI 或跨服务变更通过相应 smoke/acceptance 验收。

## 13. 当前执行建议

下一项建议实施工作是 OK-13 Markdown 文档解析服务：

1. 建立受限的 `document-parser` sidecar 和固定 TXT、DOCX、PDF 样本集。
2. 定义并测试 Markdown 子集、分页元数据和稳定错误码。
3. 在 Go 中增加 `DocumentParser` client，并转换回现有 `knowledge.Document/Block`。
4. 保留父子切片、embedding、引用和租户事务逻辑不变。
5. 与当前 `pdftotext` 基线进行 A/B 质量、延迟和内存验收。
6. 通过配置灰度启用，确认稳定后再评估是否移除旧解析路径。

该阶段不依赖模型，不改变现有检索链路，也不会扩大 AI 数据处理范围。会话搜索和重命名
（OK-04、OK-05）与它没有代码依赖，可单独排期。
