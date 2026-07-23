from datetime import datetime, timezone
from fastapi import APIRouter, BackgroundTasks, Depends
from pydantic import BaseModel, ConfigDict, Field
from sqlalchemy.orm import Session

from ..core.database import get_db
from ..core.exceptions import AppError
from ..core.tenant import TenantContext, get_tenant_context
from ..database import SessionLocal
from ..models import ScheduledReportRun, ScheduledReportTask
from ..services.audit_service import record_audit
from ..services.scheduled_report_service import execute_scheduled_report, next_run_at

router = APIRouter(prefix="/api/v1/scheduled-reports", tags=["scheduled-reports"])


class TaskIn(BaseModel):
    model_config = ConfigDict(extra="forbid")
    report_type: str
    hour: int = Field(20, ge=0, le=23)
    minute: int = Field(0, ge=0, le=59)
    timezone: str = "Asia/Shanghai"


def task_dict(task: ScheduledReportTask) -> dict:
    return {
        key: getattr(task, key)
        for key in (
            "id",
            "report_type",
            "hour",
            "minute",
            "timezone",
            "status",
            "next_run_at",
            "last_run_at",
            "created_at",
        )
    }


@router.get("")
def list_tasks(db: Session = Depends(get_db), context: TenantContext = Depends(get_tenant_context)):
    return [
        task_dict(t)
        for t in db.query(ScheduledReportTask)
        .filter(ScheduledReportTask.tenant_id == context.tenant_id)
        .order_by(ScheduledReportTask.id)
        .all()
    ]


@router.post("", status_code=201)
def create_task(
    payload: TaskIn,
    db: Session = Depends(get_db),
    context: TenantContext = Depends(get_tenant_context),
):
    task = ScheduledReportTask(
        tenant_id=context.tenant_id,
        created_by=context.user_id,
        report_type=payload.report_type,
        hour=payload.hour,
        minute=payload.minute,
        timezone=payload.timezone,
        next_run_at=next_run_at(
            payload.report_type, payload.hour, payload.minute, payload.timezone
        ),
    )
    db.add(task)
    db.flush()
    record_audit(db, context, "scheduled_report.create", "scheduled_report_task", task.id)
    return task_dict(task)


def get_task(db: Session, context: TenantContext, task_id: int) -> ScheduledReportTask:
    task = (
        db.query(ScheduledReportTask)
        .filter(
            ScheduledReportTask.tenant_id == context.tenant_id, ScheduledReportTask.id == task_id
        )
        .one_or_none()
    )
    if not task:
        raise AppError("SCHEDULED_REPORT_NOT_FOUND", "定时报告任务不存在", 404)
    return task


@router.patch("/{task_id}")
def set_status(
    task_id: int,
    enabled: bool,
    db: Session = Depends(get_db),
    context: TenantContext = Depends(get_tenant_context),
):
    task = get_task(db, context, task_id)
    task.status = "enabled" if enabled else "disabled"
    task.updated_at = datetime.now(timezone.utc)
    if enabled:
        task.next_run_at = next_run_at(task.report_type, task.hour, task.minute, task.timezone)
    record_audit(db, context, "scheduled_report.status", "scheduled_report_task", task.id)
    return task_dict(task)


async def _run(task_id: int, context: TenantContext, trigger: str) -> None:
    with SessionLocal() as db:
        task = get_task(db, context, task_id)
        await execute_scheduled_report(db, context, task, trigger)


@router.post("/{task_id}/retry", status_code=202)
def retry(
    task_id: int,
    jobs: BackgroundTasks,
    db: Session = Depends(get_db),
    context: TenantContext = Depends(get_tenant_context),
):
    get_task(db, context, task_id)
    jobs.add_task(_run, task_id, context, "manual")
    return {"status": "queued"}


@router.get("/{task_id}/runs")
def runs(
    task_id: int,
    db: Session = Depends(get_db),
    context: TenantContext = Depends(get_tenant_context),
):
    get_task(db, context, task_id)
    rows = (
        db.query(ScheduledReportRun)
        .filter(
            ScheduledReportRun.tenant_id == context.tenant_id,
            ScheduledReportRun.task_id == task_id,
        )
        .order_by(ScheduledReportRun.started_at.desc())
        .limit(50)
        .all()
    )
    return [
        {
            key: getattr(row, key)
            for key in (
                "id",
                "status",
                "trigger",
                "report_note_id",
                "error_code",
                "error_message",
                "started_at",
                "finished_at",
            )
        }
        for row in rows
    ]
