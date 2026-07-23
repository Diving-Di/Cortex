from datetime import date, datetime, timezone

import pytest

from app.core.exceptions import AppError
from app.services.dashboard_service import calculate_streak, local_day_bounds


def test_streak_includes_today_or_preserves_yesterday() -> None:
    today = date(2026, 7, 23)
    assert calculate_streak({today, date(2026, 7, 22)}, today) == 2
    assert calculate_streak({date(2026, 7, 22), date(2026, 7, 21)}, today) == 2
    assert calculate_streak({date(2026, 7, 21)}, today) == 0


def test_local_day_bounds_handles_timezone_and_cross_day() -> None:
    start, end = local_day_bounds(date(2026, 7, 23), "Asia/Shanghai")
    assert start == datetime(2026, 7, 22, 16, tzinfo=timezone.utc)
    assert end == datetime(2026, 7, 23, 16, tzinfo=timezone.utc)


def test_invalid_timezone_is_rejected() -> None:
    with pytest.raises(AppError):
        local_day_bounds(date(2026, 7, 23), "Mars/Olympus")
