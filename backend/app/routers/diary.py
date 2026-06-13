from __future__ import annotations

import os
import secrets
from typing import Optional

from fastapi import APIRouter, Depends, File, Form, HTTPException, Response, UploadFile, status
from sqlalchemy.orm import Session

from ..config import get_config
from ..database import get_db
from ..models import DiaryEntry, User
from ..schemas import DiaryOut
from ..security import get_current_user

router = APIRouter(prefix="/api/diary", tags=["diary"])

_ALLOWED_EXT = {".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp"}


def _media_root() -> str:
    root = os.path.join(get_config()["media_dir"], "diary")
    os.makedirs(root, exist_ok=True)
    return root


def _serialize(entry: DiaryEntry) -> DiaryOut:
    image_url = f"/media/diary/{entry.image_path}" if entry.image_path else None
    return DiaryOut(
        id=entry.id,
        image=image_url,
        content=entry.content or "",
        created_at=entry.created_at,
    )


@router.get("/", response_model=list[DiaryOut])
def list_entries(
    db: Session = Depends(get_db),
    user: User = Depends(get_current_user),
) -> list[DiaryOut]:
    entries = (
        db.query(DiaryEntry)
        .filter(DiaryEntry.user_id == user.id)
        .order_by(DiaryEntry.created_at.desc())
        .all()
    )
    return [_serialize(e) for e in entries]


@router.post("/", response_model=DiaryOut, status_code=status.HTTP_201_CREATED)
def create_entry(
    content: str = Form(default=""),
    image: Optional[UploadFile] = File(default=None),
    db: Session = Depends(get_db),
    user: User = Depends(get_current_user),
) -> DiaryOut:
    text = (content or "").strip()
    stored_name: Optional[str] = None

    if image is not None and image.filename:
        ext = os.path.splitext(image.filename)[1].lower()
        if ext not in _ALLOWED_EXT:
            raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail="不支持的图片格式")
        data = image.file.read()
        if not data:
            raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail="图片内容为空")
        stored_name = f"{secrets.token_hex(8)}{ext}"
        with open(os.path.join(_media_root(), stored_name), "wb") as f:
            f.write(data)

    if not text and not stored_name:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail="请上传图片或填写内容")

    entry = DiaryEntry(user_id=user.id, image_path=stored_name, content=text)
    db.add(entry)
    db.commit()
    db.refresh(entry)
    return _serialize(entry)


@router.delete("/{entry_id}/", status_code=status.HTTP_204_NO_CONTENT)
def delete_entry(
    entry_id: int,
    db: Session = Depends(get_db),
    user: User = Depends(get_current_user),
) -> Response:
    entry = (
        db.query(DiaryEntry)
        .filter(DiaryEntry.id == entry_id, DiaryEntry.user_id == user.id)
        .first()
    )
    if not entry:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="日记不存在")

    if entry.image_path:
        try:
            os.remove(os.path.join(_media_root(), entry.image_path))
        except OSError:
            pass

    db.delete(entry)
    db.commit()
    return Response(status_code=status.HTTP_204_NO_CONTENT)
