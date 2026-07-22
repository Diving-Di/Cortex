import pytest

from app.core.config import get_settings


def test_postgresql_is_required(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("DATABASE_URL", "sqlite:///forbidden.db")
    get_settings.cache_clear()
    with pytest.raises(RuntimeError, match="PostgreSQL"):
        get_settings()
    get_settings.cache_clear()
