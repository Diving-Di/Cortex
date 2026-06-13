from __future__ import annotations

from fastapi import APIRouter, Depends, HTTPException, status
from sqlalchemy.orm import Session

from ..database import get_db
from ..models import User
from ..schemas import LoginIn, LoginOut, RegisterIn
from ..security import create_token, hash_password, verify_password

router = APIRouter(prefix="/api", tags=["auth"])


@router.post("/register/", status_code=status.HTTP_201_CREATED)
def register(payload: RegisterIn, db: Session = Depends(get_db)) -> dict:
    username = payload.username.strip()
    email = payload.email.strip()
    password = payload.password

    if len(username) < 6:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail="用户名长度需至少 6 个字符")
    if len(password) < 6:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail="密码长度需至少 6 个字符")

    exists = (
        db.query(User)
        .filter((User.username == username) | (User.email == email))
        .first()
    )
    if exists:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail="用户名或邮箱已存在")

    user = User(username=username, email=email, password_hash=hash_password(password))
    db.add(user)
    db.commit()
    return {"detail": "registered"}


@router.post("/login/", response_model=LoginOut)
def login(payload: LoginIn, db: Session = Depends(get_db)) -> LoginOut:
    user = db.query(User).filter(User.username == payload.username.strip()).first()
    if not user or not verify_password(payload.password, user.password_hash):
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="用户名或密码错误",
        )

    token = create_token(db, user)
    return LoginOut(token=token, username=user.username)
