package store

import (
	"testing"
	"time"
)

func TestAIEventWindow(t *testing.T) {
	now := time.Date(2026, 8, 1, 4, 0, 0, 0, time.UTC)
	date, opens, closes := configuredAIEventWindow(now, "Asia/Shanghai", 20, 0, 10)
	if date.Format(time.DateOnly) != "2026-08-01" {
		t.Fatalf("date=%s", date)
	}
	if opens.Format("15:04") != "20:00" || closes.Sub(opens) != 10*time.Minute {
		t.Fatalf("window %s - %s", opens, closes)
	}
}

func TestConfiguredAIEventWindowAcrossYearAndDST(t *testing.T) {
	date, opens, closes := configuredAIEventWindow(time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC), "Asia/Shanghai", 20, 0, 10)
	if date.Format(time.DateOnly) != "2026-01-01" || opens.Hour() != 20 || closes.Sub(opens) != 10*time.Minute {
		t.Fatalf("year boundary: %s %s %s", date, opens, closes)
	}
	_, opens, closes = configuredAIEventWindow(time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC), "America/New_York", 20, 0, 10)
	if opens.Hour() != 20 || closes.Sub(opens) != 10*time.Minute {
		t.Fatalf("DST boundary: %s %s", opens, closes)
	}
}
