# 本机开发与验收

## 环境与启动

需要 Docker Desktop（Linux containers）、与 `backend/go.mod` 匹配的 Go，以及 Node.js 20 或以上。
完整应用启动与配置见[项目 README](../../README.md)。真实 AI 使用 Compose 内 LiteLLM，供应商 Key 仅配置在网关。

项目提供两套不同用途的 Compose：

| 文件 | 用途 |
| --- | --- |
| `docker-compose.yml` | 完整本地产品栈，数据库等内部服务不发布宿主机端口 |
| `docker-compose.ci.yml` | 隔离测试依赖，通过本机回环端口供 Go 集成测试连接 |
| `docker-compose.production.yml` | 生产配置约束与不可变镜像 overlay |
| `docker-compose.local.yml` | 本机模型调试端口 overlay |

不要将 CI 的固定测试凭据用于生产。测试库端口为 55432，Redis 为 56379，MinIO 为 59000。

## 数据库集成测试

从仓库根目录运行：

```powershell
./backend/scripts/local_integration_test.ps1
```

脚本启动隔离依赖，初始化私有桶与 Kafka Topic，运行全部迁移、vet、无缓存全量 Go 测试和 server 构建。
数据库应用连接使用 `cortex_app`，迁移与 claim 使用 `cortex_migrator`。
新库期望版本 42、57 张 public 表；需要真实 PostgreSQL/Redis/MinIO 的测试不能因变量缺失而跳过。
测试服务和数据卷默认保留供复查，重复执行会复用它们。

单独运行某组测试时：

```powershell
. ./backend/scripts/local_test_env.ps1
Set-Location backend
go test -count=1 -v ./internal/store ./internal/rediscoord ./internal/blobstore
```

Windows 用户环境变量修改仅对新进程生效；已有终端应使用上述 dot-source 加载。

## 前端与真实浏览器

```powershell
Set-Location frontend
npm ci
npm run format:check
npm run test:coverage
npm run build
npx playwright install chromium
$env:E2E_REAL_BACKEND = '1'
npm run test:e2e
```

真实 E2E 需要后端已在 `127.0.0.1:8000` 就绪；默认 Playwright 启动 4173 端口的开发前端。
若使用本机已有 Chrome，可设置 `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH` 为实际绝对路径。

## HTTP、AI 与恢复

完整 Compose 服务健康后，在仓库根目录执行：

```powershell
./backend/scripts/non_ai_smoke.ps1
./backend/scripts/ai_acceptance.ps1
./backend/scripts/production_acceptance.ps1
```

AI 验收需要已配置的真实 LiteLLM；以上脚本通过不等于完成全部生产发布门禁。
模板/活动、恢复、容量、安全扫描的要求见[发布检查清单](../RELEASE_CHECKLIST.md)。
最近已执行范围见[本机验收记录](../operations/LOCAL_INTEGRATION_ACCEPTANCE_20260907.md)。

## 产物与清理

`artifacts/`、`frontend/dist/coverage/test-results/`、`backend/*.exe` 是本地产物，不应提交。
保留需要复查的原始产物时先归档到项目外；文档仅保存脱敏摘要与运行条件。
`.env`、依赖环境、公开评测夹具和 Docker 数据卷不是自动清理对象。
