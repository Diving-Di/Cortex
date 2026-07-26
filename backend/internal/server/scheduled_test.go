package server

import (
    "testing"
    "time"
)

func TestNextScheduledRun(t *testing.T) {
    now := time.Date(2026, 7, 23, 13, 0, 0, 0, time.UTC)
    daily, err := nextScheduledRun("daily", 20, 0, "Asia/Shanghai", now)
    if err != nil {
        t.Fatal(err)
    }
    expected := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
    if !daily.Equal(expected) {
        t.Fatalf("daily next run = %s", daily)
    }
    weekly, err := nextScheduledRun("weekly", 20, 0, "Asia/Shanghai", now)
    if err != nil {
        t.Fatal(err)
    }
    zone, _ := time.LoadLocation("Asia/Shanghai")
    if weekly.In(zone).Weekday() != time.Sunday {
        t.Fatalf("weekly next run weekday = %s", weekly.In(zone).Weekday())
    }
    monthly, err := nextScheduledRun("monthly", 20, 0, "Asia/Shanghai", now)
    if err != nil {
        t.Fatal(err)
    }
    expectedMonthly := time.Date(2026, 7, 31, 20, 0, 0, 0, zone)
    if !monthly.In(zone).Equal(expectedMonthly) {
        t.Fatalf("monthly next run = %s", monthly.In(zone))
    }
}

func TestNextScheduledRunRejectsTimezone(t *testing.T) {
    if _, err := nextScheduledRun("daily", 20, 0, "Mars/Olympus", time.Now()); err == nil {
        t.Fatal("invalid timezone accepted")
    }
}

func TestNextScheduledRunRejectsReportType(t *testing.T) {
    if _, err := nextScheduledRun("yearly", 20, 0, "Asia/Shanghai", time.Now()); err == nil {
        t.Fatal("invalid report type accepted")
    }
}
