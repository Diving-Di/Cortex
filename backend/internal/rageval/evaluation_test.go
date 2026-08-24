package rageval

import (
	"context"
	"errors"
	"math"
	"testing"

	"cortex/backend/internal/ai"
	"cortex/backend/internal/store"
	"github.com/google/uuid"
)

func TestRetrievalMetricsMatchesStoredPathWithoutCollection(t *testing.T) {
	items := []store.KnowledgeCandidate{
		{DocumentID: uuid.New(), Title: "其他", SourcePath: "knowledge/t/u/source/其他.md", RouteProvenance: 1},
		{DocumentID: uuid.New(), Title: "酸辣土豆丝", SourcePath: `knowledge\t\u\source\dishes\酸辣土豆丝.md`, RouteProvenance: 7},
	}
	m := retrievalMetrics([]string{"backend/resources/howtocook/dishes/酸辣土豆丝.md"}, items, items)
	if m.HitAt1 != 0 || m.HitAt3 != 1 || m.MRRAfter != .5 {
		t.Fatalf("unexpected metrics: %#v", m)
	}
}

func TestRouteMetrics(t *testing.T) {
	items := []store.KnowledgeCandidate{
		{DocumentID: uuid.New(), Title: "gold", SourcePath: "test/gold.md", RouteProvenance: 1}, // vector only
	}
	rm := ComputeRouteMetrics([]Result{{
		SourcePaths: []string{"test/gold.md"},
		BeforeRerank: []CandidateTrace{
			{DocumentID: items[0].DocumentID.String(), Title: "gold", RouteProvenance: 1},
		},
	}})
	if rm.VectorHitAt10 < 0.99 || rm.FulltextHitAt10 > 0.01 || rm.VectorOnlyHitAt10 < 0.99 {
		t.Fatalf("expected vector-only hit: %#v", rm)
	}
}

func TestRouteMetricsDoesNotTreatNonGoldCandidateAsHit(t *testing.T) {
	rm := ComputeRouteMetrics([]Result{{
		SourcePaths: []string{"test/gold.md"},
		BeforeRerank: []CandidateTrace{{
			DocumentID: uuid.NewString(), Title: "noise", RouteProvenance: 7,
		}},
	}})
	if rm.VectorHitAt10 != 0 || rm.FulltextHitAt10 != 0 || rm.TitleHitAt10 != 0 || rm.AllThree != 0 {
		t.Fatalf("non-gold candidate was counted as a route hit: %#v", rm)
	}
}

func TestRouteMetricsFulltextIncremental(t *testing.T) {
	items := []store.KnowledgeCandidate{
		{DocumentID: uuid.New(), Title: "gold", SourcePath: "test/gold.md", RouteProvenance: 2}, // fulltext only
	}
	rm := ComputeRouteMetrics([]Result{{
		SourcePaths: []string{"test/gold.md"},
		BeforeRerank: []CandidateTrace{
			{DocumentID: items[0].DocumentID.String(), Title: "gold", RouteProvenance: 2},
		},
	}})
	if rm.FulltextHitAt10 < 0.99 || rm.FulltextIncremental < 0.99 {
		t.Fatalf("expected fulltext incremental: %#v", rm)
	}
}

func TestRouteMetricsAllThree(t *testing.T) {
	items := []store.KnowledgeCandidate{
		{DocumentID: uuid.New(), Title: "gold", SourcePath: "test/gold.md", RouteProvenance: 7}, // all three
	}
	rm := ComputeRouteMetrics([]Result{{
		SourcePaths: []string{"test/gold.md"},
		BeforeRerank: []CandidateTrace{
			{DocumentID: items[0].DocumentID.String(), Title: "gold", RouteProvenance: 7},
		},
	}})
	if rm.VectorHitAt10 < 0.99 || rm.FulltextHitAt10 < 0.99 || rm.TitleHitAt10 < 0.99 || rm.AllThree < 0.99 {
		t.Fatalf("expected all-three hit: %#v", rm)
	}
}

func TestAveragePrecision(t *testing.T) {
	got := averagePrecision([]ContextJudgment{{Rank: 3, Relevant: true}, {Rank: 1, Relevant: true}, {Rank: 2, Relevant: false}})
	want := (1.0 + 2.0/3.0) / 2.0
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestValidateSummary(t *testing.T) {
	summary := Summary{Failed: 1, Metrics: Metrics{HitAt10: .9, ContextRecall: .7}}
	err := ValidateSummary(summary, Thresholds{MaxFailed: 0, HitAt10: .95, ContextRecall: .8})
	if err == nil {
		t.Fatal("expected threshold failure")
	}
	if err := ValidateSummary(summary, Thresholds{MaxFailed: 1, HitAt10: .9, ContextRecall: .7}); err != nil {
		t.Fatalf("expected thresholds to pass: %v", err)
	}
}

func TestSummarizeCountsSuccessfulEmptyRetrievalAsMiss(t *testing.T) {
	summary := Summarize("test.jsonl", []Result{
		{Status: "success", Metrics: Metrics{HitAt10: 1, MRRBefore: 1, MRRAfter: 1}, BeforeRerank: []CandidateTrace{{Title: "gold"}}, AfterRerank: []CandidateTrace{{Title: "gold"}}},
		{Status: "success"},
	})
	if summary.RetrievalEvaluated != 2 || summary.Metrics.HitAt10 != 0.5 || summary.Metrics.MRRBefore != 0.5 {
		t.Fatalf("empty retrieval was excluded from denominator: %#v", summary)
	}
	if summary.GenerationEvaluated != 0 {
		t.Fatalf("retrieval-only results counted as generation: %#v", summary)
	}
}

func TestSummarizeBuildsTagLayersWithoutRecursiveLayers(t *testing.T) {
	summary := Summarize("test.jsonl", []Result{
		{Status: "success", Tags: []string{"fact", "easy", "fact"}, Metrics: Metrics{HitAt10: 1}},
		{Status: "success", Tags: []string{"fact", "hard"}},
	})
	fact, ok := summary.Layers["fact"]
	if !ok || fact.Total != 2 || fact.Metrics.HitAt10 != 0.5 || fact.Layers != nil {
		t.Fatalf("unexpected fact layer: %#v", fact)
	}
	if summary.Layers["easy"].Total != 1 || summary.Layers["hard"].Total != 1 {
		t.Fatalf("unexpected tag layers: %#v", summary.Layers)
	}
}

func TestRouteMetricsCountsSuccessfulEmptyRetrievalAsMiss(t *testing.T) {
	rm := ComputeRouteMetrics([]Result{
		{Status: "success", SourcePaths: []string{"gold.md"}, BeforeRerank: []CandidateTrace{{Title: "gold", RouteProvenance: routeTitle}}},
		{Status: "success", SourcePaths: []string{"missing.md"}},
	})
	if rm.TitleHitAt10 != 0.5 || rm.TitleIncremental != 0.5 {
		t.Fatalf("empty route result was excluded from denominator: %#v", rm)
	}
}

type fakeRetriever struct{}

func (fakeRetriever) Search(context.Context, string, int) ([]store.KnowledgeCandidate, error) {
	return []store.KnowledgeCandidate{{DocumentID: uuid.New(), Title: "noise", Content: "无关", RouteProvenance: 1}, {DocumentID: uuid.New(), Title: "gold", Content: "依据", RouteProvenance: 7}}, nil
}
func (fakeRetriever) Rerank(_ context.Context, _ string, v []store.KnowledgeCandidate) ([]store.KnowledgeCandidate, error) {
	score := .9
	v[1].RerankScore = &score
	return []store.KnowledgeCandidate{v[1], v[0]}, nil
}

type fakeGenerator struct{}

func (fakeGenerator) AnswerKnowledge(context.Context, ai.KnowledgeInput) (<-chan ai.StreamEvent, error) {
	ch := make(chan ai.StreamEvent, 1)
	ch <- ai.StreamEvent{Content: "回答 [K1]"}
	close(ch)
	return ch, nil
}

type fakeJudge struct{}

func (fakeJudge) Evaluate(_ context.Context, input JudgeInput) (Assessment, error) {
	if len(input.Contexts) != 2 || input.Contexts[0].Content != "依据" {
		return Assessment{}, errors.New("judge did not receive full reranked context")
	}
	return Assessment{Claims: []Claim{{Text: "回答", Supported: true}}, QuestionIntents: []Intent{{Text: "问题", Covered: true}}, ReferenceFacts: []Claim{{Text: "依据", Supported: true}}, ContextRelevance: []ContextJudgment{{Rank: 1, Relevant: true}, {Rank: 2}}, AnswerRelevancy: 1}, nil
}

func TestRunnerUsesKnowledgePipeline(t *testing.T) {
	r := Runner{Retriever: fakeRetriever{}, Generator: fakeGenerator{}, Judge: fakeJudge{}, Config: Config{SearchLimit: 2, ContextTopK: 2}}
	got := r.RunCase(context.Background(), Case{ID: "1", Query: "问题", ReferenceAnswer: "依据", SourcePaths: []string{"gold.md"}})
	if got.Status != "success" || got.Metrics.MRRAfter != 1 || got.Metrics.Faithfulness != 1 {
		t.Fatalf("unexpected result: %#v", got)
	}
}

type panicGenerator struct{}

func (panicGenerator) AnswerKnowledge(context.Context, ai.KnowledgeInput) (<-chan ai.StreamEvent, error) {
	panic("generator must not run when the online evidence gate rejects")
}

func TestRunnerUsesOnlineEvidenceGateBeforeGeneration(t *testing.T) {
	minScore := 0.5
	minMargin := 0.2
	r := Runner{
		Retriever: fakeRetriever{},
		Generator: panicGenerator{},
		Judge:     fakeJudge{},
		Config: Config{
			SearchLimit:          2,
			ContextTopK:          2,
			RerankMinScore:       &minScore,
			RerankMinMargin:      &minMargin,
			MinQualifiedEvidence: 2,
		},
	}
	got := r.RunCase(context.Background(), Case{ID: "gate", Query: "问题", ReferenceAnswer: "依据", SourcePaths: []string{"gold.md"}})
	if got.Status != "success" || got.EvidenceGate == nil || got.EvidenceGate.Passed || got.Judge != nil || got.Answer != "" {
		t.Fatalf("offline runner diverged from online evidence gate: %#v", got)
	}
}

func TestGoldTitleFromNonRecipePaths(t *testing.T) {
	path := "backend/testdata/rag/non_recipe_notes/Go并发模式学习笔记.md"
	if got := goldTitle(path); got != "Go并发模式学习笔记" {
		t.Fatalf("goldTitle(%q) = %q, want %q", path, got, "Go并发模式学习笔记")
	}
	path2 := "backend/testdata/rag/non_recipe_notes/2025年度工作总结.md"
	if got := goldTitle(path2); got != "2025年度工作总结" {
		t.Fatalf("goldTitle(%q) = %q, want %q", path2, got, "2025年度工作总结")
	}
}
