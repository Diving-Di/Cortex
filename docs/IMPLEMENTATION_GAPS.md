# 实现与生产验收待办

> 更新日期：2026-08-12

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

## 模板广场当前实现与剩余待办

已完成并验收：

- Marketplace Outbox 只领取 `aggregate_type='template'`，使用 owner token、未过期租约、周期续租和
  数据库完成态 fencing；租约丢失后不能把事件错误标记为完成。
- `new` / `trending` 排行离线写入版本键，通过 `active_version` CAS 原子切换；启动重建不再先删除
  线上 ZSet。下架和删除会 best-effort 清除当前 active 排行与详情缓存，公开读取仍回 PostgreSQL
  校验发布状态。
- daily ZSet 和 UV HLL 保留 8 天；`GET /api/v1/templates/public/{public_id}/stats` 使用 `PFCOUNT`
  返回指定日期匿名 UV。浏览去重 key 使用后端稳定摘要，不包含原始 tenant UUID。
- Redis RESP 客户端使用最大 64 条连接的有界复用池；迁移 `000025_marketplace_hardening` 新增公开模板
  trigram 搜索索引；trending 使用 7 天半衰期分数。迁移 `000026_remove_template_reports` 删除模板举报
  表及对应 API、服务端逻辑和前端入口。
- [模板广场容量验收记录](MARKETPLACE_CAPACITY_ACCEPTANCE.md) 保存了本地容器网络环境、可复现命令、
  原始结果摘要和指标口径。

仍需完成：

- 补 Outbox 多 worker 崩溃、续租失败和数据库完成 fencing 的 PostgreSQL 并发集成测试及租约指标。
- 详情缓存值尚未携带发布版本；当前安全性依赖下架失效和公开列表/详情回表校验。
- 仍无真实线上 QPS、HTTP p95/p99、缓存命中率或数据库查询下降比例。对外性能数字必须基于固定数据集、
  HTTP 压测、原始结果及数据库/Redis/连接池指标。

## AI 活动削峰待办

- 当前领取在 Redis 不可用或投影未就绪时 fail-closed。若要启用数据库完整资格回源，先实现独立并发
  舱壁、短队列、超时、熔断和连接池预算；业务性 409 不应计入熔断失败。
- 认证仍逐请求解析 Principal 并更新 Token 最近使用时间。待实现基于 Token SHA-256 摘要的 Redis
  Principal/无效 Token 缓存、`last_used_at` 限频、租户/Token 版本失效与受限认证 DB 回源。
- claim 专用 IP 限流仍在认证之后。待拆分路由，使匿名洪峰先通过隐私安全的 IP 摘要限流，再认证并做
  用户限流；Redis 故障的本地限流不能被视为多实例全局限流。
- 数据库 fallback 应在资格查询后才以条件更新竞争库存，并保持点数、claim 和事件写入同一事务；不能
  让大量降级请求在活动热点锁内串行计算资格。
- 内部 reservation fencing、pending reconciler 和 fallback 后投影修复尚未实现。引入时需新增可迁移
  数据字段、幂等 confirm/compensate Lua、单 leader 扫描和覆盖所有崩溃窗口的集成测试。

## 发布前验证

- 运行后端 vet、测试和 server/migrate 构建。
- 运行前端格式检查、测试和生产构建。
- 运行 `docker compose config --quiet`。
- 在完整环境运行 `non_ai_smoke.ps1`、`ai_acceptance.ps1`、`research_acceptance.ps1` 和
  `template_ai_event_acceptance.ps1`；模板/活动变更还要运行
  `template_ai_event_redis_failure_acceptance.ps1` 和 `ai_event_concurrency_acceptance.ps1`。
- 验证个人知识库：上传 `.md` / `.zip`、配额与并发预占、文档删除后退出检索、笔记知识开关、
  混合问答的来源保存与 `KNOWLEDGE_NO_EVIDENCE`、跨租户 404 隔离。
- 确认新实例能由 `backend/db/schema.sql` 基线加版本化迁移完成初始化（当前共 51 张表），
  且 `/recipes`、`/assistant` 均重定向到 `/knowledge`。
- 确认仓库不再包含 `/api/v1/recipes/*` 路由与忌口、时区偏好字段。

## 非当前范围

- 团队知识库、云盘同步、计费和桌面组件。
- 数据库与 Markdown 双向同步。
- 管理员审核公开模板；模板上架与下架由作者自主决定。
