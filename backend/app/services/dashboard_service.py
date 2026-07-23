from __future__ import annotations

from datetime import date, datetime, time, timedelta, timezone
from zoneinfo import ZoneInfo, ZoneInfoNotFoundError

from sqlalchemy import func
from sqlalchemy.orm import Session

from ..core.exceptions import AppError
from ..core.tenant import TenantContext
from ..models import AiUsageRecord, Note
from .m2_service import report_range


def local_day_bounds(day: date, timezone_name: str) -> tuple[datetime, datetime]:
    try:
        zone = ZoneInfo(timezone_name)
    except (ZoneInfoNotFoundError, ValueError) as exc:
        raise AppError("INVALID_TIMEZONE", "无效的 IANA 时区", 422) from exc
    start = datetime.combine(day, time.min, zone).astimezone(timezone.utc)
    end = (datetime.combine(day, time.min, zone) + timedelta(days=1)).astimezone(timezone.utc)
    return start, end


def calculate_streak(active_days: set[date], today: date) -> int:
    # A user who has not written yet today keeps the streak earned through yesterday.
    cursor = today if today in active_days else today - timedelta(days=1)
    streak = 0
    while cursor in active_days:
        streak += 1
        cursor -= timedelta(days=1)
    return streak


def dashboard_summary(
    db: Session,
    context: TenantContext,
    timezone_name: str,
    today: date | None = None,
) -> dict:
    try:
        zone = ZoneInfo(timezone_name)
    except (ZoneInfoNotFoundError, ValueError) as exc:
        raise AppError("INVALID_TIMEZONE", "无效的 IANA 时区", 422) from exc
    today = today or datetime.now(zone).date()
    start_utc, end_utc = local_day_bounds(today, timezone_name)
    base = (Note.tenant_id == context.tenant_id, Note.deleted_at.is_(None))

    recent = (
        db.query(Note).filter(*base).order_by(Note.updated_at.desc(), Note.id.desc()).limit(6).all()
    )
    today_count = (
        db.query(func.count(Note.id))
        .filter(*base, Note.created_at >= start_utc, Note.created_at < end_utc)
        .scalar()
        or 0
    )
    total_notes, total_words = (
        db.query(func.count(Note.id), func.coalesce(func.sum(Note.word_count), 0))
        .filter(*base)
        .one()
    )
    ai_requests, ai_tokens = (
        db.query(
            func.count(AiUsageRecord.id),
            func.coalesce(func.sum(AiUsageRecord.input_tokens + AiUsageRecord.output_tokens), 0),
        )
        .filter(AiUsageRecord.tenant_id == context.tenant_id)
        .one()
    )

    since = today - timedelta(days=364)
    local_created_day = func.date(func.timezone(timezone_name, Note.created_at))
    rows = (
        db.query(local_created_day.label("day"), func.count(Note.id))
        .filter(*base, Note.created_at >= local_day_bounds(since, timezone_name)[0])
        .group_by(local_created_day)
        .order_by(local_created_day)
        .all()
    )
    activity = {row.day: row[1] for row in rows}

    pending = []
    labels = {"daily": "日报", "weekly": "周报", "monthly": "月报"}
    for kind in ("daily", "weekly", "monthly"):
        period_start, period_end = report_range(kind, today)
        report_exists = (
            db.query(Note.id)
            .filter(*base, Note.type == kind, Note.note_date == period_start)
            .first()
            is not None
        )
        source_exists = (
            db.query(Note.id)
            .filter(
                *base,
                Note.note_date >= period_start,
                Note.note_date <= min(period_end, today),
                Note.type != kind,
            )
            .first()
            is not None
        )
        if source_exists and not report_exists:
            pending.append(
                {
                    "type": kind,
                    "label": labels[kind],
                    "anchor_date": today.isoformat(),
                    "period_start": period_start.isoformat(),
                }
            )

    return {
        "date": today.isoformat(),
        "timezone": timezone_name,
        "today": {"new_notes": today_count},
        "streak_days": calculate_streak(set(activity), today),
        "statistics": {
            "notes": total_notes,
            "words": total_words,
            "ai_requests": ai_requests,
            "ai_tokens": ai_tokens,
        },
        "recent_notes": [
            {
                "id": note.id,
                "title": note.title,
                "type": note.type,
                "note_date": note.note_date,
                "updated_at": note.updated_at,
                "summary": note.summary,
            }
            for note in recent
        ],
        "activity": [{"date": day.isoformat(), "count": count} for day, count in activity.items()],
        "pending_reports": pending,
    }
