package store

import "testing"

func TestStreakDays(t *testing.T) {
	if value := streakDays(map[string]bool{"2026-07-23": true, "2026-07-22": true}, "2026-07-23"); value != 2 {
		t.Fatalf("streak including today = %d", value)
	}
	if value := streakDays(map[string]bool{"2026-07-22": true, "2026-07-21": true}, "2026-07-23"); value != 2 {
		t.Fatalf("streak through yesterday = %d", value)
	}
	if value := streakDays(map[string]bool{"2026-07-21": true}, "2026-07-23"); value != 0 {
		t.Fatalf("broken streak = %d", value)
	}
}
