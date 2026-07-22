from datetime import date, datetime, timedelta, timezone
from sqlalchemy import func
from sqlalchemy.orm import Session

from ..core.exceptions import AppError
from ..core.tenant import TenantContext
from ..models import Note, NoteRevision, Tenant
from ..schemas import NoteCreate, NoteUpdate
from .audit_service import record_audit


def _word_count(content: str) -> int:
    return len("".join(content.split()))


def _period_start(note_type: str, value: date | None) -> date | None:
    if value is None or note_type in {"normal", "daily"}:
        return value
    if note_type == "weekly":
        return value - timedelta(days=value.weekday())
    return value.replace(day=1)


def get_note(db: Session, context: TenantContext, note_id: int) -> Note:
    note = (
        db.query(Note)
        .filter(Note.tenant_id == context.tenant_id, Note.id == note_id, Note.deleted_at.is_(None))
        .one_or_none()
    )
    if note is None:
        raise AppError("NOTE_NOT_FOUND", "笔记不存在", 404)
    return note


def create_note(db: Session, context: TenantContext, payload: NoteCreate) -> Note:
    if payload.type != "normal" and payload.note_date is None:
        raise AppError("NOTE_DATE_REQUIRED", "周期笔记必须指定归属日期", 422)
    tenant = db.query(Tenant).filter(Tenant.id == context.tenant_id).one()
    count = (
        db.query(func.count(Note.id))
        .filter(Note.tenant_id == context.tenant_id, Note.deleted_at.is_(None))
        .scalar()
        or 0
    )
    if count >= tenant.note_quota:
        raise AppError("NOTE_QUOTA_EXCEEDED", "笔记数量已达到配额", 409)
    note = Note(
        tenant_id=context.tenant_id,
        created_by=context.user_id,
        updated_by=context.user_id,
        type=payload.type,
        title=payload.title.strip(),
        content=payload.content,
        note_date=_period_start(payload.type, payload.note_date),
        summary=payload.summary,
        word_count=_word_count(payload.content),
    )
    if not note.title:
        raise AppError("TITLE_REQUIRED", "标题不能为空", 422)
    db.add(note)
    db.flush()
    record_audit(db, context, "note.create", "note", note.id)
    return note


def update_note(db: Session, context: TenantContext, note_id: int, payload: NoteUpdate) -> Note:
    note = get_note(db, context, note_id)
    if payload.expected_updated_at and note.updated_at != payload.expected_updated_at:
        raise AppError("NOTE_CONFLICT", "笔记已被其他请求更新", 409)
    values = payload.model_dump(exclude_unset=True, exclude={"expected_updated_at"})
    if "title" in values:
        values["title"] = values["title"].strip()
        if not values["title"]:
            raise AppError("TITLE_REQUIRED", "标题不能为空", 422)
    if "content" in values and values["content"] != note.content:
        db.add(
            NoteRevision(
                tenant_id=context.tenant_id,
                note_id=note.id,
                created_by=context.user_id,
                content=note.content,
                reason="update",
            )
        )
        note.word_count = _word_count(values["content"])
    for key, value in values.items():
        setattr(note, key, value)
    note.updated_by = context.user_id
    note.updated_at = datetime.now(timezone.utc)
    record_audit(db, context, "note.update", "note", note.id)
    return note


def delete_note(db: Session, context: TenantContext, note_id: int) -> None:
    note = get_note(db, context, note_id)
    note.deleted_at = datetime.now(timezone.utc)
    record_audit(db, context, "note.delete", "note", note.id)


def restore_revision(db: Session, context: TenantContext, note_id: int, revision_id: int) -> Note:
    note = get_note(db, context, note_id)
    revision = (
        db.query(NoteRevision)
        .filter(
            NoteRevision.tenant_id == context.tenant_id,
            NoteRevision.note_id == note.id,
            NoteRevision.id == revision_id,
        )
        .one_or_none()
    )
    if revision is None:
        raise AppError("REVISION_NOT_FOUND", "历史版本不存在", 404)
    db.add(
        NoteRevision(
            tenant_id=context.tenant_id,
            note_id=note.id,
            created_by=context.user_id,
            content=note.content,
            reason="before_restore",
        )
    )
    note.content = revision.content
    note.word_count = _word_count(note.content)
    note.updated_by = context.user_id
    note.updated_at = datetime.now(timezone.utc)
    record_audit(db, context, "note.revision.restore", "note", note.id)
    return note
