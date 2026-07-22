# Database migrations

The runtime role (`diary_app`) and migration role (`diary_migrator`) are intentionally separate.
Set both URLs before running migrations:

```powershell
$env:DATABASE_URL = "postgresql+psycopg://diary_app:password@127.0.0.1:5432/diary_listener"
$env:MIGRATION_DATABASE_URL = "postgresql+psycopg://diary_migrator:password@127.0.0.1:5432/diary_listener"
alembic upgrade head
```

Rollback and re-apply the first baseline migration with:

```powershell
alembic downgrade base
alembic upgrade head
```

`/healthz` reports process liveness. `/readyz` also verifies the database connection.
