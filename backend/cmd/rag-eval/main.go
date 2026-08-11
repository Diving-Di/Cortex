package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"diary-listener/backend/internal/ai"
	"diary-listener/backend/internal/config"
	"diary-listener/backend/internal/domain"
	"diary-listener/backend/internal/rageval"
	"diary-listener/backend/internal/store"
)

const evaluationUsername = "Diving"

type knowledgeRetriever struct {
	store                              *store.Store
	principal                          domain.Principal
	embedding                          ai.LocalEmbeddingClient
	reranker                           ai.LocalRerankClient
	model                              string
	vectorTopK, titleTopK, keywordTopK int
}

func (r knowledgeRetriever) Search(ctx context.Context, query string, limit int) ([]store.KnowledgeCandidate, error) {
	vectors, err := withRetry(ctx, 3, 500*time.Millisecond, func() ([][]float32, error) {
		return r.embedding.Embed(ctx, []string{query})
	})
	if err != nil {
		return nil, err
	}
	return r.store.SearchKnowledge(ctx, r.principal, query, vectors[0], r.model, nil,
		r.vectorTopK, r.titleTopK, r.keywordTopK, limit)
}

func withRetry[T any](ctx context.Context, attempts int, initialDelay time.Duration, call func() (T, error)) (T, error) {
	var zero T
	var err error
	for attempt := 0; attempt < attempts; attempt++ {
		var value T
		value, err = call()
		if err == nil {
			return value, nil
		}
		if attempt == attempts-1 {
			break
		}
		delay := initialDelay * time.Duration(1<<attempt)
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return zero, ctx.Err()
		}
	}
	return zero, err
}

func (r knowledgeRetriever) Rerank(ctx context.Context, query string, items []store.KnowledgeCandidate) ([]store.KnowledgeCandidate, error) {
	documents := make([]string, len(items))
	for i := range items {
		documents[i] = ai.FormatRerankDocument(items[i].Title, items[i].SourceType, items[i].Heading, items[i].Content)
	}
	scores, err := r.reranker.Rerank(ctx, query, documents)
	if err != nil {
		return nil, err
	}
	out := append([]store.KnowledgeCandidate(nil), items...)
	for i := range out {
		score := scores[i]
		out[i].RerankScore = &score
	}
	sort.SliceStable(out, func(i, j int) bool { return *out[i].RerankScore > *out[j].RerankScore })
	for i := range out {
		out[i].Rank = i + 1
	}
	return out, nil
}

func main() {
	var dataset, outputRoot, caseIDs string
	var workers, searchLimit, contextTopK int
	var preflightOnly bool
	var calibrateOnly bool
	var minimumAnswerableRecall float64
	var thresholds rageval.Thresholds
	flag.StringVar(&dataset, "dataset", "testdata/rag/knowledge_eval_merged.jsonl", "JSONL evaluation dataset")
	flag.StringVar(&outputRoot, "output", "../artifacts/rag-eval", "artifact output root")
	flag.IntVar(&workers, "workers", 1, "parallel case workers")
	flag.IntVar(&searchLimit, "search-limit", 20, "retrieval candidate count")
	flag.IntVar(&contextTopK, "context-top-k", 5, "reranked contexts sent to generation and judge")
	flag.StringVar(&caseIDs, "case-ids", "", "optional comma-separated case IDs")
	flag.BoolVar(&preflightOnly, "preflight-only", false, "validate Diving and dataset documents without calling AI")
	flag.BoolVar(&calibrateOnly, "calibrate-only", false, "run retrieval/rerank only and scan rejection thresholds")
	flag.Float64Var(&minimumAnswerableRecall, "min-answerable-recall", 0.8, "minimum recall used to select a calibrated threshold")
	flag.Float64Var(&thresholds.HitAt10, "min-hit-at-10", 0, "minimum aggregate Hit@10")
	flag.Float64Var(&thresholds.ContextRecall, "min-context-recall", 0, "minimum context recall")
	flag.Float64Var(&thresholds.ContextPrecision, "min-context-precision", 0, "minimum context precision")
	flag.Float64Var(&thresholds.Faithfulness, "min-faithfulness", 0, "minimum faithfulness")
	flag.Float64Var(&thresholds.AnswerRelevancy, "min-answer-relevancy", 0, "minimum answer relevancy")
	flag.IntVar(&thresholds.MaxFailed, "max-failed", 0, "maximum failed cases")
	flag.Parse()
	if workers < 1 || workers > 4 || searchLimit < 1 || contextTopK < 1 || contextTopK > searchLimit {
		fatal("invalid flags: workers must be 1..4 and context-top-k must be within search-limit")
	}
	cfg, err := config.Load()
	if err != nil {
		fatal("load configuration: " + err.Error())
	}
	if !preflightOnly && !calibrateOnly && strings.TrimSpace(cfg.AIAPIKey) == "" {
		fatal("AI_API_KEY is required")
	}
	cases, err := rageval.LoadCases(dataset)
	if err != nil {
		fatal("load dataset: " + err.Error())
	}
	cases = filterCases(cases, caseIDs)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	db, err := store.Open(ctx, cfg)
	if err != nil {
		fatal("open database: " + err.Error())
	}
	defer db.Close()
	principal, err := db.ResolveKnowledgeEvaluationPrincipal(ctx, evaluationUsername)
	if err != nil {
		fatal("resolve Diving: " + err.Error())
	}
	missing, ambiguous, err := db.ValidateKnowledgeEvaluationTitles(ctx, principal, rageval.GoldTitles(cases))
	if err != nil {
		fatal("validate evaluation documents: " + err.Error())
	}
	if len(missing) > 0 {
		fatal("Diving knowledge documents missing or not ready: " + strings.Join(missing, ", "))
	}
	if len(ambiguous) > 0 {
		fatal("Diving knowledge documents are ambiguous (duplicate titles): " + strings.Join(ambiguous, ", "))
	}
	if preflightOnly {
		fmt.Printf("RAG evaluation preflight passed: user=%s cases=%d documents=%d\n", evaluationUsername, len(cases), len(rageval.GoldTitles(cases)))
		return
	}
	httpClient := &http.Client{Timeout: 180 * time.Second}
	embeddingHTTPClient := &http.Client{Timeout: 30 * time.Second}
	client := &ai.EinoClient{BaseURL: cfg.AIBaseURL, APIKey: cfg.AIAPIKey, HTTPClient: httpClient}
	workflow := ai.Workflow{Client: client, Model: cfg.AIModel}
	runnerCfg := rageval.Config{Username: evaluationUsername, Dataset: filepath.Base(dataset), SearchLimit: searchLimit, ContextTopK: contextTopK, EmbeddingModel: cfg.EmbeddingModel, RerankModel: cfg.RerankModel, Model: cfg.AIModel, JudgeModel: cfg.AIModel}
	runner := rageval.Runner{Retriever: knowledgeRetriever{store: db, principal: principal, model: cfg.EmbeddingModel,
		vectorTopK: cfg.RAGVectorTopK, titleTopK: cfg.RAGTitleTopK, keywordTopK: cfg.RAGKeywordTopK,
		embedding: ai.LocalEmbeddingClient{BaseURL: cfg.EmbeddingBaseURL, APIKey: cfg.EmbeddingAPIKey, Model: cfg.EmbeddingModel, Dimensions: cfg.EmbeddingDimensions, SendDimensions: cfg.EmbeddingSendDimensions, HTTPClient: embeddingHTTPClient}, reranker: ai.LocalRerankClient{BaseURL: cfg.RerankBaseURL, Model: cfg.RerankModel, MaxDocuments: searchLimit, HTTPClient: httpClient}}, Generator: workflow, Judge: rageval.LLMJudge{Client: client, Model: cfg.AIModel}, Config: runnerCfg, RetrievalOnly: calibrateOnly}
	results := runCases(ctx, runner, cases, workers, cfg.Environment)
	if calibrateOnly {
		calibration := rageval.CalibrateRerankThreshold(cases, results, minimumAnswerableRecall)
		encoded, marshalErr := json.MarshalIndent(calibration, "", "  ")
		if marshalErr != nil {
			fatal("encode calibration: " + marshalErr.Error())
		}
		fmt.Println(string(encoded))
		if calibration.AnswerableCases == 0 || calibration.UnanswerableCases == 0 {
			fatal("calibration dataset must contain answerable and unanswerable cases")
		}
		return
	}
	summary := rageval.Summarize(filepath.Base(dataset), results)
	dir := filepath.Join(outputRoot, time.Now().Format("20060102-150405"))
	if err := rageval.WriteArtifacts(dir, runnerCfg, results, summary); err != nil {
		fatal("write artifacts: " + err.Error())
	}
	if err := rageval.ValidateSummary(summary, thresholds); err != nil {
		fatal(err.Error() + " (artifacts: " + dir + ")")
	}
	fmt.Printf("RAG evaluation complete: user=%s total=%d succeeded=%d failed=%d output=%s\n", evaluationUsername, summary.Total, summary.Succeeded, summary.Failed, dir)
	if summary.Failed > 0 {
		os.Exit(2)
	}
}

func filterCases(cases []rageval.Case, ids string) []rageval.Case {
	if strings.TrimSpace(ids) == "" {
		return cases
	}
	wanted := map[string]bool{}
	for _, id := range strings.Split(ids, ",") {
		wanted[strings.TrimSpace(id)] = true
	}
	out := make([]rageval.Case, 0, len(wanted))
	for _, c := range cases {
		if wanted[c.ID] {
			out = append(out, c)
			delete(wanted, c.ID)
		}
	}
	if len(wanted) > 0 {
		fatal("one or more --case-ids do not exist in dataset")
	}
	return out
}
func runCases(ctx context.Context, runner rageval.Runner, cases []rageval.Case, workers int, environment string) []rageval.Result {
	type job struct {
		i int
		c rageval.Case
	}
	jobs := make(chan job)
	results := make([]rageval.Result, len(cases))
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				caseCtx := ai.WithRequestMetadata(ctx, ai.RequestMetadata{RequestID: j.c.ID, RequestType: "rag_eval", Tenant: "offline", Environment: environment})
				results[j.i] = runner.RunCase(caseCtx, j.c)
				slog.Info("rag evaluation case finished", "case_id", j.c.ID, "status", results[j.i].Status, "duration_ms", results[j.i].Latencies.TotalMS)
			}
		}()
	}
	for i, c := range cases {
		select {
		case jobs <- job{i, c}:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return results
		}
	}
	close(jobs)
	wg.Wait()
	return results
}
func fatal(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }
