package knowledge

import "testing"

func TestEvaluateEvidenceGateMatchesOnlineRules(t *testing.T) {
	minScore := 0.5
	minMargin := 0.1

	got := EvaluateEvidenceGate([]float64{0.9, 0.7, 0.4}, &minScore, &minMargin, 2)
	if !got.Passed || got.MarginConflict || len(got.QualifiedIndexes) != 2 {
		t.Fatalf("unexpected passing gate: %#v", got)
	}

	got = EvaluateEvidenceGate([]float64{0.9, 0.85, 0.4}, &minScore, &minMargin, 2)
	if got.Passed || !got.MarginConflict {
		t.Fatalf("small margin passed gate: %#v", got)
	}

	got = EvaluateEvidenceGate([]float64{0.9, 0.4}, &minScore, nil, 2)
	if got.Passed || len(got.QualifiedIndexes) != 1 {
		t.Fatalf("insufficient evidence passed gate: %#v", got)
	}
}
