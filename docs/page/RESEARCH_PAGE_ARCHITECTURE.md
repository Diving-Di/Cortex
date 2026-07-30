# 小红书研究页

`/research` 用于创建关键词或公开链接研究任务，查看任务和来源，编辑生成的研究草稿，
以及忽略或删除结果。

研究数据属于当前个人租户并受 RLS 保护。页面不查询知识集合，不提供目标知识集合选择，
后端请求也不接受 `target_collection_id`。研究草稿和来源不会写入 HowToCook 知识库。

HowToCook 知识库是仓库内 `backend/resources/howtocook` 的只读静态语料，与研究功能完全隔离。
研究功能、平台授权、OCR 或 AI 不可用时，不影响笔记和今日菜谱。

主要接口：

| 方法 | 路径 |
| --- | --- |
| `POST` / `GET` | `/api/v1/research/jobs` |
| `GET` | `/api/v1/research/jobs/{job_id}` |
| `POST` | `/api/v1/research/jobs/{job_id}/cancel` |
| `POST` | `/api/v1/research/jobs/{job_id}/retry` |
| `GET` | `/api/v1/research/sources` |
| `GET` / `DELETE` | `/api/v1/research/sources/{source_id}` |
| `PATCH` | `/api/v1/research/sources/{source_id}/draft` |
| `POST` | `/api/v1/research/sources/{source_id}/ignore` |

页面使用授权门禁、任务与来源分页查询、状态轮询和草稿编辑。测试应覆盖无知识集合依赖的
任务创建、失败恢复、空列表兼容和租户边界。
