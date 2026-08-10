package rageval

import (
	"context"
	"errors"
	"math"
	"testing"

	"diary-listener/backend/internal/ai"
	"diary-listener/backend/internal/store"
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
