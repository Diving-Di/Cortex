# 实现与生产验收待办

## 当前边界

- 知识库唯一来源为 `backend/resources/howtocook`。
- `/knowledge` 页面、`/api/v1/knowledge/*`、个人知识上传、集合、解析、索引和问答已移除。
- 研究、日报、周报、月报和个人笔记不会进入 HowToCook 知识库。
- 迁移 `000012_remove_personal_knowledge` 会永久删除旧个人知识表和历史记录，并移除研究表中的旧关联字段。

## 发布前验证

- 运行后端 vet、测试和 server/migrate 构建。
- 运行前端格式检查、测试和生产构建。
- 运行 `docker compose config --quiet`。
- 在完整环境运行 `non_ai_smoke.ps1`、`ai_acceptance.ps1`、
  `recipe_sync_acceptance.ps1`、`research_acceptance.ps1`。
- 确认新实例能从仓库内固定 HowToCook revision 完成同步和索引，不读取宿主机外部语料。
- 确认普通用户无法访问 `/knowledge` 页面或 `/api/v1/knowledge/*`。

## 非当前范围

- 用户知识文件上传、集合管理和通用知识问答。
- 将研究结果、个人笔记或周期报告加入菜谱知识库。
- 数据库与 Markdown 双向同步。
- 团队知识库、云盘同步、计费和桌面组件。
