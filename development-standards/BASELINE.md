# Diary Listener MVP 工程基线

本文冻结 SDD 第 0～3 次迭代的工程决策；功能范围与非目标以 `SDD.md` 第 2 节为准。

## MVP 范围

- 四类 Markdown 笔记、标签、附件、历史版本与搜索。
- 本地优先；AI 不可用时笔记能力完整可用。
- AI 整理、周期报告和带来源引用的回忆问答。
- 每个注册用户只有一个服务端解析的个人租户。
- PostgreSQL 备份/恢复、Markdown 导出和 Windows 本地发布。

桌面悬浮组件、向量数据库、团队协作、计费、自动更新和数据库/文件双向同步不进入 MVP。

## 重构前基线

- 后端入口：`backend/app/main.py`，FastAPI + SQLAlchemy 2；旧接口以 `/api` 为前缀。
- 旧业务接口：注册、登录、聊天会话、同步 AI 回复、图片轻日记。
- 旧数据库：本地 SQLite、Docker MySQL，应用启动调用 `create_all()`。
- 前端入口：`frontend/src/main.tsx`，React 18 + TypeScript + Webpack 5 + Ant Design。
- 运行方式：后端 8000、前端 5173，Webpack 代理 `/api`。

重构采用兼容入口渐进替换旧模块；新接口统一使用 `/api/v1`。

## 数据与配置规则

- PostgreSQL 是正文唯一权威来源，Markdown 只用于导入导出。
- PostgreSQL 16 是开发、测试和生产统一版本；结构只由 Alembic 修改。
- `DATABASE_URL` 使用低权限 `diary_app`；`MIGRATION_DATABASE_URL` 使用 `diary_migrator`。
- 本地数据根目录默认为 `%LOCALAPPDATA%/DiaryListener`，包含 `attachments/`、`exports/`、`backups/`、`logs/`。
- 附件数据库字段只保存数据根目录内相对路径。
- 环境变量优先于被 Git 忽略的 `backend/config.json`；密钥不得进入仓库或日志。
- 本地服务只监听 `127.0.0.1`，前端允许来源默认为 localhost/127.0.0.1:5173。

## PostgreSQL 初始化

- 版本：PostgreSQL 16。
- 数据库：`diary_listener`。
- Owner/迁移角色：`diary_migrator`；应用角色：`diary_app`。
- Docker 首次启动通过 `backend/scripts/init-db-roles.sh` 创建角色；本机环境按 `backend/MIGRATIONS.md` 配置。
- 空库初始化：`alembic upgrade head`；回退验证：`alembic downgrade base`。
