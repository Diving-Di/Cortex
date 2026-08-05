# 实现与生产验收待办

> 更新日期：2026-08-05

## 当前边界

- 个人知识库 v2 已实现：上传单个 `.md` 或 Markdown `.zip`、知识集合、文档列表与删除、
  个人笔记知识开关、3 GiB 配额，以及带来源保存的混合问答（`/api/v1/knowledge/*`）。
- 迁移 `000017_personal_knowledge_v2` 新增 `knowledge_*` 九张表并启用 RLS，是当前知识库的
  数据库基线；`/knowledge` 页面已上线，`/recipes` 与 `/assistant` 路由重定向到 `/knowledge`。
- HowToCook 固定语料已从仓库移除（`backend/resources/howtocook` 不再存在），并一次性迁移到
  用户 `Diving` 的运行时私有知识库，不再作为系统级全局语料或应用种子分发。
- 菜谱接口（`/api/v1/recipes/*`）与忌口、时区偏好已从后端移除；相关代码、评测工具与
  验收脚本已清理，前端 `/recipes`、`/assistant` 重定向到 `/knowledge`。
- 研究、日报、周报、月报和个人笔记不会写入个人知识库；研究内容与知识库检索相互隔离。

## 发布前验证

- 运行后端 vet、测试和 server/migrate 构建。
- 运行前端格式检查、测试和生产构建。
- 运行 `docker compose config --quiet`。
- 在完整环境运行 `non_ai_smoke.ps1`、`ai_acceptance.ps1`、`research_acceptance.ps1`、
  `template_ai_event_acceptance.ps1`、`backup_acceptance.ps1`。
- 验证个人知识库：上传 `.md` / `.zip`、配额与并发预占、文档删除后退出检索、笔记知识开关、
  混合问答的来源保存与 `KNOWLEDGE_NO_EVIDENCE`、跨租户 404 隔离。
- 确认新实例能由 `backend/db/schema.sql` 基线加版本化迁移完成初始化（当前共 51 张表），
  且 `/recipes`、`/assistant` 均重定向到 `/knowledge`。
- 确认仓库不再包含 `/api/v1/recipes/*` 路由与忌口、时区偏好字段。

## 非当前范围

- 团队知识库、云盘同步、计费和桌面组件。
- 数据库与 Markdown 双向同步。
- 管理员审核公开模板；模板上架与下架由作者自主决定。
