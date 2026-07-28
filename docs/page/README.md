# 页面说明文档索引

本目录保存“当前实现”的页面级架构与功能说明。页面文档应以实际路由、组件、API 和后端行为
为准，不用于记录尚未实现的产品规划。

## 当前覆盖

| 路由 | 页面组件 | 说明文档 | 状态 |
| --- | --- | --- | --- |
| `/knowledge` | `features/knowledge/KnowledgePage.tsx` | [个人知识库文档页](KNOWLEDGE_PAGE_ARCHITECTURE.md) | 已覆盖 |
| `/research` | `features/research/ResearchPage.tsx` | [小红书研究页](RESEARCH_PAGE_ARCHITECTURE.md) | 已覆盖 |
| `/assistant` | `features/assistant/GrowthAssistantPage.tsx` | [成长助手页](ASSISTANT_PAGE_ARCHITECTURE.md) | 已覆盖 |

## 尚未建立独立页面说明

以下现有路由有页面实现，但当前没有放在本目录的独立架构说明：

| 路由 | 页面组件 |
| --- | --- |
| `/login` | `features/auth/LoginPage.tsx` |
| `/` | `features/dashboard/DashboardPage.tsx` |
| `/notes/*` | `features/notes/NotesPage.tsx` |
| `/reports` | `features/reports/ReportsPage.tsx` |
| `/settings` | `features/settings/SettingsPage.tsx` |

这些缺项不代表页面功能未实现，只表示尚无页面级专项说明。新增对应文档时，应至少包含：

- 页面目标、范围和非目标。
- 页面区域、交互、状态和响应式行为。
- 前端查询、Mutation、轮询和错误处理。
- HTTP API、后端组件和持久化模型。
- 租户、安全、降级与删除语义。
- 自动化测试和端到端验收要点。

## 维护规则

- 文件名使用 `<PAGE>_PAGE_ARCHITECTURE.md`。
- 页面改名或移动后，同步修改文档标题、路由、文件路径和交叉链接。
- 从 `docs/` 移入本目录的相对链接通常需要增加 `../`。
- 规划性方案继续保存在 `docs/`，不能用尚未实现的目标替代本目录的当前实现说明。
- 页面、API、状态机或 Worker 行为变化时，同步更新页面文档、`../api.md` 和 `../SDD.md`。
