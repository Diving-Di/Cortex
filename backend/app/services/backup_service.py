from __future__ import annotations
import hashlib, io, json, zipfile
from datetime import date, datetime
from pathlib import Path, PurePosixPath
from uuid import uuid4
from sqlalchemy.orm import Session
from ..core.config import get_settings
from ..core.exceptions import AppError
from ..core.tenant import TenantContext
from ..models import Attachment, Note, Tag, note_tags
from .attachment_service import safe_attachment_path, validate_file

def _safe_name(name: str) -> bool:
    path = PurePosixPath(name)
    return not path.is_absolute() and ".." not in path.parts and "\\" not in name

def create_backup(db: Session, context: TenantContext) -> bytes:
    notes = db.query(Note).filter(Note.tenant_id == context.tenant_id, Note.deleted_at.is_(None)).all(); tags = db.query(Tag).filter(Tag.tenant_id == context.tenant_id).all(); attachments = db.query(Attachment).filter(Attachment.tenant_id == context.tenant_id).all()
    links = db.execute(note_tags.select().where(note_tags.c.tenant_id == context.tenant_id)).mappings().all()
    data = {"version": 1, "notes": [{"id": n.id, "type": n.type, "title": n.title, "content": n.content, "note_date": n.note_date.isoformat() if n.note_date else None, "summary": n.summary} for n in notes], "tags": [{"id": t.id, "name": t.name, "color": t.color} for t in tags], "note_tags": [{"note_id": x["note_id"], "tag_id": x["tag_id"]} for x in links], "attachments": []}
    files = {}
    for item in attachments:
        content = safe_attachment_path(item.stored_path).read_bytes(); arc = f"attachments/{item.id}/{PurePosixPath(item.original_name).name}"
        files[arc] = content; data["attachments"].append({"id": item.id, "note_id": item.note_id, "name": item.original_name, "mime": item.mime_type, "sha256": hashlib.sha256(content).hexdigest(), "archive": arc})
    payload = json.dumps(data, ensure_ascii=False, separators=(",", ":")).encode(); files["data.json"] = payload
    manifest = {name: hashlib.sha256(content).hexdigest() for name, content in files.items()}
    out = io.BytesIO()
    with zipfile.ZipFile(out, "w", zipfile.ZIP_DEFLATED) as z:
        for name, content in files.items(): z.writestr(name, content)
        z.writestr("manifest.json", json.dumps(manifest, separators=(",", ":")))
    return out.getvalue()

def validate_backup(blob: bytes) -> tuple[dict, dict[str, bytes]]:
    try:
        with zipfile.ZipFile(io.BytesIO(blob)) as z:
            names = z.namelist()
            if any(not _safe_name(name) for name in names): raise AppError("UNSAFE_BACKUP_PATH", "备份包包含非法路径", 422)
            if "manifest.json" not in names or "data.json" not in names: raise AppError("INVALID_BACKUP", "备份包缺少清单", 422)
            files = {name: z.read(name) for name in names if name != "manifest.json"}; manifest = json.loads(z.read("manifest.json"))
            for name, digest in manifest.items():
                if name not in files or hashlib.sha256(files[name]).hexdigest() != digest: raise AppError("BACKUP_INTEGRITY_ERROR", "备份完整性校验失败", 422)
            return json.loads(files["data.json"]), files
    except zipfile.BadZipFile as exc: raise AppError("INVALID_BACKUP", "备份包格式无效", 422) from exc

def restore_backup(db: Session, context: TenantContext, blob: bytes) -> dict[str, int]:
    data, files = validate_backup(blob)
    if data.get("version") != 1: raise AppError("UNSUPPORTED_BACKUP_VERSION", "不支持的备份版本", 422)
    if db.query(Note).filter(Note.tenant_id == context.tenant_id, Note.deleted_at.is_(None)).count(): raise AppError("RESTORE_TARGET_NOT_EMPTY", "恢复目标必须为空", 409)
    written = []; note_map = {}; tag_map = {}
    try:
        for source in data.get("notes", []):
            item = Note(tenant_id=context.tenant_id, created_by=context.user_id, updated_by=context.user_id, type=source["type"], title=source["title"], content=source.get("content", ""), note_date=date.fromisoformat(source["note_date"]) if source.get("note_date") else None, summary=source.get("summary"), word_count=len("".join(source.get("content", "").split())))
            db.add(item); db.flush(); note_map[source["id"]] = item.id
        for source in data.get("tags", []):
            item = Tag(tenant_id=context.tenant_id, name=source["name"], color=source.get("color")); db.add(item); db.flush(); tag_map[source["id"]] = item.id
        for link in data.get("note_tags", []): db.execute(note_tags.insert().values(tenant_id=context.tenant_id, note_id=note_map[link["note_id"]], tag_id=tag_map[link["tag_id"]]))
        for source in data.get("attachments", []):
            content = files[source["archive"]]
            if hashlib.sha256(content).hexdigest() != source["sha256"]: raise AppError("BACKUP_INTEGRITY_ERROR", "附件摘要不匹配", 422)
            ext, detected_mime = validate_file(source["name"], content)
            relative = PurePosixPath("attachments") / str(context.tenant_id) / f"{datetime.utcnow():%Y/%m}" / f"{uuid4().hex}{ext}"
            target = get_settings().data_dir / Path(str(relative)); target.parent.mkdir(parents=True, exist_ok=True); target.write_bytes(content); written.append(target)
            db.add(Attachment(tenant_id=context.tenant_id, uploaded_by=context.user_id, note_id=note_map[source["note_id"]], original_name=PurePosixPath(source["name"]).name, stored_path=relative.as_posix(), mime_type=detected_mime, size=len(content), sha256=source["sha256"]))
        db.flush(); return {"notes": len(note_map), "tags": len(tag_map), "attachments": len(data.get("attachments", []))}
    except Exception:
        for target in written: target.unlink(missing_ok=True)
        raise
