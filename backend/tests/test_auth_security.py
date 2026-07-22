from datetime import datetime, timedelta, timezone

from app.core.logging import redact
from app.security import hash_token


def test_token_hash_is_one_way_and_stable() -> None:
    raw = "raw-secret-token"
    digest = hash_token(raw)
    assert raw not in digest
    assert len(digest) == 64
    assert digest == hash_token(raw)


def test_log_redaction_masks_credentials() -> None:
    value = "Authorization: Bearer abc password=hunter2 postgresql://user:secret@localhost/db"
    redacted = redact(value)
    assert "abc" not in redacted
    assert "hunter2" not in redacted
    assert ":secret@" not in redacted
    assert redacted.count("***") == 3
