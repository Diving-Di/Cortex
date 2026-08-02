package recipe

import (
	"context"
	"math"
	"strings"
	"testing"

	"diary-listener/backend/internal/ai"
	"diary-listener/backend/internal/store"
)

func TestRetrievalMetricsUsesNormalizedSourcePath(t *testing.T) {
	candidates := []store.RecipeCandidate{
		{SourcePath: "dishes/other.md"},
		{SourcePath: `dishes\vegetable_dish\酸辣土豆丝.md`},
	}
	metrics := retrievalMetrics(
		[]string{"backend/resources/howtocook/dishes/vegetable_dish/酸辣土豆丝.md"},
		candidates, candidates,
	)
	if metrics.HitAt1 != 0 || metrics.HitAt3 != 1 || metrics.MRRAfterRerank != 0.5 {
		t.Fatalf("unexpected metrics: %#v", metrics)
	}
}

func TestNormalizeSourcePathIsIndependentOfHostOS(t *testing.T) {
	const want = "dishes/vegetable_dish/酸辣土豆丝.md"
	for _, input := range []string{
		`backend\resources\howtocook\dishes\vegetable_dish\酸辣土豆丝.md`,
		"backend/resources/howtocook/dishes/vegetable_dish/酸辣土豆丝.md",
	} {
		if got := normalizeSourcePath(input); got != want {
			t.Fatalf("normalizeSourcePath(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestRetrievalMetricsShowsRerankRegression(t *testing.T) {
	gold := []string{"dishes/gold.md"}
	before := []store.RecipeCandidate{{SourcePath: "dishes/gold.md"}, {SourcePath: "dishes/noise.md"}}
	after := []store.RecipeCandidate{{SourcePath: "dishes/noise.md"}, {SourcePath: "dishes/gold.md"}}
	metrics := retrievalMetrics(gold, before, after)
	if metrics.MRRBeforeRerank != 1 || metrics.MRRAfterRerank != 0.5 {
		t.Fatalf("unexpected rerank comparison: %#v", metrics)
	}
}

func TestContextAveragePrecision(t *testing.T) {
	got := contextAveragePrecision([]JudgeContext{
		{Rank: 3, Relevant: true}, {Rank: 1, Relevant: true}, {Rank: 2, Relevant: false},
	})
	want := (1.0 + 2.0/3.0) / 2.0
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("contextAveragePrecision = %v, want %v", got, want)
	}
}

func TestDecodeJudgeAssessmentRejectsInvalidOutput(t *testing.T) {
	valid := `{"claims":[{"text":"a","supported":true,"evidence_ranks":[1]}],"question_intents":[{"text":"q","covered":true}],"reference_facts":[{"text":"r","supported":true,"evidence_ranks":[1]}],"context_relevance":[{"rank":1,"relevant":true}],"answer_relevancy":0.8}`
	value, err := decodeJudgeAssessment(valid, 1)
	if err != nil || value.AnswerRelevancy != 0.8 {
		t.Fatalf("valid judge output failed: %#v, %v", value, err)
	}
	for name, raw := range map[string]string{
		"range":   strings.Replace(valid, "0.8", "1.2", 1),
		"rank":    strings.Replace(valid, `"rank":1`, `"rank":2`, 1),
		"unknown": strings.Replace(valid, `"answer_relevancy":0.8`, `"answer_relevancy":0.8,"extra":true`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeJudgeAssessment(raw, 1); err == nil {
				t.Fatal("expected invalid judge output")
			}
		})
	}
}

func TestDecodeJudgeAssessmentAcceptsJSONFence(t *testing.T) {
	raw := "```json\n" + `{"claims":[{"text":"a","supported":true,"evidence_ranks":[1]}],"question_intents":[{"text":"q","covered":true}],"reference_facts":[{"text":"r","supported":true,"evidence_ranks":[1]}],"context_relevance":[{"rank":1,"relevant":true}],"answer_relevancy":1}` + "\n```"
	if _, err := decodeJudgeAssessment(raw, 1); err != nil {
		t.Fatal(err)
	}
}

type fakeEvalRetriever struct{}

func (fakeEvalRetriever) Search(context.Context, string, int) ([]store.RecipeCandidate, error) {
	return []store.RecipeCandidate{
		{ChunkID: 1, DocumentID: 1, SourcePath: "dishes/noise.md", Title: "noise", Snippet: "无关"},
		{ChunkID: 2, DocumentID: 2, SourcePath: "dishes/gold.md", Title: "gold", Snippet: "完整依据"},
	}, nil
}

func (fakeEvalRetriever) Rerank(_ context.Context, _ string, candidates []store.RecipeCandidate) ([]store.RecipeCandidate, error) {
	score := 0.9
	candidates[1].RerankScore = &score
	return []store.RecipeCandidate{candidates[1], candidates[0]}, nil
}

type fakeEvalGenerator struct{}

func (fakeEvalGenerator) AnswerKnowledge(context.Context, ai.KnowledgeInput) (<-chan ai.StreamEvent, error) {
	events := make(chan ai.StreamEvent, 1)
	events <- ai.StreamEvent{Content: "标准回答 [R1]"}
	close(events)
	return events, nil
}

type fakeEvalJudge struct{}

func (fakeEvalJudge) Evaluate(context.Context, JudgeInput) (JudgeAssessment, error) {
	return JudgeAssessment{
		Claims:           []JudgeClaim{{Text: "标准回答", Supported: true, EvidenceRanks: []int{1}}},
		QuestionIntents:  []JudgeIntent{{Text: "问题", Covered: true}},
		ReferenceFacts:   []JudgeClaim{{Text: "完整依据", Supported: true, EvidenceRanks: []int{1}}},
		ContextRelevance: []JudgeContext{{Rank: 1, Relevant: true}, {Rank: 2, Relevant: false}},
		AnswerRelevancy:  1,
	}, nil
}

func TestEvalRunnerRunsCurrentStagesAndScores(t *testing.T) {
	runner := EvalRunner{Retriever: fakeEvalRetriever{}, Generator: fakeEvalGenerator{}, Judge: fakeEvalJudge{},
		Config: EvalConfig{SearchLimit: 2, ContextTopK: 2}}
	result := runner.RunCase(context.Background(), EvalCase{ID: "case-1", Query: "问题",
		ReferenceAnswer: "完整依据", SourcePaths: []string{"backend/resources/howtocook/dishes/gold.md"}})
	if result.Status != "success" || result.Answer != "标准回答 [R1]" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if result.Metrics.MRRBeforeRerank != 0.5 || result.Metrics.MRRAfterRerank != 1 ||
		result.Metrics.ContextRecall != 1 || result.Metrics.ContextPrecision != 1 ||
		result.Metrics.Faithfulness != 1 || result.Metrics.AnswerRelevancy != 1 {
		t.Fatalf("unexpected metrics: %#v", result.Metrics)
	}
}

func TestSummarizeExcludesFailedCases(t *testing.T) {
	summary := Summarize("test", []EvalResult{
		{Status: "success", BeforeRerank: []CandidateTrace{{Rank: 1}}, AfterRerank: []CandidateTrace{{Rank: 1}}, Metrics: MetricScores{HitAt5: 1, Faithfulness: 0.8}},
		{Status: "failed", BeforeRerank: []CandidateTrace{{Rank: 1}}, AfterRerank: []CandidateTrace{{Rank: 1}}, Metrics: MetricScores{HitAt5: 0, Faithfulness: 0}},
	})
	if summary.Total != 2 || summary.Succeeded != 1 || summary.Failed != 1 ||
		summary.RetrievalEvaluated != 2 || summary.GenerationEvaluated != 1 ||
		summary.Metrics.HitAt5 != 0.5 || summary.Metrics.Faithfulness != 0.8 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
}
