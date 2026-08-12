# 模板广场容量验收记录

日期：2026-08-11。该结果是本地容器网络验收，不代表真实线上 QPS 或用户延迟。

## 环境与口径

- Redis：`redis:7.4.2-alpine`，关闭持久化，独立临时容器；
- 测试端：`golang:1.26`，与 Redis 同处 `cortex_default` Docker 网络；
- 客户端池上限：64；
- 用例：`TestRedisCoordinationIntegration`，包含 10,000 个并发活动预扣、排行版本切换、增量幂等、UV 与限流；
- 计时为整个 Go 测试用例 wall time，不等同于 HTTP p95/p99。

## 可复现命令

```powershell
docker run --rm -d --name cortex-marketplace-acceptance-redis --network cortex_default redis:7.4.2-alpine redis-server --save '' --appendonly no --requirepass acceptance-only-secret
docker run --rm --network cortex_default -e REDIS_TEST_URL='redis://default:acceptance-only-secret@cortex-marketplace-acceptance-redis:6379/0' -v "${PWD}:/workspace" -w /workspace golang:1.26 go test -count=1 -v ./internal/rediscoord
docker stop cortex-marketplace-acceptance-redis
```

## 原始结果摘要

```text
=== RUN   TestRedisCoordinationIntegration
--- PASS: TestRedisCoordinationIntegration (0.50s)
PASS
ok diary-listener/backend/internal/rediscoord 0.506s
```

线上性能数字仍需固定数据集、HTTP 压测客户端、连接池/数据库/Redis 指标和 p50/p95/p99 原始输出后才能发布。
