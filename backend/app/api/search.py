from datetime import date
from fastapi import APIRouter, Depends, Query
from sqlalchemy import or_
from sqlalchemy.orm import Session
from ..core.database import get_db
from ..core.tenant import TenantContext, get_tenant_context
from ..models import Note, note_tags
from ..schemas import SearchPage, SearchResult

router = APIRouter(prefix="/api/v1/search", tags=["search"])


@router.get("", response_model=SearchPage)
def search(
    q: str = "",
    note_type: str | None = Query(None, alias="type"),
    start_date: date | None = None,
    end_date: date | None = None,
    tag_id: int | None = None,
    limit: int = Query(20, ge=1, le=100),
    db: Session = Depends(get_db),
    context: TenantContext = Depends(get_tenant_context),
) -> SearchPage:
    query = db.query(Note).filter(Note.tenant_id == context.tenant_id, Note.deleted_at.is_(None))
    if q:
        query = query.filter(
            or_(
                Note.title.ilike(f"%{q}%"),
                Note.content.ilike(f"%{q}%"),
                Note.summary.ilike(f"%{q}%"),
            )
        )
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
    notes = query.order_by(Note.updated_at.desc()).limit(limit).all()
    items = []
    for note in notes:
        source = note.content or note.summary or ""
        pos = source.lower().find(q.lower()) if q else 0
        start = max(0, pos - 40)
        snippet = source[start : start + 160]
        items.append(
            SearchResult(
                id=note.id,
                title=note.title,
                snippet=snippet,
                type=note.type,
                note_date=note.note_date,
                updated_at=note.updated_at,
            )
        )
    return SearchPage(items=items, total=total)
