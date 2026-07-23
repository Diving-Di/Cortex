from __future__ import annotations

import asyncio
import logging
import os
from datetime import datetime, timezone
from uuid import UUID

from sqlalchemy import create_engine, text
from sqlalchemy.orm import Session

from ..core.config import get_settings
from ..core.tenant import TenantContext
from ..database import SessionLocal
from ..models import ScheduledReportTask
from .scheduled_report_service import execute_scheduled_report, next_run_at

logger = logging.getLogger(__name__)


def claim_due_tasks(limit: int = 10) -> list[tuple[int, UUID, int]]:
    """Claim due work with the migration/scheduler role so RLS stays intact for app traffic."""
    engine = create_engine(get_settings().migration_database_url, pool_pre_ping=True)
    try:
        with engine.begin() as connection:
            rows = (
                connection.execute(
                    text("""
                SELECT id, tenant_id, created_by, report_type, hour, minute, timezone
                FROM scheduled_report_tasks
                WHERE status = 'enabled' AND next_run_at <= now()
                ORDER BY next_run_at
                FOR UPDATE SKIP LOCKED
                LIMIT :limit
            """),
                    {"limit": limit},
                )
                .mappings()
                .all()
            )
            claimed: list[tuple[int, UUID, int]] = []
            for row in rows:
                following = next_run_at(
                    row["report_type"], row["hour"], row["minute"], row["timezone"]
                )
                connection.execute(
                    text(
                        "UPDATE scheduled_report_tasks SET next_run_at=:next, updated_at=now() WHERE id=:id"
                    ),
                    {"next": following, "id": row["id"]},
                )
                claimed.append((row["id"], row["tenant_id"], row["created_by"]))
            return claimed
    finally:
        engine.dispose()


async def scheduler_loop() -> None:
    interval = max(10, int(os.getenv("SCHEDULED_REPORT_POLL_SECONDS", "60")))
    while True:
        try:
            for task_id, tenant_id, user_id in await asyncio.to_thread(claim_due_tasks):
                with SessionLocal() as db:
                    context = TenantContext(user_id=user_id, tenant_id=tenant_id)
                    # Apply RLS before loading the claimed tenant-owned task.
                    from ..core.tenant import apply_tenant_context

                    apply_tenant_context(db, context)
                    task = (
                        db.query(ScheduledReportTask)
                        .filter(
                            ScheduledReportTask.id == task_id,
                            ScheduledReportTask.tenant_id == tenant_id,
                        )
                        .one_or_none()
                    )
                    if task:
                        await execute_scheduled_report(db, context, task, "scheduled")
        except asyncio.CancelledError:
            raise
        except Exception:
            logger.exception("Scheduled report poll failed")
        await asyncio.sleep(interval)
