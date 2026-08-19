# AI 点数活动正式压测报告（2026-08-16）

## 1. 结论

本轮在本地 Docker Compose 单后端实例环境完成 Redis 原子协调、HTTP 全链路并发抢购、k6 并发阶梯、数据库一致性和 Redis 故障降级验收。

- 1,000 个独立用户同时抢 100 个名额时，结果严格为 100 次成功、900 次 `AI_EVENT_SOLD_OUT`，无超卖、无重复发点、无非预期响应。
- k6 1,000 VU 场景在约 1.5 秒内完成 1,000 个请求，吞吐约 646 req/s；HTTP p95 为 1.05 秒、p99 为 1.35 秒、最大 1.42 秒。
- 100/250/500/1,000 VU 四档均为 0% 非预期错误，数据库最终事实始终与库存和点数账本一致。
- Redis 断开时，活动详情进入暂停态、模板读取回退 PostgreSQL、领取进入受限数据库 fallback；故障验收通过，Redis 和后端随后恢复 healthy。

当前实现通过了并发正确性和本地单实例容量验收。本文结果不能直接外推为生产 SLO；生产容量还需在目标规格、多后端实例和真实网络入口下复测。

后续追加的 100,000 请求、2,000 VU、10% 不合格用户混合测试未通过可用性和延迟阈值：失败率 1.845%、p95 19.84 秒、p99 22.01 秒。该扩展场景仍保持零超卖、零重复点数和零不合格用户获奖，但证明当前单实例不能承诺承受该突发规模。

## 2. 测试环境

| 项目 | 配置 |
|---|---|
| 日期 | 2026-08-16，Asia/Shanghai |
| 部署 | Docker Compose，单 backend 实例 |
| PostgreSQL | PostgreSQL 16 + pgvector |
| Redis | Redis 7.4.2，AOF everysec，noeviction |
| Redis 客户端 | 项目内有界连接池，最大 64 个连接 |
| 负载工具 | k6 v2.2.0，Windows amd64 |
| HTTP 入口 | `http://127.0.0.1:8000` |
| 压测用户 | 每轮 1,000 个独立 `flash-*` 用户与 Token |
| 资格数据 | 每用户连续 5 天合格笔记 |
| 活动库存 | 100 |
| AI 服务 | PowerShell 正确性验收期间停止 LiteLLM，确认领取不依赖模型 |

生产默认的认证前 IP 限流是 60 次/分钟。为避免单台 k6 客户端只测到 429，本轮在隔离环境把 `AI_EVENT_CLAIM_IP_LIMIT` 临时设为 1,200；全部测试结束后已恢复并验证为 60。每用户 3 次/分钟的限流未关闭，压测使用独立用户。

## 3. 测试变更

为使测试可重复并避免误判，本轮完成以下改造：

1. `AI_EVENT_CLAIM_IP_LIMIT` 改为配置项，默认值保持 60。
2. `ai_event_concurrency_acceptance.ps1` 支持参与人数、库存、准备并发和抢购并发参数，默认执行 1,000 用户抢 100 名额。
3. 活动开放时间设置到资格笔记创建之后，避免当天笔记因时间边界被判为不合资格。
4. Redis 投影清理显式携带认证信息。
5. 409 响应按稳定错误码分类，只把 `AI_EVENT_SOLD_OUT` 计入售罄。
6. 新增 `backend/loadtest/ai_event_claim.js`，输出成功、售罄、异常和 HTTP 延迟指标。

改造前的首次 12 抢 10 验收得到 9 成功、3 个 `AI_EVENT_INELIGIBLE`。根因是测试把活动开放时间设置为当前时间之前，导致部分刚创建的当天资格笔记落在开放边界之后；这不是库存超卖。修正夹具后 12 抢 10 和后续 1,000 抢 100 均通过。

## 4. Redis 原子协调测试

`TestRedisCoordinationIntegration` 使用独立临时 Redis，覆盖：

- 10,000 个合资格用户竞争 10 个名额；
- Lua 原子扣减；
- 重复领取；
- 活动未开始、已结束和资格不足；
- active version CAS 切换；
- pending reservation；
- 数据库失败后的库存补偿；
- 孤儿 pending 回收；
- 排行、UV、缓存和限流协调。

结果：测试套件通过，总耗时 0.485 秒；10,000 次竞争中成功数严格为 10。

该耗时是本地 Redis 集成套件总耗时，不等于线上 Redis 命令 p99。

## 5. PowerShell HTTP 正确性验收

参数：

```text
Participants       1000
Slots              100
PrepareConcurrency 32
ClaimConcurrency   1000
```

两次 1,000 用户并发正确性验收均通过：

| 轮次 | 成功 | 售罄 | Claim | 点数 Grant | AI Job | 客户端总耗时 |
|---:|---:|---:|---:|---:|---:|---:|
| 1 | 100 | 900 | 100 | 100 | 0 | 27.11 s |
| 2 | 100 | 900 | 100 | 100 | 0 | 21.15 s |

PowerShell 总耗时包含 1,000 个 runspace 的创建与调度，只用于正确性验收，不作为服务端延迟指标。

## 6. k6 HTTP 并发阶梯

每档均执行 1,000 次请求；每档前清空压测用户的 claim、reservation、点数账本与账户，重置活动为 100 个名额，清理 Redis 活动投影和本轮限流键，再等待投影重建。

| 配置 VU | 请求数 | 成功 | 售罄 | 非预期 | 吞吐 | 平均 | 中位数 | p95 | p99 | 最大 |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 100 | 1,000 | 100 | 900 | 0 | 1,001 req/s | 64.77 ms | 10.30 ms | 547.43 ms | 915.60 ms | 990.20 ms |
| 250 | 1,000 | 100 | 900 | 0 | 792 req/s | 144.73 ms | 47.35 ms | 783.90 ms | 1.13 s | 1.25 s |
| 500 | 1,000 | 100 | 900 | 0 | 698 req/s | 291.46 ms | 229.78 ms | 877.84 ms | 1.33 s | 1.38 s |
| 1,000 | 1,000 | 100 | 900 | 0 | 646 req/s | 678.81 ms | 653.58 ms | 1.05 s | 1.35 s | 1.42 s |

所有档位均满足本轮阈值：

```text
http_req_failed < 0.1%
p95 < 5 s
p99 < 10 s
claim_unexpected == 0
claim_accepted == 100
```

100 VU 档吞吐更高，是因为 100 个成功请求完成数据库事务后，其余 900 个请求大多由 Redis 快速返回售罄。随着并发增大，客户端、后端认证、Redis 连接池和数据库事务等待共同推高延迟；1,000 VU 仍未出现错误或超卖。

机器可读 k6 摘要保存在本地忽略目录：

```text
artifacts/ai-event-k6-100-summary.json
artifacts/ai-event-k6-250-summary.json
artifacts/ai-event-k6-500-summary.json
artifacts/ai-event-k6-summary.json
```

包含明文 Token 的临时 `ai-event-k6-users.json` 已在测试后删除，不进入仓库或报告。

## 7. 最终一致性结果

最后一档完成后直接查询 PostgreSQL：

```text
total_slots             100
claimed_slots           100
distinct claim tenants  100
reward grant ledgers    100
duplicate claim groups  0
invalid inventory rows  0
```

因此满足：

```text
claimed_slots == distinct claims == reward grants == total_slots
```

压测后的空闲资源快照：

| 服务 | CPU | 内存 |
|---|---:|---:|
| backend | 0.17% | 29.59 MiB |
| PostgreSQL | 1.23% | 159 MiB |
| Redis | 0.70% | 14.67 MiB |

这是测试结束后的单点快照，不代表峰值资源。下一轮应通过 Prometheus/Grafana 或 Docker stats 采样保存整个测试窗口的 CPU、内存、连接池等待和 Redis ops/s。

## 8. Redis 故障验收

执行 `template_ai_event_redis_failure_acceptance.ps1`，在停止 Redis 并重启 backend 后验证：

```json
{"Status":"passed","TemplateFallback":true,"ClaimDatabaseFallback":true,"ClaimStatus":409,"ReadinessPaused":true}
```

- 活动详情可读且 readiness 为暂停；
- 模板列表通过 PostgreSQL fallback 返回；
- 领取进入受限数据库 fallback，返回业务 409；
- Redis 重启后 backend 恢复 healthy。

## 9. 验证命令

本轮执行并通过：

```powershell
go test ./internal/rediscoord ./internal/server ./internal/store
go test ./internal/config ./internal/server
docker compose config --quiet
./backend/scripts/ai_event_concurrency_acceptance.ps1 `
  -Participants 1000 -Slots 100 -PrepareConcurrency 32 -ClaimConcurrency 1000
./backend/scripts/template_ai_event_redis_failure_acceptance.ps1
k6 run ./backend/loadtest/ai_event_claim.js
```

压测改动完成后已按仓库规范执行并通过完整 `go vet ./...`、`go test ./...` 和 `go build ./cmd/server`，同时通过 `docker compose config --quiet`。

## 10. 风险与后续建议

1. 当前是单 backend、本地回环网络，不能直接外推生产多实例容量。
2. 本轮是 1,000 请求的瞬时抢购，不是 30 分钟以上的恒定到达率或 2 小时 soak test。
3. 当前服务指标以累计 Counter 为主，缺少 HTTP、Redis、数据库事务和连接池等待 Histogram。
4. 应在目标环境增加 10/30 分钟恒定 500、1,000 RPS 测试，并采集 p50/p95/p99、CPU、GC、DB locks、Redis latency 和连接池等待。
5. 应增加压测期间 Redis 注入 50/100/500 ms 延迟、后端滚动重启和 Redis 预扣后数据库事务失败的 HTTP 级故障测试。
6. 压测环境应使用专用用户数据准备程序和自动清理事务，避免长期积累 `flash-*` 用户和笔记。

在本轮测试范围内，AI 点数领取链路达到“零超卖、零重复点数、零非预期响应，1,000 VU 下 p99 1.35 秒”的本地单实例验收结果。

## 11. 100,000 请求混合资格扩展测试

同日继续执行更高压力的 HTTP 混合场景：

```text
总请求             100,000
合资格用户          90,000（90%）
不合资格用户        10,000（10%）
活动名额            10,000
k6 VU               2,000
后端实例            1
```

数据准备不经过公开注册接口，而是通过 `prepare_ai_event_100k.sql` 在单事务内批量创建隔离用户、租户、Token 摘要和 450,000 条资格笔记。原始 Token 根据仅存于本轮进程内存的随机运行密钥生成；领取仍完整经过 HTTP、Token 认证、租户校验、限流、Redis Lua 和 PostgreSQL 最终事务。

预期结果为：

```text
成功              10,000
售罄              80,000
不合格            10,000
非预期                 0
```

实际结果：

| 指标 | 结果 |
|---|---:|
| 总请求 | 100,000 |
| 成功 | 10,000 |
| 售罄 | 78,217 |
| 不合格 | 9,938 |
| 非预期 | 1,845 |
| 吞吐 | 201.46 req/s |
| 平均延迟 | 9.89 s |
| 中位数 | 8.86 s |
| p90 | 15.90 s |
| p95 | 19.84 s |
| p99 | 22.01 s |
| 最大延迟 | 23.25 s |
| HTTP 失败率 | 1.845% |

该场景未通过目标阈值。1,845 个非预期响应按后端访问日志分为：

```text
HTTP 503    1,806
HTTP 500       39
```

39 个 500 均能在服务日志中对应到 `context deadline exceeded`。压测期间 AI 活动投影每分钟重建，观测到 7 次领取与 active version 切换碰撞；1,806 个 503 与持续高压下的投影切换/繁忙拒绝高度相关。后续应在响应指标中继续细分 `AI_EVENT_BUSY`、`AI_EVENT_UNAVAILABLE` 和其他 503，不能仅依赖状态码推断。

资源采样峰值：

| 服务 | 观测 CPU 峰值 | 观测内存峰值 |
|---|---:|---:|
| backend | 约 198% | 约 168 MiB |
| PostgreSQL | 约 170% | 约 561 MiB |
| Redis | 约 92% | 约 167 MiB |

前10,000个成功领取期间 PostgreSQL 是主要 CPU 热点；库存耗尽后，压力转移到 backend 的 HTTP/认证处理和 Redis 缓存、限流与快速拒绝路径。

虽然性能和可用性阈值失败，但最终业务一致性仍然通过：

```text
total_slots            10,000
claimed_slots          10,000
distinct claims        10,000
reward grants          10,000
不合格用户成功数             0
重复领取组                   0
非法库存记录                 0
```

结论：当前单实例配置在 2,000 VU、100,000 次混合请求下仍能守住库存和点数账本，但不能满足低错误率和延迟目标。下一步优化重点应是：

1. 避免活动开放期间无差别按分钟重建并切换同一活动投影，或让领取在版本切换下具有更稳定的重试/读路径。
2. 为认证、Redis协调和数据库事务分别增加延迟直方图，定位约20秒 p95 的排队组成。
3. 检查默认 PostgreSQL 连接池规模与 statement timeout 在10,000个成功事务下的匹配关系。
4. 在修复前不要宣称该单实例支持100,000请求突发流量；当前可证明的是一致性安全，而不是可用性达标。

机器可读结果保存在 `artifacts/ai-event-k6-100k-summary.json`。

测试结束后已撤销本轮10,000条 Claim、Reservation、20,000条相关点数 Ledger和10,000个点数账户，活动恢复为10个名额、0个已领取，认证前 IP 限流恢复为60。物理删除10万用户和45万笔记时，第一次被 `audit_logs.user_id` 外键检查拦截；调整顺序后第二次仍在 `notes.created_by` 外键检查上持续超过10分钟。两次删除事务均被取消并完整回滚，后端随后恢复 healthy。因此隔离的 `loadtest-100k-*` 用户、Token摘要和资格笔记仍保留在本地压测数据库中，原始Token运行密钥已销毁，不能再用于认证。这同时暴露了 `notes.created_by` 等外键缺少适合批量租户清理的索引/清理路径；后续应使用可整体销毁的专用压测数据库，或增加有索引、可审计的批量清理流程。

## 12. 1～7 项优化实施与复测

针对第 11 节暴露的问题，完成了以下实现：

1. worker 在活动开放且 active version 存在时冻结投影，不再每分钟重建和切换；Redis 投影丢失时仍允许修复。
2. 增加领取总耗时、限流、Redis 预占、数据库落账的累计耗时/次数，以及 Redis、fallback busy、数据库超时分类指标；增加 pgx 连接池状态和等待指标。
3. HTTP 服务把市场缓存、认证/限流、活动领取拆为独立 Redis client，避免代码层职责共用；worker 继续使用独立 client。
4. 新增 `ai_flash_event_eligibilities` 资格快照，claim 最终事务读取快照，不再为每次领取扫描笔记；快照启用 FORCE RLS。
5. 投影准备阶段批量创建或滚动 AI 点数账户，领取成功路径不再承担首次建户成本。
6. `auth_tokens.last_used_at` 改为非阻塞入队、1 秒去重批量更新，队列满时允许丢弃非关键 touch。
7. Compose 单实例默认 `DB_POOL_SIZE` 从代码默认 5 显式调整为 20，并暴露 acquire/empty-acquire 指标；新增 `notes.created_by`、`audit_logs.user_id` 索引。

资格快照首次复测发现 RLS policy 使用了错误的 session setting 名称 `app.tenant_id`，而项目统一设置的是 `app.current_tenant_id`。该问题会使数据库最终事务把所有 Redis 已判定合格的用户再次判为不合格。迁移 `000033` 已修正 policy，并用单用户 HTTP 领取验证成功后重新从零复测。

修正后的 100,000 请求复测结果：

| 指标 | 优化前 | 优化后 |
|---|---:|---:|
| 总请求 | 100,000 | 100,000 |
| 客户端确认成功 | 10,000 | 9,999 |
| 数据库最终成功 | 10,000 | 10,000 |
| 售罄 | 78,217 | 66,110 |
| 不合格 | 9,938 | 9,677 |
| 非预期 | 1,845 | 14,214 |
| 吞吐 | 201.46 req/s | 201.54 req/s |
| 平均延迟 | 9.89 s | 9.84 s |
| p95 | 19.84 s | 22.71 s |
| p99 | 22.01 s | 23.30 s |
| 最大延迟 | 23.25 s | 30.00 s |

优化后业务一致性仍通过：数据库 `claimed_slots=10,000`、Claim 10,000 条、全部 10,000 条均能关联资格快照，未超卖且无不合格用户获奖。客户端只收到 9,999 个成功，说明有 1 个请求在服务端提交成功后客户端超时，这是典型的结果未知场景，调用方必须使用同一幂等键查询/重试。

本次结果明确表明：投影版本碰撞已经消失（开放期冻结指标持续增长、version changed 为 0），但单实例总体容量没有提升，且在 2,000 个冷 Token 同时认证时错误率反而更高。连接池观测到 20 个连接全部占用，累计 `empty_acquire` 278,169 次；瓶颈已从投影切换收敛到数据库连接等待、冷认证查询和 10,000 个成功事务对同一活动库存行的串行更新。`DB_POOL_SIZE=20` 不是该硬件上的合格生产值，只是用于暴露下一层瓶颈的保守起点。

因此本轮 1～7 项属于正确性、可观测性和热点扫描消除优化，但没有达到“单实例 100,000 突发请求低错误率”的容量目标。下一步必须采用到达率模型分别压测热认证与冷认证，并评估认证 Token 预热/本地签名校验、成功落账异步化或分片库存落账、多 backend 实例和 PgBouncer；在完成这些架构变化前，不应继续单纯增大数据库连接数。

机器可读结果：`artifacts/ai-event-k6-100k-optimized-rerun.json`。测试完成后已删除 10,000 条 Claim/Reservation/奖励 Ledger，撤销 100,000 个短期测试 Token，活动恢复为 10 个名额、0 个已领取，并恢复生产默认 IP 限流 60。

## 13. 10,000 合格 UV + 1,000 不合格 UV 重试（2026-08-19）

按活动目标用户口径重新执行一次 HTTP 全链路混合领取：

```text
总请求             11,000
合格用户            10,000
不合格用户           1,000
活动名额            10,000
k6 VU                2,000
后端实例                 1
数据库连接池            20
```

测试沿用 `prepare_ai_event_100k.sql` 和 `ai_event_claim_100k.js`。11,000 个用户均使用独立短期 Token 和幂等键；仅前 10,000 个用户创建连续 5 天的合格笔记并进入资格快照。领取完整经过 HTTP、Token 认证、租户/RLS、限流、Redis 预占和 PostgreSQL 最终事务。为避免单台 k6 客户端只测到生产默认的每 IP 每分钟 60 次限制，本地隔离环境临时把 `AI_EVENT_CLAIM_IP_LIMIT` 调整为 20,000，结束后已恢复为 60。

预期与实际响应完全一致：

| 指标 | 预期 | 实际 |
|---|---:|---:|
| 总请求 | 11,000 | 11,000 |
| 成功 | 10,000 | 10,000 |
| 不合格 | 1,000 | 1,000 |
| 售罄 | 0 | 0 |
| 非预期 | 0 | 0 |
| HTTP 失败率 | 0% | 0% |

正确性阈值全部通过，但性能阈值未通过：

| 指标 | 结果 |
|---|---:|
| 吞吐 | 126.40 req/s |
| 平均延迟 | 15.31 s |
| 中位数 | 16.66 s |
| p90 | 17.48 s |
| p95 | 17.70 s |
| p99 | 18.45 s |
| 最大延迟 | 19.70 s |

`p95 < 10 s` 未满足，k6 因阈值失败以退出码 99 结束；`p99 < 20 s`、`http_req_failed < 0.1%` 以及全部响应计数阈值通过。因此本轮结论是“业务正确性通过，容量/延迟不通过”，不能把 10,000 个有效用户同时成功落账解释为该单实例已达到可接受的用户体验目标。

数据库最终事实：

```text
total_slots             10,000
claimed_slots           10,000
claims                  10,000
distinct claim tenants  10,000
reward grants           10,000
eligibility rows        10,000
不合格用户成功数              0
重复领取组                    0
```

服务端累计阶段指标显示，限流阶段 11,000 次合计 105.10 秒，Redis 预占阶段 11,000 次合计 183.16 秒，而数据库最终落账阶段 10,000 次合计 84,653.91 秒，平均约 8.47 秒。数据库池 20 个连接在测试窗口内记录 21,009 次 acquire、21,006 次 empty acquire，累计 acquire 等待 163,908.94 秒。瓶颈仍集中在冷 Token 认证的数据库访问、连接池排队，以及 10,000 个成功事务对同一活动库存行的串行更新，而不是 Redis 资格判断。投影版本切换次数为 0，Redis、fallback busy 和数据库超时错误也均为 0。

测试窗口内 `docker stats` 采样峰值：

| 服务 | CPU 峰值 | 内存峰值（约） |
|---|---:|---:|
| backend | 95.94% | 153 MiB |
| PostgreSQL | 93.73% | 708 MiB |
| Redis | 40.12% | 36 MiB |

机器可读结果保存在：

```text
artifacts/ai-event-k6-11k-summary-1787106752.json
artifacts/ai-event-k6-11k-resources-1787106752.csv
artifacts/ai-event-k6-11k-console-1787106752.txt
```

测试结束后已删除本轮 10,000 条 Claim、10,000 条 reservation、20,000 条点数 Ledger、10,000 个点数账户、10,000 条资格快照、50,000 条日写作统计、50,000 条资格笔记和 11,000 个短期 Token；活动恢复为 10 个名额、0 个已领取，生产默认 IP 限流恢复为 60，backend、PostgreSQL 和 Redis 均恢复 healthy。

物理删除 11,000 个隔离测试用户和租户时，`users` 删除再次因全库 `notes.updated_by` 外键检查超过 5 分钟而取消，事务完整回滚；随后执行上述业务数据清理并成功提交。最终保留 11,000 个无 Token、无笔记、无点数账户和无领取数据的空测试用户/租户。上次已为 `notes.created_by` 增加索引，但本轮表明 `notes.updated_by` 的批量父表删除检查仍是清理瓶颈；后续应补充与外键检查匹配的索引/可审计清理路径，或继续使用可整体销毁的专用压测数据库。

## 14. 认证池、领取舱壁和库存槽位优化复测（2026-08-19）

针对第 13 节的数据库连接池排队、冷 Token 认证和活动库存单行串行更新，完成三项实现：

1. Token 摘要缓存保留 5 分钟短 TTL 和租户版本失效机制，新增同 Token `singleflight` 合并回源；登录签发 Token 时主动写热缓存。冷 Token 数据库回源改用独立 `AUTH_DB_POOL_SIZE=32` 连接池，不再挤占普通业务池。
2. 正常领取落账增加独立舱壁，`AI_EVENT_CLAIM_CONCURRENCY=16`，超过并发预算的请求在队列中有界等待；最终采用 `AI_EVENT_CLAIM_QUEUE_TIMEOUT_MS=10000`，超时稳定返回 `AI_EVENT_BUSY` 并补偿 Redis reservation。
3. 新增 `ai_flash_event_inventory_slots`：每个活动名额对应一条启用 FORCE RLS 的数据库槽位。领取事务使用 `FOR UPDATE SKIP LOCKED` 获取未占用槽位，在同一事务绑定租户和 Claim；数据库唯一约束继续防止重复领取和超卖。`claimed_slots` 改由后台每秒按槽位汇总，不再由每笔事务更新同一活动行。

迁移 `000034_ai_event_inventory_slots` 已在现有本地数据库成功应用。正式复测前，12 个用户并发竞争 10 个名额的 HTTP 正确性闸门通过：10 成功、2 售罄，Claim 和奖励账本各 10。

### 14.1 舱壁调优尝试

首次使用 5 秒队列超时执行相同 11,000 请求场景：

```text
成功              8,493
不合格            1,000
AI_EVENT_BUSY      1,507
p95               5.16 s
吞吐              613.80 req/s
```

该轮证明库存单行热点已经解除且延迟显著下降，但 5 秒不足以吸收 2,000 VU 瞬时突发。全部 1,507 个非预期响应均为舱壁主动返回的 `AI_EVENT_BUSY`，没有超卖、数据库超时或 Redis 错误。清理该轮全部业务数据后，将队列超时调整为 10 秒并从零复测。

### 14.2 最终 10,000 合格 UV + 1,000 不合格 UV 结果

最终配置：

```text
总请求                  11,000
合格用户                 10,000
不合格用户                1,000
活动名额                 10,000
k6 VU                     2,000
普通业务连接池               20
认证连接池                   32
领取落账并发                 16
舱壁队列超时                 10 s
```

响应正确性全部通过：

| 指标 | 预期 | 实际 |
|---|---:|---:|
| 总请求 | 11,000 | 11,000 |
| 成功 | 10,000 | 10,000 |
| 不合格 | 1,000 | 1,000 |
| 售罄 | 0 | 0 |
| `AI_EVENT_BUSY` | 0 | 0 |
| 其他非预期 | 0 | 0 |
| HTTP 失败率 | <0.1% | 0% |

性能结果：

| 指标 | 优化前 | 优化后 | 变化 |
|---|---:|---:|---:|
| 吞吐 | 126.40 req/s | 612.30 req/s | 4.84 倍 |
| 平均延迟 | 15.31 s | 3.08 s | 降低 79.9% |
| 中位数 | 16.66 s | 2.56 s | 降低 84.6% |
| p90 | 17.48 s | 5.28 s | 降低 69.8% |
| p95 | 17.70 s | 5.58 s | 降低 68.5% |
| p99 | 18.45 s | 6.13 s | 降低 66.8% |
| 最大延迟 | 19.70 s | 6.28 s | 降低 68.1% |

最终 k6 退出码为 0，`p95 < 10 s`、`p99 < 20 s`、错误率和全部业务计数阈值均通过。

数据库最终事实也全部一致：

```text
total_slots             10,000
claimed_slots           10,000
occupied inventory      10,000
claims                  10,000
distinct claim tenants  10,000
reward grants           10,000
不合格用户成功数              0
重复领取组                    0
slot/claim drift              0
```

连接池与阶段指标：

| 指标 | 优化前 | 优化后 |
|---|---:|---:|
| 普通池 empty acquire | 21,006 | 16 |
| 普通池累计 acquire 等待 | 163,908.94 s | 1.80 s |
| 认证池 empty acquire | 未独立统计 | 2,287 |
| 认证池累计 acquire 等待 | 未独立统计 | 442.57 s |
| 平均冷认证总耗时 | 未独立统计 | 118.32 ms |
| 领取阶段平均耗时（含舱壁等待） | 8.47 s | 3.12 s |

普通业务连接池等待基本消除，冷 Token 的等待被隔离到认证池。由于本场 11,000 个 Token 全部由 SQL 直接创建、未经过登录写热缓存，因此这是刻意保留的冷认证压力；真实登录签发的 Token 首次业务请求会直接命中缓存。

测试窗口资源峰值：

| 服务 | CPU 峰值 | 内存峰值（约） |
|---|---:|---:|
| backend | 201.05% | 153 MiB |
| PostgreSQL | 314.47% | 927 MiB |
| Redis | 79.87% | 36 MiB |

CPU 峰值高于优化前是因为相同请求在约 18 秒内完成，而非约 87 秒；吞吐提升后单位时间完成了更多认证和数据库事务。该结果仍然只代表本地单 backend、回环网络和当前硬件，不能直接外推生产 SLO。

机器可读结果：

```text
artifacts/ai-event-k6-11k-slots10s-summary-1787109746.json
artifacts/ai-event-k6-11k-slots10s-resources-1787109746.csv
artifacts/ai-event-k6-11k-slots10s-console-1787109746.txt
```

测试结束后已删除本轮和舱壁调优轮的 Claim、reservation、库存槽位占用、点数账本、点数账户、资格快照、资格笔记、短期 Token 及孤儿 Claim outbox；活动恢复为 10 个名额、0 个已领取，IP 限流恢复为 60。最终配置为认证池 32、领取落账并发 16、队列超时 10 秒，backend、PostgreSQL 和 Redis 均为 healthy。本轮生成的空测试用户/租户仍因前述 `notes.updated_by` 外键清理瓶颈保留，但已无 Token、笔记、点数或活动业务数据。

## 15. 10,000 合格 UV + 1,000 不合格 UV 竞争 500 份奖励（2026-08-19）

第 14 节验证的是 10,000 份奖励全部成功落账的极端写入容量，不是活动最终目标库存。本节按纠正后的活动口径重新从零测试：

```text
合格用户             10,000
不合格用户            1,000
总请求               11,000
奖励库存                500
k6 VU                2,000
预期成功                500
预期售罄              9,500
预期不合格            1,000
```

执行前确认数据库中存在 10,000 条资格快照和 500 条库存槽位；1,000 名不合格用户没有资格快照。所有用户使用独立短期 Token 和幂等键，领取仍完整经过冷 Token 认证、租户/RLS、限流、Redis Lua、领取舱壁和 PostgreSQL 最终事务。

响应结果完全符合预期：

| 指标 | 预期 | 实际 |
|---|---:|---:|
| 总请求 | 11,000 | 11,000 |
| 成功 | 500 | 500 |
| 售罄 | 9,500 | 9,500 |
| 不合格 | 1,000 | 1,000 |
| `AI_EVENT_BUSY` | 0 | 0 |
| 其他非预期 | 0 | 0 |
| HTTP 失败率 | <0.1% | 0% |

性能结果：

| 指标 | 结果 |
|---|---:|
| 吞吐 | 639.47 req/s |
| 平均延迟 | 2.76 s |
| 中位数 | 2.57 s |
| p90 | 5.24 s |
| p95 | 5.49 s |
| p99 | 5.83 s |
| 最大延迟 | 6.33 s |

k6 退出码为 0，`p95 < 10 s`、`p99 < 20 s`、错误率和全部业务计数阈值均通过。

数据库最终事实：

```text
total_slots                500
claimed_slots              500
occupied inventory slots   500
claims                     500
distinct claim tenants     500
reward grants              500
不合格用户成功数                 0
重复领取组                       0
slot/claim drift                 0
```

本轮只有 500 个请求进入成功落账事务，其余 9,500 个合格请求由 Redis 快速返回售罄，1,000 个请求返回不合格。普通业务池仅执行 502 次 acquire，`empty_acquire=16`、累计连接获取等待 1.32 秒；没有 capacity busy、数据库超时、Redis 错误或投影版本切换。

测试窗口资源峰值：

| 服务 | CPU 峰值 | 内存峰值（约） |
|---|---:|---:|
| backend | 200.23% | 144 MiB |
| PostgreSQL | 342.13% | 993 MiB |
| Redis | 54.10% | 44 MiB |

机器可读结果：

```text
artifacts/ai-event-k6-11k-500-summary-1787112015.json
artifacts/ai-event-k6-11k-500-resources-1787112015.csv
artifacts/ai-event-k6-11k-500-console-1787112015.txt
```

测试结束后已删除本轮 500 条 Claim、500 条 reservation、1,000 条相关点数 Ledger、10,000 个预热账户、10,000 条资格快照、50,000 条日写作统计、50,000 条资格笔记、11,000 个短期 Token 和 500 条 Claim outbox。活动恢复为 10 个名额、0 个已领取，IP 限流恢复为 60，backend、PostgreSQL 和 Redis 均为 healthy。最终验收结论以本节的“500 份奖励”口径为准。
