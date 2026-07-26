# Web MVP 发布验收记录

验收日期：2026-07-23  
验收平台：Docker Desktop、Docker Compose、PostgreSQL 16、Nginx 1.27、Python 3.11  
访问入口：`http://127.0.0.1:5173`

## 发布形态

- 前端使用 Node 多阶段构建，生产镜像只包含 Nginx 和 Webpack 构建产物。
- Nginx 提供 SPA 静态资源，将 `/api/` 同源反向代理到 Go/Gin 后端。
- PostgreSQL 空库通过 `backend/db/schema.sql` 初始化。
- PostgreSQL 数据和应用附件分别使用 Docker 命名卷持久化。

## 自动化测试

| 项目 | 结果 | 证据摘要 |
| --- | --- | --- |
| 后端全量测试 | 通过 | `go vet ./...`、`go test ./...` 和独立 PostgreSQL 16 空库初始化通过 |
| 前端格式 | 通过 | Prettier 检查通过 |
| 前端测试 | 通过 | Vitest 1 个测试文件、2 项测试通过 |
| 生产构建 | 通过 | TypeScript 检查和 Webpack production build 通过 |
| Compose 配置 | 通过 | `docker compose config --quiet` 通过 |

## Web 部署验收

| 项目 | 结果 | 证据摘要 |
| --- | --- | --- |
| 容器健康 | 通过 | PostgreSQL、LiteLLM 与 Go 后端健康检查通过，Nginx 前端入口返回 200 |
| SPA 与 PWA | 通过 | 首页、笔记深层路由、manifest、Service Worker 和离线页均由 Nginx 提供 |
| 默认个人租户 | 通过 | 两个新注册用户自动获得不同的个人租户 |
| 四类笔记 | 通过 | 通过 Nginx 入口创建 normal、daily、weekly、monthly 共 4 篇笔记 |
| 工作台 | 通过 | Dashboard 返回 4 篇笔记及对应统计 |
| 租户注入防护 | 通过 | 请求体提交 `tenant_id` 返回 422 |
| 跨租户访问 | 通过 | 第二个用户读取第一个用户笔记返回 404 |
| PostgreSQL RLS | 通过 | 低权限应用角色未设置租户上下文时不可读取业务数据；设置可信上下文后仅返回当前租户数据 |
| 标签、附件和搜索 | 通过 | PostgreSQL 集成测试覆盖标签关联、附件鉴权、中文检索和伪造文件拒绝 |
| AI 不可用降级 | 通过 | 未配置 AI 时返回明确错误，普通笔记、搜索、导出和备份保持可用 |
| 备份恢复 | 通过 | PostgreSQL 集成测试将包含笔记、标签和附件的备份恢复到空个人租户 |
| 服务重启 | 通过 | 重启 Go 后端容器并恢复健康后，重启前创建的笔记仍可读取 |
| 敏感信息扫描 | 通过 | 源码和镜像配置未发现开发密钥；生产前端无 source map、无 Node 运行时 |
| 运行日志 | 通过 | 验收期间前后端日志无 ERROR、CRITICAL 或 Traceback |
| 基础性能 | 通过 | 经 Nginx 连续执行 100 次 Dashboard 请求，p95 36.46 ms，最大 82.96 ms |

## 验收中修复

PostgreSQL 集成测试发现 AI SSE 响应开始后，原请求事务级 RLS 上下文已经结束，导致
`ai_usage_records` 写入被 RLS 拒绝。现已在两个流式生成器事务开始时重新应用由服务端解析的
`TenantContext`，修复后全部 17 项 PostgreSQL 测试通过。
