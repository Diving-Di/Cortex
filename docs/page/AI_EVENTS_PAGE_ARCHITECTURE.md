# AI 限量活动页

`/ai-events` 是每日限量免费 AI 点数活动页面：展示当前/下一场活动的开放时间、剩余名额、当前用户
资格与月度 AI 点数，并支持幂等领取和查询领取结果。

## 页面目标、范围与非目标

- 目标：让符合条件的用户领取每日有限的免费 AI 点数，并透明展示资格、点数与领取结果。
- 范围：活动倒计时（服务端时间校准）、资格与点数展示、领取名额、最近 7 天完全匿名成功名单、
  当前用户历史领取与生成结果。
- 非目标：付费/充值/退款、可提现积分、管理员审核、活动配置在前端硬编码。

## 页面区域与交互

- 活动卡片：名称、开放/关闭时间、固定消耗点数、剩余名额（`remaining_slots_approx`，仅展示）、
  `scheduled` / `open` / `closed` 阶段。
- 倒计时：基于 `server_time` 与本地时钟的偏移量计算，未开始时显示到开场的倒计时，开场后
  显示到关闭的倒计时。
- 资格区：连续记录天数（连续 5 天，含当天且只计 20:00 前完成的笔记）、当月可用点数、
  今日是否已领取。
- 领取：活动开放且符合资格时显示“立即领取”，携带幂等键调用领取接口，成功后提示“月报正在
  生成”。
- 名单：最近 7 天成功名单完全匿名（不显示昵称/账号）。
- 我的领取：领取后查询本场结果和点数到账状态。

## 前端数据流

- `useQuery(['ai-event'])` 15 秒轮询 `GET /api/v1/ai-events/current`，页面可见时
  `visibilitychange` 立即刷新。
- `useQuery(['ai-points'])` 读取 `GET /api/v1/ai-points/balance`。
- `useQuery(['ai-event-claim', id])` 在已领取时查询 `GET .../claims/me`，展示本场领取结果。
- 领取使用 mutation（`POST .../claims`），失败展示后端稳定 `message`。
- 倒计时每秒由本地定时器推进，最终时间判断以服务端 `server_time`/`opens_at`/`closes_at` 为准，
  不用客户端时间作最终裁决。

## HTTP API

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/v1/ai-points/balance` | 当月 grant/held/consumed/available |
| `GET` | `/api/v1/ai-events/current` | 当前/下一场活动、服务端时间、弹窗窗口 |
| `GET` | `/api/v1/ai-events/history` | 最近活动和完全匿名成功名单 |
| `GET` | `/api/v1/ai-events/{event_id}` | 活动详情与当前用户资格 |
| `POST` | `/api/v1/ai-events/{event_id}/claims` | 领取名额，要求 `Idempotency-Key` |
| `GET` | `/api/v1/ai-events/{event_id}/claims/me` | 当前用户本场领取状态 |
| `GET` | `/api/v1/ai-event-claims/{claim_id}` | 查询领取结果 |

## 后端组件与持久化模型

- 表：`ai_point_accounts` / `ai_point_ledger`（点数账本）、`ai_flash_event_settings`（活动参数）、
  `ai_flash_events`、`ai_flash_claims`、`ai_event_jobs`、`tenant_daily_writing_stats`（连续记录）。
- Redis 仅承担高峰原子预扣与快速拒绝：Lua 脚本按 `TIME` 裁决开放时间与库存，预扣成功后才进入
  PostgreSQL；数据库唯一约束、点数账本与 claim 保存最终事实。
- 活动投影采用版本化 Key：builder 分批构建候选版本并在完成后原子切换 active pointer；领取 Lua
  先校验版本指针，切换窗口返回 `AI_EVENT_BUSY`，避免把半成品投影当成完整库存。
- 领取事务写入点数账本并即时增加可用点数，不创建 AI 生成任务或报告。异常补发使用
  `backend/scripts/grant_ai_event_replacement.ps1`。
- 活动参数集中保存于 `ai_flash_event_settings`：默认 `Asia/Shanghai` 每天 20:00 开放、20:10
  关闭，10 个名额，每次赠送 100 点；不在代码中硬编码。

## 资格与点数规则

- 连续 5 天记录（含活动当天；当天只计 20:00 开场前完成的有效笔记）。
- 每日每用户最多领取一次；领取即视为本次月报生成的明确授权。
- PostgreSQL 是领取和点数最终事实；异常场景通过审计和人工补发处理。

## 租户、安全、降级与删除

- 活动数据按租户 RLS 隔离；成功名单完全匿名。
- 领取 Redis 不可用时 fail-closed（返回 503），避免突发流量冲击数据库库存；活动详情/倒计时
  Redis 不可用时由 PostgreSQL 提供并标记剩余名额不可用。
- claim 路由当前仍位于统一认证组内，认证会查询 PostgreSQL；项目尚未实现 Principal Redis 缓存、
  认证前活动 IP 限流或受限 PostgreSQL 完整资格兜底。因此 Redis 未配置、预热缺失或调用失败时继续
  fail-closed，不能把数据库回源方案描述成现有能力。
- `/healthz` 不依赖 Redis；Redis 关键数据可由 PostgreSQL 重建。
- 删除租户时清理活动相关缓存；活动数据不进入公开统计以外的可见范围。

## 测试与验收

- 覆盖：零超卖、零重复扣点、零跨租户来源、补偿可对账、Redis 故障只关领取、连续记录资格、
  每日每用户一次。
- 端到端：`template_ai_event_acceptance.ps1`、`template_ai_event_redis_failure_acceptance.ps1`、
  `ai_event_concurrency_acceptance.ps1`。

## 削峰边界与演进方向

当前正常路径已经做到“Redis 完整投影快速拒绝，只有预扣成功者进入 PostgreSQL 最终事务”，但每次请求
仍先执行 Token/User/Tenant 解析，且 `auth_tokens.last_used_at` 会产生认证写放大。安全演进应分阶段进行：

1. 增加只保存 Token SHA-256 摘要索引的 Principal 缓存、短期无效 Token 负缓存和 `last_used_at`
   限频；缓存 TTL 不得超过 Token 剩余有效期，租户停用、Token 撤销与退出必须使缓存失效。
2. 把活动 IP 限流移到认证之前，用户限流保留在可信 Principal 之后；Redis 故障时只能采用保守的
   单实例应急限流，并由网关/WAF 补全多实例全局限制。
3. 如启用 PostgreSQL fallback，必须使用独立并发舱壁、短队列、事务超时和熔断器，并为普通业务保留
   连接容量；预算耗尽返回 `AI_EVENT_BUSY`，不得无条件旁路 Redis。
4. fallback 事务先做活动快照、幂等、连续记录和点数校验，最后以条件更新竞争库存，避免资格查询期间
   持有活动热点行锁。快路径和 fallback 最终都由 PostgreSQL 唯一约束与同一事务裁决。
5. 若引入内部 `reservation_id` fencing 和 pending 对账，必须先完成迁移、幂等 Lua、leader 租约、
   崩溃点测试与投影重建；客户端 `Idempotency-Key` 不能替代内部 reservation ID。

这些事项记录在 `docs/IMPLEMENTATION_GAPS.md`。上线前至少验证 Redis 断连/超时、预热切换、并发超卖、
重复领取、补偿失败、数据库超时及普通业务连接池隔离。
