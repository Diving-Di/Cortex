# AI 聊天 & 轻日记

一个 AI 聊天 + 轻日记的 Web 应用 demo。左侧功能栏提供「聊天」「日记」两个功能：聊天支持与 AI 多轮对话；日记可以上传一张图片并配上一段文字，作为分享的轻日记，数据存储在数据库中。

## 技术栈

- 后端：Python、FastAPI、SQLAlchemy 2.0
- 数据库：本地默认 SQLite，Docker 环境使用 MySQL 8.0
- AI：DeepSeek / OpenAI 兼容的 Chat Completions 接口
- 前端：React、TypeScript、Webpack 5、Ant Design
- 容器化：Docker、Docker Compose

## 功能

- 用户注册、登录、退出（Token 鉴权）
- 聊天：多轮对话、历史会话列表、新建/删除会话
- 日记：上传图片 + 文字，时间线浏览、删除
- AI 接入 DeepSeek 兼容接口，密钥通过本地 `config.json` 配置且不入库

## 数据库设计

| 表 | 说明 |
| --- | --- |
| `users` | 注册账号（用户名、邮箱、密码哈希） |
| `auth_tokens` | 登录 Token，与用户一对多 |
| `conversations` | 聊天会话，归属用户 |
| `messages` | 会话内的每条消息（role = user / assistant） |
| `diary_entries` | 轻日记（图片 + 文字），归属用户 |

应用启动时会自动建表（`SQLAlchemy Base.metadata.create_all`），无需手动迁移。

## 目录结构

```text
.
├── backend/             # FastAPI + SQLAlchemy 后端
│   └── app/
│       ├── main.py      # FastAPI 入口
│       ├── config.py    # 运行时配置（含 AI 密钥）
│       ├── database.py  # SQLAlchemy 引擎/会话
│       ├── models.py    # 数据库模型
│       ├── security.py  # 密码哈希 / Token / 鉴权依赖
│       ├── ai.py        # DeepSeek 兼容 AI 客户端
│       └── routers/     # auth / chat / diary 路由
├── frontend/            # React + TypeScript + Webpack 前端
├── docs/api.md          # 接口明细
├── docker-compose.yml
└── README.md
```

## 配置 AI 密钥

复制示例配置并填入 DeepSeek 的 API Key（该文件已被 `.gitignore` 忽略，不会提交）：

```bash
cd backend
cp config.example.json config.json
# 编辑 config.json，填入 ai.api_key
```

未配置密钥时后端会返回本地模拟回复，应用依然可运行。

## 快速启动

### 使用 Docker Compose

```bash
docker-compose up --build
```

启动后访问：

- 前端：http://localhost:5173
- 后端：http://localhost:8000
- API 文档：http://localhost:8000/docs

### 本地开发

先启动后端：

```bash
cd backend
python -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
uvicorn app.main:app --host 0.0.0.0 --port 8000
```

再启动前端（Webpack DevServer，默认代理 `/api` 与 `/media` 到后端）：

```bash
cd frontend
npm install
npm run dev
```

前端默认地址：http://localhost:5173

## 前端构建

```bash
cd frontend
npm run build   # 先 tsc 类型检查，再 webpack 打包到 dist/
```
