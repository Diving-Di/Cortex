from fastapi import APIRouter, Depends
from sqlalchemy.orm import Session
from ..core.database import get_db
from ..core.exceptions import AppError
from ..core.tenant import TenantContext, get_tenant_context
from ..models import Tag, note_tags
from ..schemas import TagAssignment, TagCreate, TagOut
from ..services.note_service import get_note

router = APIRouter(prefix="/api/v1", tags=["tags"])


@router.get("/tags", response_model=list[TagOut])
def list_tags(
    db: Session = Depends(get_db), context: TenantContext = Depends(get_tenant_context)
) -> list[Tag]:
    return db.query(Tag).filter(Tag.tenant_id == context.tenant_id).order_by(Tag.name).all()


@router.post("/tags", response_model=TagOut, status_code=201)
def create_tag(
    payload: TagCreate,
    db: Session = Depends(get_db),
    context: TenantContext = Depends(get_tenant_context),
) -> Tag:
    name = payload.name.strip()
    if not name:
        raise AppError("TAG_NAME_REQUIRED", "标签名称不能为空", 422)
    existing = (
        db.query(Tag).filter(Tag.tenant_id == context.tenant_id, Tag.name == name).one_or_none()
    )
    if existing:
        return existing
    item = Tag(tenant_id=context.tenant_id, name=name, color=payload.color)
    db.add(item)
    db.flush()
    return item


@router.put("/notes/{note_id}/tags", response_model=list[TagOut])
def assign_tags(
    note_id: int,
    payload: TagAssignment,
    db: Session = Depends(get_db),
    context: TenantContext = Depends(get_tenant_context),
) -> list[Tag]:
    get_note(db, context, note_id)
    tags = (
        db.query(Tag).filter(Tag.tenant_id == context.tenant_id, Tag.id.in_(payload.tag_ids)).all()
        if payload.tag_ids
        else []
    )
    if len(tags) != len(set(payload.tag_ids)):
        raise AppError("TAG_NOT_FOUND", "标签不存在", 404)
    db.execute(
        note_tags.delete().where(
            note_tags.c.tenant_id == context.tenant_id, note_tags.c.note_id == note_id
        )
    )
    for tag in tags:
        db.execute(
            note_tags.insert().values(tenant_id=context.tenant_id, note_id=note_id, tag_id=tag.id)
        )
    return tags


@router.get("/notes/{note_id}/tags", response_model=list[TagOut])
def note_tag_list(
    note_id: int,
    db: Session = Depends(get_db),
    context: TenantContext = Depends(get_tenant_context),
) -> list[Tag]:
    get_note(db, context, note_id)
    return (
        db.query(Tag)
        .join(note_tags, note_tags.c.tag_id == Tag.id)
        .filter(note_tags.c.tenant_id == context.tenant_id, note_tags.c.note_id == note_id)
        .all()
    )
