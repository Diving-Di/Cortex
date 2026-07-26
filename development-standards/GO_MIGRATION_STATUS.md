# Diary Listener Go 后端状态

> 状态：Go 后端已成为唯一生产候选实现
> 更新日期：2026-07-26

## 当前实现

- [x] 唯一后端目录为 `backend/`，Python 后端已删除。
- [x] Go 1.26、Gin、pgx/v5、PostgreSQL 16。
- [x] 4 空格缩进，Go 文件行首 Tab 为 0。
- [x] 认证、Token、个人租户和 transaction-local RLS。
- [x] 笔记、版本、标签、搜索和 dashboard。
- [x] 附件、Markdown 导出、备份和恢复。
- [x] AI Provider、SSE、整理、报告、引用和回忆。
- [x] 旧聊天和轻日记兼容接口。
- [x] 定时报表和并发安全 scheduler。
- [x] LiteLLM Proxy 和 `diary-default` 逻辑模型。
- [x] 单一 Compose `backend` 服务，监听 8000。
- [x] PostgreSQL 空库初始化基线。

## 已通过验收

| 验收 | 结果 |
| --- | --- |
| `go vet ./...` | 通过 |
| `go test ./...` | 通过 |
| Windows/Linux 构建 | 通过 |
| Go runtime 镜像 | 约 10.9 MB |
| 新 PostgreSQL 空库 | 18 张表、15 条 RLS Policy |
| 空库 ready、注册、登录 | 通过 |
| 双租户笔记隔离 | 跨租户 HTTP 404 |
| 跨租户附件 | HTTP 404 |
| 标签、中文检索、dashboard | 通过 |
| 导出、备份、恢复 | 通过 |
| 租户软删除/恢复 | 403 / active |
| AI 未配置隔离 | AI 503，笔记可写 |
| Kimi 真实 SSE/M2 | 通过 |
| LiteLLM `diary-default` | 通过 |
| 报告与回忆引用回查 | 通过 |
| 定时报表经 LiteLLM | success |
| 双 worker claim | 同一任务仅一条 run |
| Compose 健康 | db、llm-gateway、backend healthy |

验收脚本：

```powershell
.\backend\scripts\non_ai_smoke.ps1
.\backend\scripts\ai_acceptance.ps1
```

## 当前已知限制

- OpenAI 本地账户返回 `429 insufficient_quota`，当前可用主模型为 Kimi。
- LiteLLM 本地阶段使用 master key；生产需要正式虚拟密钥。
- 应用内 token 统计仍为字符估算，准确用量以网关为准。
- `backend/db/schema.sql` 负责空库基线，尚缺版本化增量迁移命令。

## 后续清单

- [ ] 增加版本化增量迁移、advisory lock 和回滚测试。
- [ ] 配置 LiteLLM 管理数据库、虚拟密钥、预算和指标。
- [ ] 贯通应用与网关请求追踪 ID。
- [ ] 增加完整生产安全扫描和备份恢复演练。
- [ ] 按实际部署需求增加 Helm。
