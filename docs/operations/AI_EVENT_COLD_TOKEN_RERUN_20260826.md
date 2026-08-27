# AI 限量活动冷 Token 并发复测（2026-08-26）

> 适用边界：Windows、Docker Desktop、本地单 backend Compose 环境。本结果是独立冷 Token 最坏情况
> 回归，不代表真实冷热混合流量、线上用户数或生产 SLA。

## 环境与请求模型

- Git Commit：`6d9649ca6d0592ada7443de977e2d19936471148`
- k6：v2.2.0，单独压测进程
- backend：1 个本地 Compose 实例
- PostgreSQL 16、Redis 7、Redpanda/Kafka
- 11,000 个独立冷 Token：10,000 名合格用户、1,000 名不合格用户
- 活动库存 500，2,000 VU，每名用户只提交一次领取
- 本地单客户端 IP 限流由 60 临时调整为 20,000，结束后恢复为 60

## 正式结果

| 指标 | 结果 | 验收 |
|---|---:|---:|
| 请求数 | 11,000 | 11,000 |
| 成功领取 | 500 | 500 |
| 售罄 | 9,500 | 9,500 |
| 不合格 | 1,000 | 1,000 |
| 非预期响应 | 0 | 0 |
| HTTP 失败率 | 0% | < 0.1% |
| 吞吐 | 870.64 req/s | 记录值 |
| 平均延迟 | 2.062 s | 记录值 |
| P50 | 1.571 s | 记录值 |
| P90 | 4.279 s | 记录值 |
| P95 | 4.871 s | 历史兼容门槛 < 10 s，通过 |
| P99 | < 20 s | k6 阈值通过；本轮 summary 未导出精确值 |
| 最大延迟 | 6.493 s | 记录值 |

数据库最终事实：

```text
configured_inventory=500
claimed_slots=500
occupied_inventory_slots=500
distinct_claim_tenants=500
reward_grants=500
duplicate_claim_tenants=0
duplicate_reward_tenants=0
unprocessed_outbox_events=0
failed_outbox_events=0
```

因此本轮没有超卖、重复 Claim、重复奖励或负库存，Outbox 最终无积压。与 2026-08-25 同口径的
488.32 req/s、P95 9.114 s 相比，吞吐提高 78.3%，P95 降低 46.6%；由于 Docker Desktop 状态和
容器生命周期不同，这组变化只能作为本地回归对照，不能归因为某一代码优化。

## 资源采样

正式请求窗口很短，`docker stats` 仅取得 3 个样本：backend CPU 峰值 203.39%、内存从 133 MiB
升至 146.6 MiB；PostgreSQL CPU 峰值 142.66%、内存约 334.1 MiB；Redis CPU 峰值 70.13%、
内存从 29.26 MiB 升至 42.38 MiB。样本不足以判断持续连接池使用率、泄漏或稳态资源趋势，不能代替
Prometheus 时间序列和 60 分钟稳态测试。

## 无效准备轮与修正

正式轮之前有两类准备错误，均从数据库 0 Claim、0 奖励状态重新开始，不计入容量结果：

1. 未临时提高单客户端 IP 限流，10,940 个请求返回 429；
2. 活动窗口临时前移后，当天资格笔记时间仍晚于新 `opens_at`，合格用户被判为不合格。

正式轮已同时修正限流和 `created_at < opens_at` 的资格时间语义。k6 脚本现显式导出 P99 和最大值，
避免后续 summary 只记录阈值结论。

## 清理与结论

- 11,000 个测试用户、Token、50,000 条资格笔记、Claim、reservation、库存槽、点数记录均已清理；
- 活动恢复为 10 个名额、0 已领取，IP 限流恢复为 60；
- 原始 summary、console、facts、metadata 和资源 CSV 位于本地忽略的
  `artifacts/ai-event-k6-redo-1787715435-final3-*`。

本次“独立冷 Token 认证与热点库存竞争压力测试”并发正确性和历史兼容性能门槛通过。附件方案要求的
真实冷热混合流量、单接口基线、1/2/3 backend、Redis/Kafka 故障、60 分钟稳态和 4 小时耐久测试仍需
分别执行，因此本报告不能作为完整生产容量验收。
