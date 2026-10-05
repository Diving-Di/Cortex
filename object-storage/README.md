# MinIO 构建

MinIO 和 mc 原先的 Quay、Docker Hub 预编译镜像均已在 CI 返回公开拉取拒绝。
这里直接从官方源码的固定发布提交构建，保持原有服务版本与 S3 行为；不依赖第三方重打包镜像。

| 组件 | 原始版本 | 源码提交 |
| --- | --- | --- |
| MinIO | RELEASE.2025-07-23T15-54-02Z | ee211f1b5dbd8f4c4d4e3b8be1d4aa37eef9977e |
| mc | RELEASE.2025-07-21T05-28-08Z | 82c5d2530428bea3897490e7030004ee3b011e3a |

Compose 自动构建 `server` 和 `client` targets，生成本地镜像
`cortex-minio:RELEASE.2025-07-23T15-54-02Z` 与 `cortex-mc:RELEASE.2025-07-21T05-28-08Z`。
独立执行恢复验收前可运行 `docker compose -f docker-compose.ci.yml build minio minio-init`；
恢复脚本也支持 `-MinIOImage`、`-MinIOClientImage` 指定已审查的镜像。

构建需要访问 GitHub、Go module proxy 和 Alpine 软件源。依赖按上游 go.sum 校验，源码提交固定；
镜像保留上游 AGPL 许可证和源码地址。首次构建比拉取预编译镜像耗时更长，后续可复用本机构建缓存。
