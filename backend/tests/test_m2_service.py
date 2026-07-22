from datetime import date
import pytest
from app.core.exceptions import AppError
from app.services.m2_service import parse_memory_query, report_range, snippet


def test_report_ranges_cover_calendar_periods():
    assert report_range("daily", date(2026, 7, 23)) == (date(2026, 7, 23), date(2026, 7, 23))
    assert report_range("weekly", date(2026, 7, 23)) == (date(2026, 7, 20), date(2026, 7, 26))
    assert report_range("monthly", date(2024, 2, 10)) == (date(2024, 2, 1), date(2024, 2, 29))
    with pytest.raises(AppError):
        report_range("yearly", date(2026, 1, 1))


def test_memory_query_extracts_relative_and_explicit_dates():
    assert parse_memory_query("上周杭州做了什么", date(2026, 7, 23))[:2] == (
        date(2026, 7, 13),
        date(2026, 7, 19),
    )
    start, end, words = parse_memory_query("2026年7月20日去了杭州西湖", date(2026, 7, 23))
    assert start == end == date(2026, 7, 20)
    assert any("杭州西湖" in word for word in words)


def test_snippet_enforces_context_limit():
    assert snippet(" a\n b   c ", 4) == "a b "
