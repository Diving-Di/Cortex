# 本机数据库与功能集成验收（2026-09-07）

> 文档整理后归入日期验收目录。原始日志和备份已归档到项目外 `E:/Codebase/Cortex-local-archive-20260907/.tmp-acceptance-20260907/`；以下记录中的路径描述验收时位置。

本次验收通过。工作区 Go 代码通过真实 PostgreSQL、Redis、MinIO 测试；HTTP、AI、浏览器和恢复验收使用本机现有 Compose 后端镜像。未执行生产发布。

## 环境配置

- `DATABASE_URL` 与 `MIGRATION_DATABASE_URL` 已保存到当前 Windows 用户环境变量，分别使用 `cortex_app` 和 `cortex_migrator`，连接本机 CI 测试库。
- 当前已打开的终端可执行 `. ./backend/scripts/local_test_env.ps1` 加载测试连接及 Redis、MinIO 测试参数。
- 使用 `docker-compose.ci.yml` 创建独立测试服务和数据卷，业务数据库未用于 Go 集成测试。
- Docker Desktop 启动失败原因是残留的 `dockerInference` 通信端点无法删除；旧运行目录已保留为 `C:/Users/35117/AppData/Local/Docker/run-backup-20260907-gc`，重建运行目录后引擎恢复，原容器和卷保留。

## 验收结果

| 检查 | 结果 |
| --- | --- |
| 新 PostgreSQL 16 初始化与全部迁移 | 版本 42，57 张 public 表 |
| Go 全量测试（含子测试） | 170 通过，0 失败，0 跳过 |
| GC 数据库集成 | 崩溃回收、旧 owner 隔离、附件记录清理、软删除附件释放配额通过 |
| 其他数据库集成 | RLS、索引原子性、索引租约、Outbox 并发/回收、scheduler 租约通过 |
| Redis 集成 | 通过 |
| 真实 MinIO 版本删除 | 删除旧版本、重复删除、最新版本保留通过；测试桶与对象已清理 |
| Go 格式、vet、server 构建 | 通过 |
| Go 覆盖率门禁 | 27.3%，要求至少 18% |
| 确定性 RAG 回归夹具 | 通过 |
| 前端格式、单元测试、覆盖率与生产构建 | 13 个测试文件、30 项测试通过；语句覆盖率 52.01% |
| Playwright | 2 项通过，含真实注册/登录/dashboard/退出；使用本机 Chrome |
| 非 AI HTTP 冒烟 | 中文搜索、跨租户附件 404、导出、删除租户后鉴权失败等通过 |
| 真实 LiteLLM AI 验收 | `cortex-default` SSE、整理确认、报告确认和来源持久化通过 |
| 文档解析 | PDF、DOCX、图片 OCR 通过 |
| Compose 与运行检查 | 配置校验通过，14 个核心服务 healthy，生产验收脚本通过 |
| 隔离备份/恢复 | 迁移 42、57 表、44 张租户表均 FORCE RLS；恢复后非 AI HTTP 冒烟通过 |

备份/恢复使用 CI 测试库，恢复耗时约 20.96 秒。该备份没有业务 MinIO 对象，不能据此声称验证了真实业务对象的灾难恢复；对象版本删除已由单独的真实 MinIO 集成测试验证。备份及原始测试日志保留在忽略目录 `.tmp-acceptance-20260907/`，恢复演练临时容器、网络与卷已由脚本清理。

首次浏览器执行因缺少 Playwright 对应 Chromium 失败；改用已有 Chrome 后两项测试通过。取消了不再需要的慢速 Chromium 下载。

## 重跑

在仓库根目录执行：

```powershell
./backend/scripts/local_integration_test.ps1
```

入口已实际重跑通过，已有 Kafka Topic 不会造成重复初始化失败。此入口负责依赖、迁移、全量 Go 测试、vet 和构建；前端、HTTP、AI 与恢复验收仍使用仓库各自的命令和脚本。
