# 个人工作流与轻量部署验收

日期：2026-10-05。对象为本轮未提交工作树和本机重新构建的 `cortex-backend:local` / `cortex-frontend:local`，不是线上发布证明。

## 变更范围

- 笔记默认列表、类型/关键词/标签/日期筛选，账号隔离的标签页草稿、冲突比较、正文历史预览与恢复。
- 报告实际来源绑定、取消及失败保留、最近五次成功草稿比较和恢复、已保存报告入口。
- 知识问答来源片段、历史来源查询、完成回答确认保存、索引失败重试。
- 工作台待处理与上周回顾、可跳过引导、活动入口调整、用量估算标识。
- 独立轻量 Compose、评测产物比较工具及文档整合。没有修改数据库结构；仍为迁移 42 和 57 张 public 表。

具体契约见 [API](../api.md)、[页面说明](../page/README.md)和[个人部署](../guides/PERSONAL_DEPLOYMENT.md)。

## 环境与命令

Windows / PowerShell；本机前端使用 Node 24.11.0，默认 PATH 中的 Node 18 不满足项目要求。
Docker Engine 28.4；集成依赖使用 `docker-compose.ci.yml`，功能浏览器使用轻量栈的 Nginx 生产产物。
Docker Hub 元数据拉取出现 EOF，本次构建通过已有镜像参数选择 Public ECR 的 Node 20 / Nginx 1.27 镜像；没有改变仓库默认镜像。

```powershell
.\backend\scripts\local_integration_test.ps1
.\backend\scripts\non_ai_smoke.ps1
docker compose config --quiet
.\backend\scripts\check_light_compose.ps1
docker compose -f docker-compose.light.yml up -d --build db backend frontend
python scripts/check_docs.py
python -m unittest discover -s scripts -p test_compare_rag_runs.py

Set-Location backend
go run ./cmd/rag-regression-check
# local_integration_test.ps1 已执行 go vet ./...、go test -count=1 -v ./... 和 go build ./cmd/server。

Set-Location ../frontend
$env:PATH = 'C:\Program Files\nodejs;' + $env:PATH
npm run format:check
npm test
npm run build
$env:E2E_REAL_BACKEND = '1'
$env:E2E_BASE_URL = 'http://127.0.0.1:5173'
$env:PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH = 'C:\Program Files\Google\Chrome\Application\chrome.exe'
npm run test:e2e -- --workers=1
```

## 结果

| 项目 | 结果与证明范围 |
| --- | --- |
| Go vet / 全量测试 / server build | 通过；集成依赖实际运行，新测试覆盖回答所属账号与租户、失效来源、配额、四路并发幂等、未完成回答及恢复冲突/历史保留 |
| schema / RLS | `TestProductionSchemaAndRLSContract` 通过；新轻量数据卷初始化后 ready、注册和登录通过 |
| 前端格式 / TypeScript / Webpack | 通过；Webpack 5.107.2 生产构建成功 |
| Vitest | 18 个测试文件、48 项测试通过；含报告旧版本恢复后来源 ID 保持一致 |
| Playwright | 单 worker，3 项通过；浏览器登录契约，以及真实栈注册/登录/退出、筛选、刷新恢复草稿、离开保护、保存和历史恢复 |
| 非 AI HTTP smoke | 通过；笔记、标签、搜索、附件、ZIP 导出，跨租户附件 404、软删除租户 401 |
| Compose | 完整配置解析和轻量 profile/内部端口/卷隔离检查通过；轻量 db、backend、frontend 全部 healthy |
| 评测工具 | 3 项比较工具测试通过；公开 RAG 冻结夹具结构与证据 hash 校验通过 |
| 文档 | 根 README 与整理后的 docs 相对文件链接检查通过；删除 9 份重复/被替代文档，保留日期化验收证据 |

初次浏览器并行运行出现导航超时，最终改为单 worker，使用最新生产镜像验证通过。
模板重试单元测试修正为同时等待解除 disabled 和加载动画结束，避免在 Ant Design 尚拦截点击时触发下一次请求。
这些修正没有放宽功能断言或引入自动业务重试。

## 本轮未验证及限制

- 未运行真实 LiteLLM/供应商的 `ai_acceptance.ps1`；流式取消、失败保留和来源绑定的前端测试使用模拟模型事件。不能据此宣称真实回答质量、成本或网关故障切换通过。
- 轻量 `ai` / `knowledge` 可选 profile 仅验证 Compose 配置，未启动 CPU Embedding、Reranker 和解析器做性能验收。
- 未做目标生产环境容量、联合备份恢复、TLS 或多节点故障演练；以前日期的实测结论保持其原适用范围。
- 草稿只在当前标签页 sessionStorage 中保存，最长七天；不跨设备，不等于备份。五份生成历史是浏览器草稿历史；服务器 revision 只恢复正文，不恢复旧报告来源快照。
- 本机验收栈保留独立数据卷及测试账号；没有替换完整部署的数据卷，也没有发布到外部服务。
