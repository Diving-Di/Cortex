package rageval

import (
	"math"
	"testing"
)

func scoreResult(score float64, title string) Result {
	return Result{AfterRerank: []CandidateTrace{{Title: title, RerankScore: &score}}}
}

func TestCalibrateRerankThresholdPrefersZeroFalseAnswersWithinRecallFloor(t *testing.T) {
	yes, no := true, false
	cases := []Case{
		{Answerable: &yes, SourcePaths: []string{"gold-a.md"}},
		{Answerable: &yes, SourcePaths: []string{"gold-b.md"}},
		{Answerable: &no}, {Answerable: &no},
	}
	results := []Result{
		scoreResult(0.9, "gold-a"), scoreResult(0.7, "gold-b"),
		scoreResult(0.6, "noise"), scoreResult(0.2, "noise"),
	}
	got := CalibrateRerankThreshold(cases, results, 1.0)
	if math.Abs(got.RecommendedThreshold-0.65) > 1e-12 {
		t.Fatalf("recommended threshold=%v, want midpoint 0.65", got.RecommendedThreshold)
	}
}

func TestCalibrateRerankThresholdDoesNotSacrificeRecallWhenFalseRateTies(t *testing.T) {
	yes, no := true, false
	cases := []Case{{Answerable: &yes, SourcePaths: []string{"a.md"}}, {Answerable: &yes, SourcePaths: []string{"b.md"}}, {Answerable: &no}}
	results := []Result{scoreResult(0.99, "a"), scoreResult(0.97, "b"), scoreResult(0.03, "noise")}
	got := CalibrateRerankThreshold(cases, results, 0.5)
	if got.RecommendedThreshold != 0.5 {
		t.Fatalf("recommended threshold=%v, want separating midpoint 0.5", got.RecommendedThreshold)
	}
}
