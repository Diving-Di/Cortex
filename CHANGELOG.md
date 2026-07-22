# Changelog

## Unreleased

- 实现 M2 AI 笔记能力：快速记录整理的预览/确认流程、日周月报告来源计算与引用持久化、带日期解析和租户隔离引用的回忆问答。
- 新增 `report_sources`、`message_sources` 数据表、RLS 策略及 Alembic `0005_m2_ai_notes` 迁移。
- 工作台、周期报告和回忆书页面接入 SSE 流式草稿，并确保生成内容由用户确认后才写入。

本文件记录 Diary Listener 项目的重要变更。

格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循[语义化版本](https://semver.org/lang/zh-CN/)。

## [Unreleased]

### Git 提交记录

> 维护规则：每次功能、修复、重构或文档提交完成后，在此记录其短哈希、日期和提交说明，再提交 Changelog。由于提交哈希由提交内容计算得出，Changelog 维护提交不记录自身，避免产生无限递归提交。

| Commit | 日期 | 类型 | 说明 |
|---|---|---|---|
| `60df873` | 2026-07-23 | feat | `feat: complete SDD modules 15.5 through 15.8` |
| `54424f5` | 2026-07-23 | refactor | `refactor: establish modular app architecture` |
| `1ee9378` | 2026-07-22 | docs | `docs: add Diary Listener refactor SDD` |

### Added

- 新增 CodeMirror Markdown 编辑器、GFM 预览、防抖自动保存、冲突检测和未保存提示。
- 新增租户隔离的标签、附件、中文搜索、Markdown 导出、备份与受控恢复能力。
- 新增 OpenAI 兼容异步客户端、SSE 流式响应、Provider 配置和 AI 用量记录。
- 新增 PostgreSQL/Alembic、个人租户、认证安全、笔记 CRUD、历史版本和 RLS 基础设施。
- 新增后端 PostgreSQL 集成测试、前端路由测试及生产构建检查。
- 新增 `development-standards/` 项目开发规范目录。
- 新增 Diary Listener 重构软件设计说明书（SDD），用于定义项目目标、系统架构、数据设计、接口、安全要求、实施计划及验收标准。

### Changed

- 后端重组为 `core`、`models`、`schemas`、`services`、`api` 和 `ai` 分层结构。
- 数据库运行时统一为 PostgreSQL，并使用独立应用角色和迁移角色。
- 前端引入 React Router、TanStack Query、路由懒加载和 Webpack 分包预算。
- SDD 15.1～15.8 已按实现和测试结果更新完成状态。
- 将根目录下的 `SDD.md` 归档到 `development-standards/SDD.md`，统一管理项目设计与开发规范文档。
