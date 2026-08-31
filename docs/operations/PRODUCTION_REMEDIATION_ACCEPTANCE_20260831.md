# 上线整改与本地目标栈验收（2026-08-31）

## 结论

基于提交 `6e8b3cf935005632df962f33bb0a0d3183771033` 的未提交整改工作树，本地 Docker Desktop 目标栈已完成仓库内可闭环的发布、运行、业务、恢复和浏览器验收。此结论不等同于真实生产环境上线批准；真实域名/TLS、多节点基础设施、外部告警送达、值班排班、容量成本和法务决策仍需在目标环境完成。

## 已验证结果

- 当前数据库迁移版本 42；57 张 public 表，其中 56 张业务表；44 张租户表启用 RLS 且 44 张强制 RLS。
- `production_acceptance.ps1`：14 个必需服务 healthy，应用容器非 root，内部基础设施无宿主端口，Prometheus target 全部 up，敏感日志模式扫描通过。
- 非 AI smoke：租户、笔记、标签、搜索、Dashboard、附件、跨租户 404、Markdown ZIP 导出与软删除租户拒绝认证通过。
- AI acceptance：LiteLLM 虚拟密钥认证、`cortex-default` 流式聊天、整理草稿/确认写入、报告和租户内来源保存通过。
- 模板和 AI 活动：正常流程与 Redis 停机降级通过；1000 路并发在 28.63 秒内精确得到 100 个成功和 900 个售罄，数据库存在 100 个唯一 claim、0 个错误任务和 100 个奖励 grant。
- 真实 Chrome/Playwright 通过生产 Nginx 镜像完成注册、登录、Dashboard、活动弹窗和退出；后端容器重建后 Nginx 动态 DNS 解析复验通过。
- 后端 `gofmt`、`go vet`、全量测试、22.2% 语句覆盖率（门槛 18%）、RAG regression、server/migrate 构建通过。
- 前端格式、13 个测试文件/30 个用例、52.01% 语句覆盖率和 production Webpack build 通过。
- Prometheus 配置、14 条告警、3 条 SLO 和 Alertmanager 配置通过官方校验工具。
- 生产 overlay 用五个假 digest、HTTPS 示例地址和测试告警路径验证成功；可变 `latest` 镜像验证为 fail-closed。示例值只用于配置逻辑测试，不是生产凭据或生产证据。
- 隔离 PostgreSQL 上迁移 42、schema/RLS、对象 GC 过期租约接管、旧 worker fencing、对象删除后附件清理和软删除附件配额测试连续两轮通过。

## 备份与恢复证据

联合备份目录：`artifacts/release-audit-backup-20260830121316`。

- 备份时迁移版本 41、57 张 public 表、数据库 239625 bytes、应用数据 87 bytes、MinIO 数据 7344 bytes。
- 隔离恢复核对 57 张 public 表、44/44 RLS/FORCE RLS、缺失文件 0、孤儿文件 0。
- 本机观测 RTO 19.075 秒、RPO 65 秒；这些数值只描述本次本地演练，不能作为生产 SLA。
- 迁移 42 是向后兼容的租约列和索引扩展；本轮另在隔离数据库执行并验证，但备份包本身生成于迁移 41，目标环境发布前仍应生成迁移 42 的新备份。

## 本地验收边界

- Reranker 本地目标栈使用 CPU PyTorch 以避免在 Docker Desktop 重复下载 CUDA 13 运行时；发布 Dockerfile 默认仍为 CUDA 13，CUDA 镜像构建和 GPU 推理必须由发布 CI/目标 GPU 节点验证。
- Docker Hub 在本机 DNS 环境不可达，因此 Dockerfile 基础镜像支持 build arg，本地使用 Amazon ECR Public 镜像代理；默认值仍指向官方镜像。
- 本地单节点 PostgreSQL、Redis、MinIO、Redpanda 和 Elasticsearch 不能证明多节点故障切换、复制一致性或生产容量。

## 仍阻断真实生产上线的外部事项

- 配置真实域名、证书、HTTPS 入口、可信代理边界与安全扫描。
- 提供五个由 CI 发布并签名/证明的真实 digest 镜像，完成目标环境部署和回滚演练。
- 配置真实 Alertmanager receiver，验证 primary/secondary、升级与恢复通知实际送达，并确认当期值班人员。
- 在目标规格和真实入口到达率下复测数据库、Redis、对象存储、Kafka、Elasticsearch、Embedding、Reranker、LiteLLM 的容量、成本和故障切换。
- 由业务负责人批准隐私政策、数据保留、备份保留、用户内容处理和供应商数据边界。
