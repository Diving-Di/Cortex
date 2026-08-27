package store

import (
	"testing"
	"time"
)

func TestAuthTouchDue(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	stale := now.Add(-6 * time.Minute)
	recent := now.Add(-4 * time.Minute)
	if !authTouchDue(nil, now) {
		t.Fatal("a token without last_used_at must be touched")
	}
	if !authTouchDue(&stale, now) {
		t.Fatal("a stale token must be touched")
	}
	if authTouchDue(&recent, now) {
		t.Fatal("a recently used token must not be touched")
	}
}
