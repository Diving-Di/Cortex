# Cortex 生产 SLO 与值班契约

> 本文定义可执行的默认门禁，不把本地结果描述为生产 SLA。上线前由服务负责人按目标环境容量确认阈值，
> 并在外部值班系统记录当期 primary/secondary；仓库不保存个人联系方式。

## 服务目标

| 范围 | 30 天目标 | SLI | 停止发布/回滚条件 | 责任角色 |
|---|---:|---|---|---|
| 已认证及公开 API（排除探针） | 可用性 99.5% | 非 5xx 请求 / 全部请求 | 1 小时错误预算燃烧率 > 2 持续 15 分钟 | Cortex service owner / primary on-call |
| 非流式 API | P95 < 1 秒 | HTTP 延迟直方图 | P95 > 1 秒持续 15 分钟 | Backend owner |
| 数据库 ready | 99.9% | `cortex_database_ready` | 连续失败 2 分钟 | Database owner / primary on-call |
| 联合备份 | 每 24 小时成功一次 | 最近成功 Unix 时间 | 超过 25 小时 | Data protection owner |
| 隔离恢复演练 | 每 30 天成功一次 | 最近成功 Unix 时间 | 超过 31 天 | Data protection owner |

SSE 首字节、完整结束率和 AI 上游可用性单独观察，不纳入非 AI API 延迟 SLO。AI 降级不得影响认证、笔记、
搜索、附件和导出。所有标签只使用固定路由模板、方法、状态和组件，不使用 tenant、user、邮箱、正文或 query。

## 值班和告警送达门禁

生产 `ALERTMANAGER_CONFIG_FILE` 必须指向由运维负责人审核的接收器配置。每次变更接收器后注入一条测试告警，
确认 primary 收到、secondary 可升级、恢复通知可见，并把时间、接收器类型和工单号写入发布证据；不得把 webhook、
邮件凭据或个人联系方式提交到仓库。默认配置只在本地 Alertmanager 中保留告警，不能满足生产发布门禁。

严重级别：critical 立即通知 primary，15 分钟未确认升级 secondary；warning 在工作时段处理，若影响数据完整性或
错误预算则升级 critical。处置遵循 `docs/runbooks/`，任何跨租户、RLS 或备份 checksum 异常都必须停止切流。
