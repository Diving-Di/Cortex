import io, re, zipfile
from fastapi import APIRouter, Depends, File, UploadFile
from fastapi.responses import Response
from sqlalchemy.orm import Session
from ..core.database import get_db
from ..core.exceptions import AppError
from ..core.tenant import TenantContext, get_tenant_context
from ..models import Note
from ..services.backup_service import create_backup, restore_backup

router = APIRouter(prefix="/api/v1", tags=["data"])


@router.post("/backups")
def backup(
    db: Session = Depends(get_db), context: TenantContext = Depends(get_tenant_context)
) -> Response:
    return Response(
        create_backup(db, context),
        media_type="application/zip",
        headers={"Content-Disposition": "attachment; filename=diary-listener-backup.zip"},
    )


@router.post("/backups/restore")
async def restore(
    file: UploadFile = File(...),
    db: Session = Depends(get_db),
    context: TenantContext = Depends(get_tenant_context),
) -> dict[str, int]:
    blob = await file.read(512 * 1024 * 1024 + 1)
    if len(blob) > 512 * 1024 * 1024:
        raise AppError("BACKUP_TOO_LARGE", "备份包过大", 413)
    return restore_backup(db, context, blob)


@router.post("/exports/markdown")
def export_markdown(
    db: Session = Depends(get_db), context: TenantContext = Depends(get_tenant_context)
) -> Response:
    notes = (
        db.query(Note)
        .filter(Note.tenant_id == context.tenant_id, Note.deleted_at.is_(None))
        .order_by(Note.note_date, Note.id)
        .all()
    )
    out = io.BytesIO()
    used = set()
    with zipfile.ZipFile(out, "w", zipfile.ZIP_DEFLATED) as z:
        for note in notes:
            base = (
                re.sub(r'[<>:"/\\|?*\x00-\x1f]', "_", note.title).strip(" .") or f"note-{note.id}"
            )
            name = f"{note.note_date or 'undated'}-{base}.md"
            candidate = name
            index = 2
            while candidate.lower() in used:
                candidate = name[:-3] + f"-{index}.md"
                index += 1
            used.add(candidate.lower())
            z.writestr(candidate, f"# {note.title}\n\n{note.content}".encode("utf-8"))
    return Response(
        out.getvalue(),
        media_type="application/zip",
        headers={"Content-Disposition": "attachment; filename=diary-listener-markdown.zip"},
    )
