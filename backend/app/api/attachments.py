from pathlib import Path
from fastapi import APIRouter, Depends, File, UploadFile
from fastapi.responses import FileResponse, Response
from sqlalchemy.orm import Session
from ..core.database import get_db
from ..core.exceptions import AppError
from ..core.tenant import TenantContext, get_tenant_context
from ..models import Attachment
from ..schemas import AttachmentOut
from ..services.attachment_service import safe_attachment_path, save_upload
from ..services.note_service import get_note

router = APIRouter(prefix="/api/v1/attachments", tags=["attachments"])

@router.post("", response_model=AttachmentOut, status_code=201)
async def upload_attachment(note_id: int, file: UploadFile = File(...), db: Session = Depends(get_db), context: TenantContext = Depends(get_tenant_context)) -> Attachment:
    get_note(db, context, note_id); return await save_upload(db, context, note_id, file)

def _get(db: Session, context: TenantContext, attachment_id: int) -> Attachment:
    item = db.query(Attachment).filter(Attachment.tenant_id == context.tenant_id, Attachment.id == attachment_id).one_or_none()
    if item is None: raise AppError("ATTACHMENT_NOT_FOUND", "附件不存在", 404)
    return item

@router.get("/note/{note_id}", response_model=list[AttachmentOut])
def list_attachments(note_id: int, db: Session = Depends(get_db), context: TenantContext = Depends(get_tenant_context)) -> list[Attachment]:
    get_note(db, context, note_id); return db.query(Attachment).filter(Attachment.tenant_id == context.tenant_id, Attachment.note_id == note_id).all()

@router.get("/{attachment_id}")
def download_attachment(attachment_id: int, db: Session = Depends(get_db), context: TenantContext = Depends(get_tenant_context)) -> FileResponse:
    item = _get(db, context, attachment_id); path = safe_attachment_path(item.stored_path)
    if not path.is_file(): raise AppError("ATTACHMENT_FILE_MISSING", "附件文件缺失", 410)
    return FileResponse(path, media_type=item.mime_type, filename=item.original_name)

@router.delete("/{attachment_id}", status_code=204, response_class=Response)
def delete_attachment(attachment_id: int, db: Session = Depends(get_db), context: TenantContext = Depends(get_tenant_context)) -> Response:
    item = _get(db, context, attachment_id); path = safe_attachment_path(item.stored_path); tombstone = path.with_suffix(path.suffix + ".deleting")
    if path.exists(): path.replace(tombstone)
    try:
        db.delete(item); db.commit()
    except Exception:
        if tombstone.exists(): tombstone.replace(path)
        raise
    tombstone.unlink(missing_ok=True)
    return Response(status_code=204)
