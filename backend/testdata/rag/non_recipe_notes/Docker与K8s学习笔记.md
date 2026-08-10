# Docker 与 Kubernetes 学习笔记

## Docker 核心概念

### 镜像 vs 容器 vs 数据卷

- **镜像（Image）**：只读模板，包含了运行应用所需的文件系统和配置
- **容器（Container）**：镜像的运行实例，有独立的读写层，容器删除后数据丢失
- **数据卷（Volume）**：持久化存储，独立于容器的生命周期

### Dockerfile 最佳实践

```dockerfile
# 多阶段构建示例
FROM golang:1.22-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o server ./cmd/server

FROM alpine:3.20
RUN adduser -D appuser
COPY --from=builder /build/server /app/server
USER appuser
CMD ["/app/server"]
```

关键实践：
- 使用特定版本的基础镜像，避免 `latest` 标签
- 利用 Dockerfile 的缓存机制：`COPY go.mod` 在 `COPY .` 之前，依赖变更不会导致重新下载
- 多阶段构建减少最终镜像体积（Go 编译环境 ~800MB，最终 alpine 镜像 ~20MB）
- 使用非 root 用户运行应用

### Docker Compose 的关键配置

```yaml
services:
  app:
    build:
      context: ./app
      args:
        GOPROXY: ${GOPROXY:-https://proxy.golang.org,direct}
    cpus: 2.0
    mem_limit: 2g
    pids_limit: 256
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8000/healthz"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 30s
```

## Kubernetes 入门

### 核心对象

| 对象 | 作用 | 类比 |
|------|------|------|
| Pod | 最小部署单元，包含一个或多个容器 | Docker 容器 |
| Deployment | 管理 Pod 的副本和滚动更新 | Docker Compose 的 service |
| Service | 为 Pod 提供稳定的网络入口 | 负载均衡器 |
| ConfigMap | 非敏感配置 | .env 文件 |
| Secret | 敏感信息 | 加密的 .env |
| PersistentVolume | 持久化存储 | Docker Volume |

### 资源限制

```yaml
apiVersion: v1
kind: Pod
spec:
  containers:
  - name: app
    image: cortex:latest
    resources:
      requests:
        memory: "256Mi"
        cpu: "500m"
      limits:
        memory: "1Gi"
        cpu: "2000m"
```

`requests` 是调度器使用的保证资源，`limits` 是容器能使用的硬上限。requests 小于 limits 时，CPU 可以在有剩余时 burst 到 limits，但 OOM 仍以 limits 为准判死刑。

### 健康检查

```yaml
livenessProbe:
  httpGet:
    path: /healthz
    port: 8000
  initialDelaySeconds: 30
  periodSeconds: 10
readinessProbe:
  httpGet:
    path: /readyz
    port: 8000
  initialDelaySeconds: 10
  periodSeconds: 5
```

关键区别：liveness 失败会重启容器，readiness 失败只会把 Pod 从 Service 摘除但不重启。

## 我们为什么选择 Docker Compose 而不是 K8s

Cortex 目前是单机部署，用户量在百级。Docker Compose 够用：

- 不需要多节点调度（K8s 的最小集群也需要 3 台机器）
- 不需要自动扩缩容（流量稳定）
- 不需要服务发现和负载均衡（所有服务在同一个 compose 网络中）
- 运维成本低（docker compose up 一条命令 vs K8s 需要几台机器 + etcd + 证书管理）
