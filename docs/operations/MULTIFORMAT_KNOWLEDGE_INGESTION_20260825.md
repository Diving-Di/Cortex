# 多格式知识库摄取实现记录（2026-08-25）

## 范围

- 顶层文件：Markdown、Markdown ZIP、PDF、DOC、DOCX、PNG、JPG/JPEG、WebP。
- ZIP 兼容行为不变：只摄取 Markdown，并保存其引用的 PNG/JPG 资源。
- PDF 执行逐页文本抽取，无文本页转 OCR；图片执行 `chi_sim+eng` OCR；DOCX 提取段落与表格；DOC 使用 antiword。

## 安全与故障边界

二进制原文件仍经现有 BlobStore/MinIO 私有对象路径保存。Kafka 解析消费者把文件发送给仅在 Compose 内网暴露的 `document-parser`，解析结果以任务版本为边界暂存 PostgreSQL，再通过 Outbox 触发独立 Embedding 消费者；Embedding 完成后触发 Elasticsearch 投影消费者。该解析容器没有数据库、MinIO、Kafka、Elasticsearch、LiteLLM 或供应商密钥，使用非 root、只读根文件系统、空 capabilities、PID/内存/CPU 限制及有界 tmpfs。

上传层校验扩展名与 magic/MIME，并限制文件大小。解析层再次限制大小、PDF 页数、图片像素与 DOCX 解压规模，拒绝加密 PDF、DOCX 宏、损坏或类型不匹配文件。解析响应有界，错误只记录稳定 code，不记录正文或上游响应。

稳定失败码包括 `KNOWLEDGE_FILE_TYPE_MISMATCH`、`KNOWLEDGE_DOCUMENT_UNSAFE`、`KNOWLEDGE_DOCUMENT_EMPTY`、`KNOWLEDGE_OCR_FAILED`、`KNOWLEDGE_PARSER_UNAVAILABLE` 与 `KNOWLEDGE_QUOTA_EXCEEDED`。解析器不可用不会影响既有索引查询、笔记、附件或其他非摄取接口。

## 验证结果

- Go 上传格式/magic 校验、解析客户端契约、错误映射、空结果单测：通过。
- `go vet ./...`、`go test ./...`、`go build ./cmd/server`：通过。
- 前端 Prettier、14 个测试文件/36 项测试、生产构建：通过。
- `docker compose config --quiet`：使用补齐后的真实 `.env` 通过。
- `document-parser` 与 backend 镜像构建、替换：通过；两服务均 healthy，后端 `/healthz`、`/readyz` 均为 200。
- 容器内独立解析冒烟：PDF、DOCX、图片 OCR 全部通过。
- API 端到端：PDF、DOCX、PNG 上传后均达到 `ready`，对应索引任务均为 `success`，无失败码。
- RAG 取证：分别查询 `Cortex PDF acceptance alpha 2026`、`DOCX acceptance beta 2026`、`Cortex OCR gamma 2026`，三次均返回 HTTP 200，同时收到 `sources` 与 `done` SSE 事件。

首次在索引任务刚成功时查询曾返回 `KNOWLEDGE_NO_EVIDENCE`；等待 Elasticsearch 异步投影完成后，同一链路三类格式全部通过。这符合当前索引写入与搜索投影最终一致的设计，客户端应以文档 ready 后的短暂投影窗口为可重试状态。

页码目前以生成 Markdown 的“第 N 页”分节进入分块内容，可供答案引用上下文识别；将页码、OCR 置信度写入独立数据库引用字段不属于本次兼容实现，后续可通过版本化迁移增强。
