package server

import (
	"testing"
	"time"
)

func TestResearchJitterIsBoundedAndStable(t *testing.T) {
	base := time.Second
	first := researchJitter(base, 42, 3)
	second := researchJitter(base, 42, 3)
	if first != second || first < 800*time.Millisecond || first > 1200*time.Millisecond {
		t.Fatalf("jitter=%s second=%s", first, second)
	}
}

func TestResearchBackoffIsBounded(t *testing.T) {
	if got := researchBackoff(1); got != 15*time.Second {
		t.Fatalf("first backoff=%s", got)
	}
	if got := researchBackoff(99); got != 240*time.Second {
		t.Fatalf("bounded backoff=%s", got)
	}
}
