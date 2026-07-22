from __future__ import annotations

import json
import os
from dataclasses import dataclass, field
from functools import lru_cache
from pathlib import Path
from typing import Any

BASE_DIR = Path(__file__).resolve().parents[2]


def _read_json(path: Path) -> dict[str, Any]:
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
        return value if isinstance(value, dict) else {}
    except (OSError, json.JSONDecodeError):
        return {}


@dataclass(frozen=True)
class Settings:
    database_url: str = field(repr=False)
    migration_database_url: str = field(repr=False)
    cors_origins: tuple[str, ...]
    log_level: str
    pool_size: int
    max_overflow: int
    pool_timeout: int
    statement_timeout_ms: int
    media_dir: Path
    secret_key: str = field(repr=False)
    ai: dict[str, Any] = field(repr=False)
    token_ttl_hours: int = 24 * 30

    def as_legacy_dict(self) -> dict[str, Any]:
        return {
            "database_url": self.database_url,
            "media_dir": str(self.media_dir),
            "secret_key": self.secret_key,
            "ai": self.ai,
        }


@lru_cache(maxsize=1)
def get_settings() -> Settings:
    raw = _read_json(BASE_DIR / "config.json")
    ai_raw = raw.get("ai") if isinstance(raw.get("ai"), dict) else {}
    database_url = os.getenv("DATABASE_URL") or str(raw.get("database_url") or "")
    if not database_url.startswith(("postgresql://", "postgresql+psycopg://")):
        raise RuntimeError("DATABASE_URL must use PostgreSQL (postgresql+psycopg://...)")
    origins_raw = os.getenv("CORS_ORIGINS")
    origins = tuple(x.strip() for x in origins_raw.split(",") if x.strip()) if origins_raw else tuple(raw.get("cors_origins") or ("http://127.0.0.1:5173", "http://localhost:5173"))
    if not origins or "*" in origins:
        raise RuntimeError("CORS_ORIGINS must contain explicit trusted origins")
    return Settings(
        database_url=database_url,
        migration_database_url=os.getenv("MIGRATION_DATABASE_URL", database_url),
        cors_origins=origins,
        log_level=os.getenv("LOG_LEVEL", "INFO").upper(),
        pool_size=int(os.getenv("DB_POOL_SIZE", "5")),
        max_overflow=int(os.getenv("DB_MAX_OVERFLOW", "10")),
        pool_timeout=int(os.getenv("DB_POOL_TIMEOUT", "10")),
        statement_timeout_ms=int(os.getenv("DB_STATEMENT_TIMEOUT_MS", "15000")),
        media_dir=BASE_DIR / "media",
        secret_key=os.getenv("SECRET_KEY") or str(raw.get("secret_key") or "dev-insecure-change-me"),
        ai={
            "api_key": os.getenv("AI_API_KEY") or ai_raw.get("api_key", ""),
            "base_url": os.getenv("AI_BASE_URL") or ai_raw.get("base_url", "https://api.deepseek.com/v1"),
            "model": os.getenv("AI_MODEL") or ai_raw.get("model", "deepseek-chat"),
            "system_prompt": os.getenv("AI_SYSTEM_PROMPT") or ai_raw.get("system_prompt", "你是一个温暖、贴心的 AI 助手。"),
        },
        token_ttl_hours=int(os.getenv("TOKEN_TTL_HOURS", "720")),
    )


def get_config() -> dict[str, Any]:
    return get_settings().as_legacy_dict()
