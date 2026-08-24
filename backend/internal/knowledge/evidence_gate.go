package knowledge

// EvidenceGateResult describes the candidates that survive the same rerank
// evidence gate used by online knowledge chat and offline evaluation.
type EvidenceGateResult struct {
	QualifiedIndexes []int `json:"qualified_indexes"`
	MarginConflict   bool  `json:"margin_conflict"`
	Passed           bool  `json:"passed"`
}

// EvaluateEvidenceGate applies the configured minimum score, minimum evidence
// count, and top-two margin to rerank scores in descending rank order.
func EvaluateEvidenceGate(scores []float64, minScore, minMargin *float64, minEvidence int) EvidenceGateResult {
	if minEvidence < 1 {
		minEvidence = 1
	}
	qualified := make([]int, 0, len(scores))
	for i, score := range scores {
		if minScore == nil || score >= *minScore {
			qualified = append(qualified, i)
		}
	}
	marginConflict := minMargin != nil && len(qualified) > 1 &&
		scores[qualified[0]]-scores[qualified[1]] < *minMargin
	return EvidenceGateResult{
		QualifiedIndexes: qualified,
		MarginConflict:   marginConflict,
		Passed:           len(qualified) >= minEvidence && !marginConflict,
	}
}
