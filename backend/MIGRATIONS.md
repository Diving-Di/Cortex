# 数据库迁移

Diary Listener 统一使用 PostgreSQL 16，应用运行角色（`diary_app`）与迁移角色（`diary_migrator`）有意分离。数据库结构只允许通过 Alembic 修改，应用启动不会调用 `create_all()` 隐式建表。

执行迁移前设置两个连接地址：

```powershell
$env:DATABASE_URL = "postgresql+psycopg://diary_app:password@127.0.0.1:5432/diary_listener"
$env:MIGRATION_DATABASE_URL = "postgresql+psycopg://diary_migrator:password@127.0.0.1:5432/diary_listener"
alembic upgrade head
```

初始化或升级数据库：

```powershell
alembic upgrade head
alembic current
```

仅在开发或全新测试数据库中验证完整回退与重放：

```powershell
alembic downgrade base
alembic upgrade head
```

`alembic downgrade base` 会删除由迁移管理的结构，不应对需要保留数据的数据库执行。

Docker Compose 会在数据库就绪后自动执行 `alembic upgrade head`。`/healthz` 只报告进程存活，`/readyz` 还会验证数据库连接。
