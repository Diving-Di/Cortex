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

已在 2026-08-12 完成：

- Outbox PostgreSQL 集成测试覆盖多 worker 竞争、崩溃租约重领、续租和旧 owner 完成 fencing；新增续租、
  租约丢失和完成 fencing 指标。
- 详情缓存和 negative cache 命中均回 PostgreSQL 校验当前发布版本/状态，不再仅依赖失效消息保证正确性。
- `backend/scripts/marketplace_http_capacity.ps1` 输出 QPS、p50/p95/p99 和失败数；真实线上数字仍须在目标环境运行并归档。
- `backend/scripts/knowledge_capacity.ps1` 已完成 100/1,000/10,000 合成文档 RLS 检索容量验收；尚未覆盖 HTTP、Embedding、Reranker 与 LLM 并发饱和点。
- 联合备份与隔离恢复已在本机通过；观测 RPO/RTO 不是生产 SLA，源卷历史孤儿仍需经产品级保留策略处理，未授权直接删除。

## AI 活动削峰实现

- Redis 不可用时启用受限 PostgreSQL fallback：2 个并发槽、1.5 秒超时、连续三次基础设施失败后熔断
  15 秒；业务 409 不计入熔断。资格先计算，库存以条件更新竞争，点数、claim、reservation 和 Outbox同事务。
- 认证使用 Token SHA-256 摘要 Redis Principal/无效 Token 缓存，校验租户版本，`last_used_at` 最多每
  5 分钟更新；注销和删除租户主动失效缓存。
- claim 路由在认证前执行隐私安全的 IP 摘要限流，认证后执行用户限流。
- 迁移 `000027_auth_and_ai_event_fencing` 增加认证版本和 reservation token/version/state；fallback 成功由
  下一次版本化投影重建修复 Redis。

## 发布前验证

- 运行后端 vet、测试和 server/migrate 构建。
- 运行前端格式检查、测试和生产构建。
- 运行 `docker compose config --quiet`。
- 在完整环境运行 `non_ai_smoke.ps1`、`ai_acceptance.ps1`、`research_acceptance.ps1` 和
  `template_ai_event_acceptance.ps1`；模板/活动变更还要运行
  `template_ai_event_redis_failure_acceptance.ps1` 和 `ai_event_concurrency_acceptance.ps1`。
- 验证个人知识库：上传 `.md` / `.zip`、配额与并发预占、文档删除后退出检索、笔记知识开关、
  混合问答的来源保存与 `KNOWLEDGE_NO_EVIDENCE`、跨租户 404 隔离。
- 验证知识问答质量回流：保存完成/失败结果时只生成脱敏 trace；五类反馈可按 request ID 幂等更新，
  本人复核后可晋升到 draft 数据集，冻结时生成 manifest hash；不存在或跨租户资源返回 404。
  CI 运行合成 fixture 的 schema/hash 门禁，真实检索指标仍在受控发布环境运行。
- 确认新实例能由 `backend/db/schema.sql` 基线加版本化迁移完成初始化（当前共 55 张表），
  且 `/recipes`、`/assistant` 均重定向到 `/knowledge`。
- 确认仓库不再包含 `/api/v1/recipes/*` 路由与忌口、时区偏好字段。

## 非当前范围

- 团队知识库、云盘同步、计费和桌面组件。
- 数据库与 Markdown 双向同步。
- 管理员审核公开模板；模板上架与下架由作者自主决定。
