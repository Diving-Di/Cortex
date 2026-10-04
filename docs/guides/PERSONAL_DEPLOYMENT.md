# 个人轻量部署与完整部署

校对日期：2026-10-05。

## 选择部署方式

| 方式 | 启动文件 | 默认服务 | 适用范围 |
| --- | --- | --- | --- |
| 个人轻量版 | `docker-compose.light.yml` | db、backend、frontend | 个人记录、搜索、标签、附件、版本恢复、模板、Markdown 导出 |
| 完整版 | `docker-compose.yml` | PostgreSQL、Redis、MinIO、Redpanda、Elasticsearch、网关、解析与模型服务、监控等 | 完整知识处理和外部基础设施验收 |

轻量版使用 `STORAGE_BACKEND=local`、`EVENT_BUS=postgres`、`RAG_RETRIEVAL_BACKEND=postgres`；
三个服务沿用既有镜像、权限分离、RLS、迁移和健康检查。Redis 未启用，活动与模板走既有数据库降级路径。
轻量版的 `light_db_data`、`light_app_data` 与完整版卷独立，Compose 项目名为 `cortex-light`。
切换文件不会迁移旧数据库、附件或 MinIO 对象。已有用户应先备份并按对象迁移流程处理，不能把切换当作迁移。

## 启动非 AI 核心

复制根目录 `.env.example` 到被 Git 忽略的 `.env`，设置互不相同的数据库应用、迁移和 LiteLLM 数据库密码。
轻量版核心无需供应商 Key、MinIO 或 Elasticsearch 凭据。供应商凭据只供可选网关使用。

```powershell
docker compose -f docker-compose.light.yml config --quiet
docker compose -f docker-compose.light.yml up -d --build db backend frontend
docker compose -f docker-compose.light.yml ps
./backend/scripts/non_ai_smoke.ps1
```

Web 为 `http://127.0.0.1:5173`，API 为 `http://127.0.0.1:8000`。数据库不暴露宿主机端口。
完整栈和轻量栈使用相同 Web/API 端口，不能同时绑定这些端口；运行前确认目标实例和卷。
容器重建保留数据卷；停止时使用 `down`，不要用 `down -v` 清除数据。

## 按需增加 AI 和知识解析

```powershell
docker compose -f docker-compose.light.yml --profile ai up -d llm-gateway
docker compose -f docker-compose.light.yml --profile knowledge up -d document-parser embedding-service reranker-service
```

生成模型仍通过 LiteLLM 的 `cortex-default`。先配置网关主密钥和供应商凭据，在内部网络签发虚拟密钥，
再设置 `.env` 中 `LITELLM_VIRTUAL_KEY` 并重建 backend。不要公开网关管理端口。
签发流程与预算规则见[网关规范](../LLM_GATEWAY.md)。

知识 profile 沿用固定 512 维 GTE 中文 Embedding 与 BGE 精排；轻量版精排使用 CPU 镜像和 CPU 设备，
首次构建下载固定 revision 的模型，需要网络、磁盘和内存，CPU 延迟必须在目标机器测量。
未启动网关或模型服务时，AI/索引返回明确错误，非 AI 核心继续工作。启动可选服务后可在知识页重试索引。

轻量核心的验收应包括 ready、注册/登录、笔记 CRUD、中文搜索、附件、版本恢复、模板、导出和跨租户 404。
知识与 AI profile 需另做真实模型、引用核验、索引重试与故障恢复验收，不能以配置校验代替运行测试。

## 升级和备份

新数据库初始化基线后运行到迁移 42，共 57 张 public 表；本轮体验改进没有增加表或迁移。
既有备份脚本默认面向完整版，轻量版备份应明确指定 `cortex-light` 项目、数据库卷和应用卷，
并在隔离目录恢复验证；不要直接用完整版脚本假设备份了轻量实例。
生产发布继续遵循[发布检查清单](../RELEASE_CHECKLIST.md)，个人轻量版不代表生产规模承诺。
