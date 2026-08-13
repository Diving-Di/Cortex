# 发布 Checklist

- [ ] `gofmt -l .` 无输出，后端 `go vet ./...`、`go test ./...`、server/migrate build 通过。
- [ ] 前端 format check、tests、build 通过；`docker compose config --quiet` 和 Prometheus rules 校验通过。
- [ ] schema migration、RLS/FORCE RLS、低权限 `diary_app` 与跨租户 404 验收通过。
- [ ] non-AI smoke 通过；AI 已配置时受控 AI acceptance 通过，AI 不可用不影响非 AI 功能。
- [ ] 联合备份 manifest/checksum 已生成，最近隔离恢复演练未过期且 DB/文件双向一致。
- [ ] `/healthz`、`/readyz`、全部 Compose 服务健康，磁盘、队列、租约和备份告警无未处置项。
- [ ] RAG regression fixture 与受控评测无回退；真实 bad case 仅在用户复核脱敏后晋升。
- [ ] 容量结果对应当前 commit/环境；未验证的吞吐、成本、RPO/RTO 不写成 SLA。
