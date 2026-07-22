from __future__ import annotations
import hashlib
import mimetypes
from datetime import datetime
from pathlib import Path
from uuid import uuid4
from fastapi import UploadFile
from sqlalchemy import func
from sqlalchemy.orm import Session
from ..core.config import get_settings
from ..core.exceptions import AppError
from ..core.tenant import TenantContext
from ..models import Attachment, Tenant

SIGNATURES = {
    ".png": ("image/png", b"\x89PNG\r\n\x1a\n"),
    ".jpg": ("image/jpeg", b"\xff\xd8\xff"),
    ".jpeg": ("image/jpeg", b"\xff\xd8\xff"),
    ".pdf": ("application/pdf", b"%PDF-"),
}
TEXT_EXTENSIONS = {".txt": "text/plain", ".md": "text/markdown"}


def attachment_root() -> Path:
    return get_settings().data_dir / "attachments"


def safe_attachment_path(relative: str) -> Path:
    root = attachment_root().resolve()
    target = (get_settings().data_dir / relative).resolve()
    if root not in target.parents:
        raise AppError("INVALID_ATTACHMENT_PATH", "附件路径无效", 500)
    return target


def validate_file(filename: str, data: bytes) -> tuple[str, str]:
    if not data:
        raise AppError("EMPTY_FILE", "附件不能为空", 422)
    ext = Path(filename).suffix.lower()
    if ext in SIGNATURES:
        mime, signature = SIGNATURES[ext]
        if not data.startswith(signature):
            raise AppError("INVALID_FILE_SIGNATURE", "文件内容与扩展名不匹配", 422)
        return ext, mime
    if ext in TEXT_EXTENSIONS:
        try:
            data.decode("utf-8")
        except UnicodeDecodeError as exc:
            raise AppError("INVALID_FILE_SIGNATURE", "文本附件必须为 UTF-8", 422) from exc
        return ext, TEXT_EXTENSIONS[ext]
    raise AppError("UNSUPPORTED_FILE_TYPE", "不支持的附件类型", 422)


async def save_upload(
    db: Session, context: TenantContext, note_id: int, upload: UploadFile
) -> Attachment:
    settings = get_settings()
    data = await upload.read(settings.max_attachment_bytes + 1)
    if len(data) > settings.max_attachment_bytes:
        raise AppError("ATTACHMENT_TOO_LARGE", "附件超过单文件大小限制", 413)
    tenant = db.query(Tenant).filter(Tenant.id == context.tenant_id).one()
    used = (
        db.query(func.coalesce(func.sum(Attachment.size), 0))
        .filter(Attachment.tenant_id == context.tenant_id)
        .scalar()
        or 0
    )
    if used + len(data) > tenant.attachment_quota_bytes:
        raise AppError("ATTACHMENT_QUOTA_EXCEEDED", "附件空间配额不足", 409)
    ext, mime = validate_file(upload.filename or "", data)
    now = datetime.utcnow()
    relative = (
        Path("attachments")
        / str(context.tenant_id)
        / f"{now:%Y}"
        / f"{now:%m}"
        / f"{uuid4().hex}{ext}"
    )
    target = settings.data_dir / relative
    target.parent.mkdir(parents=True, exist_ok=True)
    try:
        target.write_bytes(data)
        item = Attachment(
            tenant_id=context.tenant_id,
            uploaded_by=context.user_id,
            note_id=note_id,
            original_name=Path(upload.filename or "attachment").name[:255],
            stored_path=relative.as_posix(),
            mime_type=mime,
            size=len(data),
            sha256=hashlib.sha256(data).hexdigest(),
        )
        db.add(item)
        db.flush()
        return item
    except Exception:
        target.unlink(missing_ok=True)
        raise
