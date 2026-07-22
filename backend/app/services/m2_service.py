from __future__ import annotations
import re
from datetime import date, timedelta
from sqlalchemy import case, or_
from sqlalchemy.orm import Session
from ..core.exceptions import AppError
from ..core.tenant import TenantContext
from ..models import Note


def report_range(kind: str, anchor: date) -> tuple[date, date]:
    if kind == "daily":
        return anchor, anchor
    if kind == "weekly":
        start = anchor - timedelta(days=anchor.weekday())
        return start, start + timedelta(days=6)
    if kind == "monthly":
        start = anchor.replace(day=1)
        next_month = (start.replace(day=28) + timedelta(days=4)).replace(day=1)
        return start, next_month - timedelta(days=1)
    raise AppError("INVALID_REPORT_TYPE", "报告类型必须是 daily、weekly 或 monthly", 422)


def report_sources(
    db: Session, context: TenantContext, kind: str, anchor: date
) -> tuple[date, date, list[Note]]:
    start, end = report_range(kind, anchor)
    notes = (
        db.query(Note)
        .filter(
            Note.tenant_id == context.tenant_id,
            Note.deleted_at.is_(None),
            Note.note_date >= start,
            Note.note_date <= end,
            Note.type != kind,
        )
        .order_by(Note.note_date, Note.id)
        .limit(100)
        .all()
    )
    return start, end, notes


def parse_memory_query(
    question: str, today: date | None = None
) -> tuple[date | None, date | None, list[str]]:
    today = today or date.today()
    start = end = None
    if "今天" in question:
        start = end = today
    elif "昨天" in question:
        start = end = today - timedelta(days=1)
    elif "本周" in question:
        start = today - timedelta(days=today.weekday())
        end = start + timedelta(days=6)
    elif "上周" in question:
        end = today - timedelta(days=today.weekday() + 1)
        start = end - timedelta(days=6)
    match = re.search(r"(20\d{2})[-年/](\d{1,2})[-月/](\d{1,2})", question)
    if match:
        start = end = date(*map(int, match.groups()))
    searchable = re.sub(r"20\d{2}[-年/]\d{1,2}[-月/]\d{1,2}日?", " ", question)
    for phrase in (
        "今天",
        "昨天",
        "本周",
        "上周",
        "做了什么",
        "有什么",
        "有哪些",
        "记录",
        "请问",
        "告诉我",
        "回忆一下",
    ):
        searchable = searchable.replace(phrase, " ")
    searchable = re.sub(r"[我的了在是有和与吗呢？?，,。]", " ", searchable)
    words = re.findall(r"[\u4e00-\u9fff]{2,}|[A-Za-z0-9_]{2,}", searchable)
    return start, end, words[:8]


def memory_candidates(
    db: Session, context: TenantContext, question: str, limit: int = 8
) -> list[Note]:
    start, end, words = parse_memory_query(question)
    query = db.query(Note).filter(Note.tenant_id == context.tenant_id, Note.deleted_at.is_(None))
    if start:
        query = query.filter(Note.note_date >= start)
    if end:
        query = query.filter(Note.note_date <= end)
    if words:
        clauses = [
            or_(
                Note.title.ilike(f"%{word}%"),
                Note.content.ilike(f"%{word}%"),
                Note.summary.ilike(f"%{word}%"),
            )
            for word in words
        ]
        score = sum((case((clause, 1), else_=0) for clause in clauses))
        query = query.filter(or_(*clauses)).order_by(
            score.desc(), Note.note_date.desc().nullslast()
        )
    else:
        query = query.order_by(Note.note_date.desc().nullslast())
    return query.limit(limit).all()


def snippet(content: str, limit: int = 500) -> str:
    clean = " ".join(content.split())
    return clean[:limit]
