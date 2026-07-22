from datetime import date
from fastapi import APIRouter, Depends, Query, Response, status
from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import Session

from ..core.database import get_db
from ..core.exceptions import AppError
from ..core.tenant import TenantContext, get_tenant_context
from ..models import Note, NoteRevision, note_tags
from ..schemas import NoteCreate, NoteOut, NotePage, NoteUpdate, RevisionOut
from ..services.note_service import (
    create_note,
    delete_note,
    get_note,
    restore_revision,
    update_note,
)

router = APIRouter(prefix="/api/v1/notes", tags=["notes"])


@router.get("", response_model=NotePage)
def list_notes(
    page: int = Query(1, ge=1),
    page_size: int = Query(20, ge=1, le=100),
    note_type: str | None = Query(None, alias="type"),
    start_date: date | None = None,
    end_date: date | None = None,
    tag_id: int | None = None,
    db: Session = Depends(get_db),
    context: TenantContext = Depends(get_tenant_context),
) -> NotePage:
    query = db.query(Note).filter(Note.tenant_id == context.tenant_id, Note.deleted_at.is_(None))
    if note_type:
        query = query.filter(Note.type == note_type)
    if start_date:
        query = query.filter(Note.note_date >= start_date)
    if end_date:
        query = query.filter(Note.note_date <= end_date)
    if tag_id:
        query = query.join(note_tags, note_tags.c.note_id == Note.id).filter(
            note_tags.c.tenant_id == context.tenant_id, note_tags.c.tag_id == tag_id
        )
    total = query.count()
    items = (
        query.order_by(Note.note_date.desc().nullslast(), Note.updated_at.desc())
        .offset((page - 1) * page_size)
        .limit(page_size)
        .all()
    )
    return NotePage(items=items, page=page, page_size=page_size, total=total)


@router.post("", response_model=NoteOut, status_code=status.HTTP_201_CREATED)
def post_note(
    payload: NoteCreate,
    db: Session = Depends(get_db),
    context: TenantContext = Depends(get_tenant_context),
) -> Note:
    try:
        return create_note(db, context, payload)
    except IntegrityError as exc:
        db.rollback()
        raise AppError("PERIOD_NOTE_EXISTS", "该周期笔记已存在", 409) from exc


@router.get("/{note_id}", response_model=NoteOut)
def read_note(
    note_id: int,
    db: Session = Depends(get_db),
    context: TenantContext = Depends(get_tenant_context),
) -> Note:
    return get_note(db, context, note_id)


@router.patch("/{note_id}", response_model=NoteOut)
def patch_note(
    note_id: int,
    payload: NoteUpdate,
    db: Session = Depends(get_db),
    context: TenantContext = Depends(get_tenant_context),
) -> Note:
    return update_note(db, context, note_id, payload)


@router.delete("/{note_id}", status_code=status.HTTP_204_NO_CONTENT, response_class=Response)
def remove_note(
    note_id: int,
    db: Session = Depends(get_db),
    context: TenantContext = Depends(get_tenant_context),
) -> Response:
    delete_note(db, context, note_id)
    return Response(status_code=204)


@router.get("/{note_id}/revisions", response_model=list[RevisionOut])
def revisions(
    note_id: int,
    db: Session = Depends(get_db),
    context: TenantContext = Depends(get_tenant_context),
) -> list[NoteRevision]:
    get_note(db, context, note_id)
    return (
        db.query(NoteRevision)
        .filter(NoteRevision.tenant_id == context.tenant_id, NoteRevision.note_id == note_id)
        .order_by(NoteRevision.created_at.desc())
        .all()
    )


@router.post("/{note_id}/revisions/{revision_id}/restore", response_model=NoteOut)
def restore(
    note_id: int,
    revision_id: int,
    db: Session = Depends(get_db),
    context: TenantContext = Depends(get_tenant_context),
) -> Note:
    return restore_revision(db, context, note_id, revision_id)
