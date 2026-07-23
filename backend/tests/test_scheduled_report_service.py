from datetime import datetime, timezone

import pytest

from app.core.exceptions import AppError
from app.services.scheduled_report_service import next_run_at


def test_daily_next_run_uses_requested_timezone():
    now = datetime(2026, 7, 23, 13, 0, tzinfo=timezone.utc)  # 21:00 Shanghai
    assert next_run_at("daily", 20, 0, "Asia/Shanghai", now) == datetime(
        2026, 7, 24, 12, 0, tzinfo=timezone.utc
    )


def test_weekly_runs_on_sunday():
    now = datetime(2026, 7, 23, 0, 0, tzinfo=timezone.utc)
    result = next_run_at("weekly", 20, 0, "Asia/Shanghai", now)
    assert result.astimezone(__import__("zoneinfo").ZoneInfo("Asia/Shanghai")).weekday() == 6


def test_invalid_timezone_is_rejected():
    with pytest.raises(AppError):
        next_run_at("daily", 20, 0, "not/a-timezone")
