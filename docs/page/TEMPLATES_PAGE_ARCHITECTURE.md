# 模板广场页

`/notes` 下的“模板广场”是 Markdown 模板的浏览与创作入口：用户可以创建私有模板、自主上架为
公开快照，浏览/点赞/收藏/使用公开模板，并查看榜单。原笔记列表在 `/notes/list`。

## 页面目标、范围与非目标

- 目标：把日记/复盘等结构化 Markdown 沉淀为可复用模板，并通过公开广场分享；使用模板时由
  服务端原子创建笔记并记录可信统计。
- 范围：私有模板 CRUD、作者自主上架/下架、公开模板榜单（推荐/今日热门/近期趋势/最新上架）、
  点赞/收藏/使用/浏览、从模板创建笔记、公开昵称。
- 非目标：管理员审核、团队协作、公开图片上传、关注/私信/评论，以及把公开内容写入笔记正文
  之外的地方。

## 路由与信息架构

| 路由 | 页面 | 说明 |
| --- | --- | --- |
| `/notes` | 模板广场（默认） | `NotesPage` 根据本地偏好跳转：`TemplatesPage` 或 `/notes/list` |
| `/notes/list` | 我的笔记 | 原 `NoteList` |
| `/notes/:id` | 笔记编辑 | `NoteEditor`，从模板创建后跳转到这里 |

`TemplatesPage` 使用 Tabs 组织：**模板广场**（公开榜单 + 搜索/分类 + 无限滚动）与
**我的模板**（私有 / 已公开 / 已下架）；页面顶部提供“我的笔记”入口。

## 页面区域与交互

- 公开榜单：切换 `recommended` / `daily` / `trending` / `new` 榜单，游标分页滚动加载；
  列表项展示标题、分类、说明与 Markdown 预览，支持点赞、收藏、查看详情和使用模板。
- 浏览上报：首次看到某公开模板时调用 `POST .../views` 上报（仅一次，防重复）。
- 我的模板：创建、编辑（Markdown 编辑与预览）、上架、下架、删除；编辑/上架使用乐观锁版本。
- 公开昵称：上架前必须先设置公开昵称（`/api/v1/public-profile`）。
- 使用模板：确认后调用服务端原子创建接口，成功后跳转 `/notes/{note_id}`。

## 前端数据流

- `useInfiniteQuery(['templates','public', ranking])` 加载公开榜单；
- `useQuery(['templates','mine'])` 加载我的模板；
- `useMutation` 统一处理 publish / withdraw / delete / use-private / use 等操作，成功后刷新
  列表并（对 use）跳转到新笔记。
- 所有失败展示后端返回的稳定 `message`，不展示内部细节。

## HTTP API

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` / `PUT` | `/api/v1/public-profile` | 读取或设置公开昵称 |
| `GET` / `POST` | `/api/v1/templates/mine`、`/api/v1/templates` | 查询或创建私有模板 |
| `GET` / `PATCH` / `DELETE` | `/api/v1/templates/{id}` | 读取、乐观锁更新或软删除自己的模板 |
| `POST` | `/api/v1/templates/{id}/publish` | 作者上架当前版本并生成公开快照 |
| `POST` | `/api/v1/templates/{id}/withdraw` | 作者下架公开快照 |
| `POST` | `/api/v1/templates/{id}/use` | 携带 `Idempotency-Key` 从自己的模板原子创建笔记 |
| `GET` | `/api/v1/templates/public` | 公开模板列表；支持 `ranking`、`category`、`query`、游标 |
| `GET` | `/api/v1/templates/public/{public_id}` | 公开模板详情 |
| `PUT` / `DELETE` | `/api/v1/templates/public/{public_id}/like` | 点赞 / 取消点赞（幂等） |
| `PUT` / `DELETE` | `/api/v1/templates/public/{public_id}/favorite` | 收藏 / 取消收藏（幂等） |
| `POST` | `/api/v1/templates/public/{public_id}/use` | 携带 `Idempotency-Key` 原子创建笔记 |
| `POST` | `/api/v1/templates/public/{public_id}/views` | 上报有效浏览（可采样） |
| `GET` | `/api/v1/templates/public/{public_id}/stats?day=YYYYMMDD` | 查询指定日期匿名 UV |

## 后端组件与持久化模型

- 表：`writing_templates`（私有原稿）、`template_publications`（发布记录）、
  `published_template_snapshots`（公开快照）、`template_reactions`（点赞/收藏）、
  `template_usages`（使用记录）、`template_public_stats`（聚合榜）、
  `outbox_events`（统计投影事件）。
- Redis：公开详情 Cache Aside、排行榜 ZSet、UV 的 HLL；Outbox worker 幂等投影到排行与统计，
  Redis 清空后从公开统计重建。
- handler 只处理 HTTP 契约；SQL/事务位于 `internal/store`；统计投影由
  `RunMarketplaceWorker` 处理。

### 私有原稿、发布记录与公开快照

模板没有直接把租户私有表暴露给公开查询。`writing_templates` 保存作者可编辑原稿并以 `version`
做乐观锁；上架时，Store 在同一事务中写入发布记录和版本化公开快照。快照固化标题、分类、说明、
Markdown 正文和公开昵称，不携带租户 ID、登录名或私有笔记数据。作者后续编辑原稿不会静默改变已经
发布的版本；使用记录保存快照版本，便于解释笔记来源。

### 事务、幂等与 Outbox

- 点赞和收藏明细由 PostgreSQL 唯一约束保证幂等，聚合计数与 Outbox 事件和业务变更同事务提交。
- 从私有或公开模板创建笔记时，`Idempotency-Key` 在租户内唯一；笔记、模板使用记录、统计、审计和
  Outbox 同事务写入，客户端超时重试不会创建第二篇笔记。
- Marketplace Outbox worker 只领取 `aggregate_type='template'`，以 `FOR UPDATE SKIP LOCKED` 和 30 秒
  租约领取事件，处理期间周期续租。完成或退避必须同时匹配 owner、未过期租约和未完成状态；Redis
  以 event ID 去重并写入 PostgreSQL 计算的绝对分数，避免 at-least-once 重放造成计数漂移。

### 排行、详情缓存与回表校验

- `new`、`trending`、`daily` 使用 Redis ZSet；`recommended` 仍由 PostgreSQL 生成个性化结果。
- `new` / `trending` 使用版本键离线分批构建，通过 `active_version` expected-old CAS 原子切换；读取与
  增量投影都先解析当前 active 版本，重建期间继续读取完整旧榜。trending 分数采用 7 天半衰期衰减。
- Redis 排行只负责候选排序。命中后仍回 PostgreSQL 校验发布状态、读取公开快照并补齐当前用户的
  点赞/收藏状态，因此不能把它描述为“缓存命中后数据库零访问”。
- 详情采用 Cache Aside；缓存不保存租户态 reaction。下架依靠投影 worker 清理缓存，公开查询回表时
  仍以 PostgreSQL 的发布状态为准。
- 浏览先用包含稳定摘要的 10 分钟 `SET NX` 去重，再异步投影到排行；key 不包含原始 tenant UUID。
  daily ZSet 与 UV HLL 保留 8 天，统计接口使用 `PFCOUNT` 返回指定日期 UV。浏览量属于可接受/采样
  统计，不是严格访问日志；Redis 不可用时返回 `unique_visitors_available=false`，不伪造为零。

## 租户、安全、降级与删除

- 私有原稿受租户 RLS 保护；公开读取只访问脱敏快照，不暴露其他租户的私有原稿、租户 ID、
  登录名或日记内容；跨租户访问私有模板统一 404。
- 从模板创建笔记必须携带幂等键，防止双击创建两篇。
- Redis 不可用时：模板浏览 fail-open（降级 PostgreSQL），不影响私有模板 CRUD 与使用。
- Redis 故障时点赞、收藏和使用等核心事实继续写 PostgreSQL 并保留 Outbox；浏览上报作为非核心统计
  主动丢弃，限流退化为单实例能力。
- 作者下架或删除租户时立即隐藏公开快照并清理缓存；公开统计只保留匿名聚合。

## 测试与验收

- 覆盖：私有模板乐观锁、上架/下架可见性、公开快照隔离、幂等使用、点赞/收藏/使用统计、
  Outbox 幂等消费、Redis 清空后重建、Redis 故障降级。
- 端到端：`template_ai_event_acceptance.ps1`、`template_ai_event_redis_failure_acceptance.ps1`。

## 已知边界与后续改进

1. Outbox 消费隔离、续租和数据库 fencing 已实现；仍需补多 worker 崩溃/抢占的 PostgreSQL 并发测试
   与租约观测指标。
2. 排行双缓冲和下架 best-effort 失效已实现；详情缓存值尚未携带发布版本，安全兜底仍是 PostgreSQL
   发布状态回表校验。
3. Redis 客户端已有最大 64 条连接的复用池，但仍是项目内 RESP 实现；后续可评估成熟客户端的集群、
   telemetry 和连接健康能力。
4. 搜索 SQL 仍使用包含匹配语义，但迁移 000025 已建立对应 trigram GIN 索引；需用生产规模数据执行
   `EXPLAIN (ANALYZE, BUFFERS)` 验证计划稳定性。
5. 本地可复现容量结果见 `docs/MARKETPLACE_CAPACITY_ACCEPTANCE.md`。仓库仍没有真实线上 QPS、HTTP
   p95/p99、缓存命中率证据，不得把本地 Go 测试 wall time 表述为线上性能。

## 代码定位

| 文件 | 职责 |
| --- | --- |
| `frontend/src/features/templates/TemplatesPage.tsx` | 页面、Tabs、无限滚动和交互 mutation |
| `frontend/src/api/templates.ts` | 模板 API 类型和请求 |
| `backend/internal/server/marketplace.go` | HTTP、签名游标、缓存、限流和降级编排 |
| `backend/internal/store/marketplace.go` | RLS 查询、事务、幂等和 Outbox 写入 |
| `backend/internal/store/outbox.go` | 事件租约、退避、绝对投影查询和指标 |
| `backend/internal/server/marketplace_worker.go` | 启动重建与持续消费 |
| `backend/internal/rediscoord/client.go` | ZSet、详情缓存、HLL、Lua 和排行重建 |
| `backend/internal/migrations/sql/000014_template_marketplace_ai_events.up.sql` | 表、索引、约束、RLS 和授权 |
| `backend/internal/migrations/sql/000025_marketplace_hardening.up.sql` | 公开模板 trigram 搜索索引 |
| `backend/internal/migrations/sql/000026_remove_template_reports.up.sql` | 删除模板举报表 |
