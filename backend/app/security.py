from __future__ import annotations

import hashlib
import hmac
import secrets
from datetime import datetime, timedelta, timezone
from typing import Optional

from fastapi import Depends, Header, HTTPException, status
from sqlalchemy.orm import Session

from .core.config import get_settings
from .database import get_db
from .models import AuthToken, User

_PBKDF2_ROUNDS = 260000


def hash_password(password: str) -> str:
    salt = secrets.token_hex(16)
    digest = hashlib.pbkdf2_hmac("sha256", password.encode(), salt.encode(), _PBKDF2_ROUNDS).hex()
    return f"pbkdf2_sha256${_PBKDF2_ROUNDS}${salt}${digest}"


def verify_password(password: str, stored: str) -> bool:
    try:
        algo, rounds, salt, digest = stored.split("$")
        computed = hashlib.pbkdf2_hmac("sha256", password.encode(), salt.encode(), int(rounds)).hex()
        return algo == "pbkdf2_sha256" and hmac.compare_digest(computed, digest)
    except (ValueError, TypeError):
        return False


def hash_token(raw_token: str) -> str:
    return hashlib.sha256(raw_token.encode("utf-8")).hexdigest()


def create_token(db: Session, user: User) -> str:
    raw_token = secrets.token_urlsafe(32)
    now = datetime.now(timezone.utc)
    db.add(AuthToken(token_hash=hash_token(raw_token), user_id=user.id, expires_at=now + timedelta(hours=get_settings().token_ttl_hours)))
    db.flush()
    return raw_token


def get_current_auth_token(
    authorization: Optional[str] = Header(default=None),
    db: Session = Depends(get_db),
) -> AuthToken:
    scheme, _, raw_token = (authorization or "").partition(" ")
    if scheme.lower() not in {"token", "bearer"} or not raw_token.strip():
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="Authentication credentials were not provided.")
    now = datetime.now(timezone.utc)
    token = db.query(AuthToken).filter(AuthToken.token_hash == hash_token(raw_token.strip()), AuthToken.revoked_at.is_(None), AuthToken.expires_at > now).first()
    if token is None:
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="Invalid or expired token.")
    token.last_used_at = now
    return token


def get_current_user(token: AuthToken = Depends(get_current_auth_token)) -> User:
    return token.user
