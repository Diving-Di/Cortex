# Cortex 发布检查清单

> 更新日期：2026-08-25
> 本文是发布门禁的唯一汇总入口。功能设计见 `README.md`、`docs/SDD.md`、`docs/RAG.md` 和
> `docs/api.md`；历史容量、故障与恢复证据保存在 `docs/operations/`。

## 1. 变更与证据

- [ ] 发布范围、风险、配置变更、迁移版本和回滚方式已记录。
- [ ] 工作树中没有无关改动、密钥、真实私人正文、供应商响应或本地临时产物。
- [ ] 新功能或缺陷修复具有与风险相称的测试，README、API、设计与运行手册已同步。
- [ ] 规划能力与已上线能力严格区分；本地吞吐、成本、RPO/RTO 不被描述为生产 SLA。

## 2. 静态检查、测试与构建

后端：

```powershell
Set-Location backend
gofmt -l .
go vet ./...
pwsh ./scripts/check_go_coverage.ps1 -Minimum 18
go build ./cmd/server
go build ./cmd/migrate
```

- [ ] `gofmt -l .` 无输出，vet、测试和两个构建均通过。
- [ ] 使用外部基础设施的发布额外构建并演练 `blob-migrate`；relay、知识索引、搜索投影与文件 GC
  已由 `cmd/server` 内的受管 runner 承载，不再构建额外部署入口。

前端与 Compose：

```powershell
Set-Location frontend
npm run format:check
npm run test:coverage
npm run test:e2e
npm run build

Set-Location ..
docker compose config --quiet
```

- [ ] 前端格式检查、测试、生产构建和 Compose 配置校验均通过。
- [ ] Prometheus 规则通过 `promtool` 校验，指标和告警保持低基数。
- [ ] Prometheus 所有必需 target 为 up；Alertmanager 使用生产 receiver，测试告警的 primary/secondary 升级与恢复通知送达有证据。
- [ ] 空库 PostgreSQL/Redis/MinIO/Kafka/Elasticsearch 集成 Job 通过，集成测试没有因缺少环境变量跳过。
- [ ] govulncheck、生产 npm audit、pip-audit、Gitleaks、Trivy 文件系统/运行镜像扫描通过并生成 SBOM。

## 3. 数据库、租户与文件安全

- [ ] 新空库完成全部版本化迁移；当前预期迁移版本 41、63 张业务表（连同 `schema_migrations`
  共 64 张 public 表），迁移记录和 schema 基线一致。
- [ ] 租户业务表启用并强制 RLS；`cortex_app` 使用低权限连接，跨租户资源访问表现为 404。
- [ ] 注册、登录、Token 过期/撤销、软删除租户拒绝认证通过验收。
- [ ] 乐观锁、revision、软删除和来源有效性检查没有被绕过。
- [ ] 附件、知识文件与研究资产只使用服务端生成的安全对象定位；MinIO 对象 key、本地回退路径、
  路径穿越、超额和恶意 ZIP 均通过验收。
- [ ] 数据库迁移具有回滚或明确的不可逆说明；执行不可逆操作前已有可恢复备份。

## 4. 核心功能验收

- [ ] `backend/scripts/non_ai_smoke.ps1` 通过；AI、Embedding、Reranker 或 Redis 不可用时，认证、
  笔记、搜索、附件和导出仍可用。
- [ ] AI 已配置时 `backend/scripts/ai_acceptance.ps1` 通过；整理与报告坚持草稿确认，引用只来自当前租户。
- [ ] 研究功能变更时运行 `research_acceptance.ps1`，验证授权、采集、忽略/重试及降级。
- [ ] 模板或 AI 活动变更时运行 `template_ai_event_acceptance.ps1`、
  `template_ai_event_redis_failure_acceptance.ps1` 和 `ai_event_concurrency_acceptance.ps1`。

## 5. 个人知识库与 RAG

- [ ] `.md` / `.zip` 上传、3 GiB 配额、并发预占、恶意路径、删除退出检索和笔记知识开关通过。
- [ ] 索引任务按 `queued/loading/parsing/embedding/persisting/completed/failed` 持久化阶段，块进度单调；
  lease 丢失的旧 worker 不能切换版本或覆盖新进度。
- [ ] 已有活动索引在重建成功前持续服务，失败不会清除旧版本。
- [ ] 知识问答公开 `retrieval_progress` 不包含 prompt、正文块、身份、内部 URL 或上游响应。
- [ ] 来源、反馈、取消、断流、幂等 request ID 和 `incomplete` 回答符合 `docs/api.md`。
- [ ] 无合格证据不调用生成；`ambiguous/scope_conflict/absent` 路由正确。
- [ ] 澄清状态绑定 tenant、user、conversation、原 request ID 和服务端集合范围；正常、重复、过期、
  跨用户/租户、超长补充和恢复后仍无证据均通过验收。
- [ ] `RAG_PLANNER_ENABLED` 默认关闭；如需开启，冻结的 comparison/trend/cross_period 数据集证明其相对
  单查询基线在质量、P95、调用次数和 token 成本上可接受，且子查询数不超过 4。
- [ ] RAG regression fixture、引用核验和受控评测没有不可解释回退；真实 bad case 只在用户复核脱敏后晋升。
- [ ] Markdown/ZIP、PDF、DOC/DOCX、PNG/JPG/WebP 白名单与隔离解析/OCR、资源上限、失败回滚一致；
  Excel 和演示文稿仍被拒绝。

## 6. 运行状态、灾备与容量

- [ ] `/healthz`、`/readyz` 正常；Compose 的 `db`、`redis`、`llm-gateway`、`embedding-service`、
  `reranker-service`、`minio`、`kafka`、`elasticsearch` 和 `backend` healthy。
- [ ] 磁盘、对象容量、连接池、Outbox/Kafka 积压、消费租约、ES 投影延迟、索引失败、AI 断流和备份告警没有未处置项。
- [ ] 联合备份 manifest/checksum 已生成；最近一次隔离恢复演练覆盖 PostgreSQL 和当前引用的数据卷文件，
  DB/文件双向一致。
- [ ] 容量证据对应当前 commit、配置和目标环境，包含失败率、p50/p95/p99 与原始输出。
- [ ] 已复核 `docs/operations/` 中相关报告的适用边界。当前已知限制：单实例冷认证和活动库存热点、
  10,000 文档候选扫描，以及尚未完整测量的 HTTP/Embedding/Reranker/LLM 并发成本。

## 7. 发布与回滚

- [ ] 镜像使用不可变版本；配置、迁移、应用切换和回滚顺序已经演练。
- [ ] 发布镜像具有 SBOM、provenance、attestation 和 digest 证据；部署输入不是可变 tag。
- [ ] 发布后重新运行 ready、认证、非 AI smoke 和本次变更的最小验收。
- [ ] 数据结构按 expand → migrate/backfill → switch → contract 演进；回滚不依赖删除用户数据。
- [ ] 发布观察窗口、负责人和停止条件明确；错误率、P95、队列或租约指标越界时停止并回滚。
- [ ] `docs/SLO.md` 的责任角色已映射到当期 primary/secondary，自动回退和人工回滚证据可定位且不包含密钥。
