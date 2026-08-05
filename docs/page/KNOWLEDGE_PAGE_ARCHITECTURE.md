# 个人知识库页

`/knowledge` 是个人知识库 v2 的入口，用于上传 Markdown / Markdown ZIP 资料、查看文档索引
状态、容量配额，并删除不再需要的知识文档。`/recipes` 与 `/assistant` 已重定向到本页。

知识数据属于当前个人租户并受 RLS 保护；每个租户容量上限 3 GiB，客户端提交的 `tenant_id`
始终被忽略。HowToCook 固定语料已从仓库移除并迁移到用户 `Diving` 的运行时私有知识库，
个人知识库只检索当前租户上传的资料与主动开启知识问答的个人笔记。

## 页面区域

- 上传区：拖放或选择单个 `.md` 或 `.zip`（ZIP 可包含 Markdown 引用的 PNG/JPG/GIF/WebP 图片），
  上传前展示格式与配额说明；上传成功后返回 202 并异步建立索引。
- 配额区：展示已用、剩余容量和进度条，容量判断以后端返回为准。
- 文档列表：显示标题、来源类型（上传资料 / 个人笔记）、大小、索引状态与失败摘要，支持删除。
- 状态：`uploaded`、`parsing`、`indexing`、`ready`、`failed`、`deleting`；失败文档保留
  稳定错误摘要，不展示服务器路径。

## 主要接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/v1/knowledge/documents` | 列出当前租户文档与 3 GiB 配额 |
| `POST` | `/api/v1/knowledge/uploads` | 上传 `.md` / `.zip`，安全落盘后返回 202 |
| `DELETE` | `/api/v1/knowledge/documents/{id}` | 删除文档并使其退出检索 |
| `PATCH` | `/api/v1/notes/{id}/knowledge` | 开启或关闭笔记知识索引（供笔记设置使用） |

后台 `RunKnowledgeIndexer` 负责解析、父子切块与 Embedding；问答通过
`POST /api/v1/knowledge/chat/stream` 在服务端验证的范围内混合检索、精排并 SSE 回答。

## 安全、降级与验收

- 上传文件保存在 `CORTEX_DATA_DIR/knowledge/{tenant_id}/...` 安全相对路径，不作为公开静态目录。
- AI / Embedding 不可用时，上传与删除仍可用；索引任务标记失败，问答返回
  `KNOWLEDGE_EMBEDDING_UNAVAILABLE` / `KNOWLEDGE_RERANK_UNAVAILABLE`，无证据返回
  `KNOWLEDGE_NO_EVIDENCE`。
- 验收覆盖：`.md` / `.zip` 上传与配额、跨租户 404 隔离、文档删除后退出检索、笔记知识开关、
  混合问答来源保存与降级边界。
