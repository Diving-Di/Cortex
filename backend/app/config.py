"""Runtime configuration loader.

Configuration precedence (highest to lowest):
1. Existing environment variables (never overwritten)
2. backend/config.json  (gitignored, holds secrets like the AI api key)
3. Built-in defaults

The AI section follows an OpenAI / DeepSeek compatible schema so the project can
talk to DeepSeek (or any OpenAI-compatible endpoint) by only filling config.json.
"""
from __future__ import annotations

import json
import os
from functools import lru_cache
from pathlib import Path
from typing import Any, Dict

BASE_DIR = Path(__file__).resolve().parent.parent


def _read_json(path: Path) -> Dict[str, Any]:
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
        return data if isinstance(data, dict) else {}
    except Exception:
        return {}


@lru_cache(maxsize=1)
def get_config() -> Dict[str, Any]:
    file_cfg = _read_json(BASE_DIR / "config.json")

    ai_cfg = file_cfg.get("ai", {}) if isinstance(file_cfg.get("ai"), dict) else {}

    def pick(env_key: str, cfg_value: Any, default: Any) -> Any:
        if os.environ.get(env_key):
            return os.environ[env_key]
        if cfg_value not in (None, ""):
            return cfg_value
        return default

    config = {
        "secret_key": pick("SECRET_KEY", file_cfg.get("secret_key"), "dev-insecure-change-me"),
        "media_dir": str(BASE_DIR / "media"),
        "database_url": _build_database_url(file_cfg),
        "ai": {
            # DeepSeek compatible defaults
            "api_key": pick("AI_API_KEY", ai_cfg.get("api_key"), ""),
            "base_url": pick("AI_BASE_URL", ai_cfg.get("base_url"), "https://api.deepseek.com/v1"),
            "model": pick("AI_MODEL", ai_cfg.get("model"), "deepseek-chat"),
            "system_prompt": pick(
                "AI_SYSTEM_PROMPT",
                ai_cfg.get("system_prompt"),
                "你是一个温暖、贴心的 AI 助手，用简洁自然的中文与用户多轮对话。",
            ),
        },
    }
    return config


def _build_database_url(file_cfg: Dict[str, Any]) -> str:
    if os.environ.get("DATABASE_URL"):
        return os.environ["DATABASE_URL"]
    if file_cfg.get("database_url"):
        return str(file_cfg["database_url"])

    # Docker provides DB_HOST -> use MySQL, otherwise fall back to local SQLite.
    if os.environ.get("DB_HOST"):
        name = os.environ.get("DB_NAME", "diary_db")
        user = os.environ.get("DB_USER", "user")
        password = os.environ.get("DB_PASSWORD", "password")
        host = os.environ.get("DB_HOST", "db")
        port = os.environ.get("DB_PORT", "3306")
        return f"mysql+pymysql://{user}:{password}@{host}:{port}/{name}?charset=utf8mb4"

    return f"sqlite:///{(BASE_DIR / 'diary.sqlite3').as_posix()}"
