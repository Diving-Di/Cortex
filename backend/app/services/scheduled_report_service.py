from __future__ import annotations

from datetime import date, datetime, timedelta, timezone
from zoneinfo import ZoneInfo, ZoneInfoNotFoundError

from sqlalchemy.orm import Session

from ..ai.client import OpenAICompatibleClient
from ..core.config import get_settings
from ..core.exceptions import AppError
from ..core.tenant import TenantContext, apply_tenant_context
from ..models import Note, NoteRevision, ReportSource, ScheduledReportRun, ScheduledReportTask
from .m2_service import report_sources, snippet
from .note_service import create_note
from ..schemas import NoteCreate


def next_run_at(
    report_type: str, hour: int, minute: int, timezone_name: str, now: datetime | None = None
) -> datetime:
    try:
        zone = ZoneInfo(timezone_name)
    except ZoneInfoNotFoundError as exc:
        raise AppError("INVALID_TIMEZONE", "无效的 IANA 时区", 422) from exc
    local_now = (now or datetime.now(timezone.utc)).astimezone(zone)
    candidate = local_now.replace(hour=hour, minute=minute, second=0, microsecond=0)
    if report_type == "daily":
        if candidate <= local_now:
            candidate += timedelta(days=1)
    elif report_type == "weekly":
        candidate += timedelta(days=(6 - candidate.weekday()) % 7)
        if candidate <= local_now:
            candidate += timedelta(days=7)
    elif report_type == "monthly":
        candidate = candidate.replace(day=1)
        next_month = (candidate.replace(day=28) + timedelta(days=4)).replace(day=1)
        candidate = next_month - timedelta(days=1)
        if candidate <= local_now:
            following = (next_month.replace(day=28) + timedelta(days=4)).replace(day=1)
            candidate = following - timedelta(days=1)
    else:
        raise AppError("INVALID_REPORT_TYPE", "报告类型无效", 422)
    return candidate.astimezone(timezone.utc)


async def execute_scheduled_report(
    db: Session, context: TenantContext, task: ScheduledReportTask, trigger: str
) -> ScheduledReportRun:
    apply_tenant_context(db, context)
    started = datetime.now(timezone.utc)
    run = ScheduledReportRun(
        tenant_id=context.tenant_id,
        task_id=task.id,
        status="running",
        trigger=trigger,
        started_at=started,
    )
    db.add(run)
    db.flush()
    try:
        anchor = datetime.now(ZoneInfo(task.timezone)).date()
        start, _, sources = report_sources(db, context, task.report_type, anchor)
        if not sources:
            raise AppError("REPORT_NO_SOURCES", "所选周期没有来源笔记", 422)
        config = get_settings().ai
        if not config.get("api_key"):
            raise AppError("AI_NOT_CONFIGURED", "AI 未配置", 503)
        material = "\n\n".join(
            f"[来源 #{n.id} {n.note_date} {n.title}]\n{snippet(n.content, 2000)}" for n in sources
        )
        prompt = (
            f"仅依据以下来源撰写 {task.report_type} Markdown 报告，使用 [#笔记ID] 引用，不得虚构。\n"
            + material
        )
        client = OpenAICompatibleClient(config["base_url"], config["api_key"])
        chunks: list[str] = []
        async for chunk in client.stream_chat(
            config["model"], [{"role": "user", "content": prompt}]
        ):
            chunks.append(chunk)
        content = "".join(chunks)
        note = (
            db.query(Note)
            .filter(
                Note.tenant_id == context.tenant_id,
                Note.type == task.report_type,
                Note.note_date == start,
                Note.deleted_at.is_(None),
            )
            .one_or_none()
        )
        if note:
            db.add(
                NoteRevision(
                    tenant_id=context.tenant_id,
                    note_id=note.id,
                    created_by=context.user_id,
                    content=note.content,
                    reason="scheduled_report_overwrite",
                )
            )
            note.content = content
            note.word_count = len("".join(content.split()))
            note.updated_at = datetime.now(timezone.utc)
            db.query(ReportSource).filter(
                ReportSource.tenant_id == context.tenant_id,
                ReportSource.report_note_id == note.id,
            ).delete()
        else:
            note = create_note(
                db,
                context,
                NoteCreate(
                    type=task.report_type,
                    title=f"{start} {task.report_type} 报告",
                    content=content,
                    note_date=start,
                ),
            )
        db.flush()
        for rank, source in enumerate(sources, 1):
            db.add(
                ReportSource(
                    tenant_id=context.tenant_id,
                    report_note_id=note.id,
                    source_note_id=source.id,
                    rank=rank,
                )
            )
        run.status = "success"
        run.report_note_id = note.id
    except Exception as exc:
        run.status = "failed"
        run.error_code = exc.code if isinstance(exc, AppError) else type(exc).__name__
        run.error_message = str(exc)[:1000]
    run.finished_at = datetime.now(timezone.utc)
    task.last_run_at = run.finished_at
    task.next_run_at = next_run_at(
        task.report_type, task.hour, task.minute, task.timezone, run.finished_at
    )
    db.commit()
    return run
