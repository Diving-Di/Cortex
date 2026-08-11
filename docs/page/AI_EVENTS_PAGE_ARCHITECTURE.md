# AI 限量活动页

`/ai-events` 是“每日限量 AI 深度月报”活动页面：展示当前/下一场活动的开放时间、剩余名额、
当前用户资格与月度 AI 点数，支持领取名额并跟踪生成任务状态。

## 页面目标、范围与非目标

- 目标：让符合条件的用户抢到每日有限的 AI 深度月报名额，并透明展示资格、点数与生成进度。
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
- 我的领取：已领取后轮询领取状态（`queued` / `running`），完成后显示生成的月报入口。

## 前端数据流

- `useQuery(['ai-event'])` 15 秒轮询 `GET /api/v1/ai-events/current`，页面可见时
  `visibilitychange` 立即刷新。
- `useQuery(['ai-points'])` 读取 `GET /api/v1/ai-points/balance`。
- `useQuery(['ai-event-claim', id])` 在已领取时轮询 `GET .../claims/me`，`queued`/`running`
  每 2 秒刷新。
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
| `GET` | `/api/v1/ai-event-claims/{claim_id}` | 查询任务状态与报告 ID |

## 后端组件与持久化模型

- 表：`ai_point_accounts` / `ai_point_ledger`（点数账本）、`ai_flash_event_settings`（活动参数）、
  `ai_flash_events`、`ai_flash_claims`、`ai_event_jobs`、`tenant_daily_writing_stats`（连续记录）。
- Redis 仅承担高峰原子预扣与快速拒绝：Lua 脚本按 `TIME` 裁决开放时间与库存，预扣成功后才进入
  PostgreSQL；数据库唯一约束、点数账本与 claim/job 状态机保存最终事实。
- Worker 使用有限租约领取任务，成功后自动写入带来源的月报（保留 revision）；最终失败释放冻结
  点数但不返普通名额。人工补发使用 `backend/scripts/grant_ai_event_replacement.ps1`。
- 活动参数集中保存于 `ai_flash_event_settings`：默认 `Asia/Shanghai` 每天 20:00 开放、20:10
  关闭，10 个名额，固定消耗 100 点；不在代码中硬编码。

## 资格与点数规则

- 连续 5 天记录（含活动当天；当天只计 20:00 开场前完成的有效笔记）。
- 每日每用户最多领取一次；领取即视为本次月报生成的明确授权。
- 平台故障导致生成最终失败时返还点数、不返普通名额，可人工补发。

## 租户、安全、降级与删除

- 活动数据按租户 RLS 隔离；成功名单完全匿名。
- 领取 Redis 不可用时 fail-closed（返回 503），避免突发流量冲击数据库库存；活动详情/倒计时
  Redis 不可用时由 PostgreSQL 提供并标记剩余名额不可用。
- `/healthz` 不依赖 Redis；Redis 关键数据可由 PostgreSQL 重建。
- 删除租户时清理活动相关缓存；活动数据不进入公开统计以外的可见范围。

## 测试与验收

- 覆盖：零超卖、零重复扣点、零跨租户来源、补偿可对账、Redis 故障只关领取、连续记录资格、
  每日每用户一次。
- 端到端：`template_ai_event_acceptance.ps1`、`template_ai_event_redis_failure_acceptance.ps1`、
  `ai_event_concurrency_acceptance.ps1`。
