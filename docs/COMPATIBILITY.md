# Cortex 技术标识兼容策略

Cortex 是产品展示名称。为避免破坏现有安装，下列内部标识在当前主版本保持不变：

- Go module `diary-listener/backend` 和前端 npm package `ai-chat-diary-frontend`；
- `DIARY_*`、`RAG_*`、`AI_*` 等环境变量；
- PostgreSQL 角色 `diary_app`、`diary_migrator`、数据库名及现有 schema；
- `DIARY_DATA_DIR`、Compose `db_data` / `app_data` 卷和卷内目录；
- 已发布的 localStorage key 与旧 `/api/chat/`、`/api/diary/` 兼容路径。

浏览器标题、PWA metadata、导航和 OCI image label 使用 Cortex。内部标识如果在后续
主版本改名，必须采用“先双读、再新写、最后停读旧值”的版本化迁移；数据卷只能原位复用
或复制验证后切换，不得通过改 Compose project/volume 名隐式创建空卷。回滚版本继续读取
旧标识，因此迁移期不得删除旧 key、角色、目录或兼容接口。
