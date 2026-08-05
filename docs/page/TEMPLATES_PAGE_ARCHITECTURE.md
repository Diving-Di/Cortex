# 模板广场页

`/notes` 下的“模板广场”是 Markdown 模板的浏览与创作入口：用户可以创建私有模板、自主上架为
公开快照，浏览/点赞/收藏/使用公开模板，并查看榜单。原笔记列表在 `/notes/list`。

## 页面目标、范围与非目标

- 目标：把日记/复盘等结构化 Markdown 沉淀为可复用模板，并通过公开广场分享；使用模板时由
  服务端原子创建笔记并记录可信统计。
- 范围：私有模板 CRUD、作者自主上架/下架、公开模板榜单（推荐/今日热门/近期趋势/最新上架）、
  点赞/收藏/使用/浏览/举报、从模板创建笔记、公开昵称。
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
- 举报：在公开模板上提交举报与原因，举报不自动下架，由后续处理决定。
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
| `POST` | `/api/v1/templates/public/{public_id}/reports` | 提交举报反馈 |

## 后端组件与持久化模型

- 表：`writing_templates`（私有原稿）、`template_publications`（发布记录）、
  `published_template_snapshots`（公开快照）、`template_reactions`（点赞/收藏）、
  `template_usages`（使用记录）、`template_reports`（举报）、`template_public_stats`（聚合榜）、
  `outbox_events`（统计投影事件）。
- Redis：公开详情 Cache Aside、排行榜 ZSet、UV 的 HLL；Outbox worker 幂等投影到排行与统计，
  Redis 清空后从公开统计重建。
- handler 只处理 HTTP 契约；SQL/事务位于 `internal/store`；统计投影由
  `RunMarketplaceWorker` 处理。

## 租户、安全、降级与删除

- 私有原稿受租户 RLS 保护；公开读取只访问脱敏快照，不暴露其他租户的私有原稿、租户 ID、
  登录名或日记内容；跨租户访问私有模板统一 404。
- 从模板创建笔记必须携带幂等键，防止双击创建两篇。
- Redis 不可用时：模板浏览 fail-open（降级 PostgreSQL），不影响私有模板 CRUD 与使用。
- 作者下架或删除租户时立即隐藏公开快照并清理缓存；公开统计只保留匿名聚合。
- 完整备份包含私有模板和个人收藏，不包含公开快照、公共排名、举报与活动数据。

## 测试与验收

- 覆盖：私有模板乐观锁、上架/下架可见性、公开快照隔离、幂等使用、点赞/收藏/使用统计、
  Outbox 幂等消费、Redis 清空后重建、Redis 故障降级。
- 端到端：`template_ai_event_acceptance.ps1`、`template_ai_event_redis_failure_acceptance.ps1`、
  `backup_acceptance.ps1`。
