package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"diary-listener/backend/internal/ai"
	"diary-listener/backend/internal/config"
	"diary-listener/backend/internal/recipe"
	"diary-listener/backend/internal/store"
)

func main() {
	var dataset string
	var outputRoot string
	var workers int
	var searchLimit int
	var contextTopK int
	var caseIDs string
	flag.StringVar(&dataset, "dataset", "testdata/rag/recipe_eval_v1.jsonl", "JSONL evaluation dataset")
	flag.StringVar(&outputRoot, "output", "../artifacts/rag-eval", "artifact output root")
	flag.IntVar(&workers, "workers", 1, "parallel case workers")
	flag.IntVar(&searchLimit, "search-limit", 10, "vector retrieval candidate count")
	flag.IntVar(&contextTopK, "context-top-k", 10, "reranked contexts sent to generation and judge")
	flag.StringVar(&caseIDs, "case-ids", "", "optional comma-separated case IDs")
	flag.Parse()
	if workers < 1 || workers > 4 || searchLimit < 1 || contextTopK < 1 || contextTopK > searchLimit {
		fatal("invalid flags: workers must be 1..4 and context-top-k must be within search-limit")
	}

	cfg, err := config.Load()
	if err != nil {
		fatal("load configuration: " + err.Error())
	}
	if strings.TrimSpace(cfg.AIAPIKey) == "" {
		fatal("AI_API_KEY is required for generation and judge evaluation")
	}
	cases, err := recipe.LoadEvalCases(dataset)
	if err != nil {
		fatal("load dataset: " + err.Error())
	}
	if strings.TrimSpace(caseIDs) != "" {
		wanted := map[string]bool{}
		for _, id := range strings.Split(caseIDs, ",") {
			wanted[strings.TrimSpace(id)] = true
		}
		filtered := cases[:0]
		for _, item := range cases {
			if wanted[item.ID] {
				filtered = append(filtered, item)
				delete(wanted, item.ID)
			}
		}
		if len(wanted) > 0 {
			fatal("one or more --case-ids do not exist in the dataset")
		}
		cases = filtered
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	db, err := store.Open(ctx, cfg)
	if err != nil {
		fatal("open database: " + err.Error())
	}
	defer db.Close()

	client := &ai.EinoClient{BaseURL: cfg.AIBaseURL, APIKey: cfg.AIAPIKey,
		HTTPClient: &http.Client{Timeout: 180 * time.Second}}
	workflow := ai.Workflow{Client: client, Model: cfg.AIModel}
	runnerConfig := recipe.EvalConfig{SearchLimit: searchLimit, ContextTopK: contextTopK,
		EmbeddingModel: cfg.EmbeddingModel, RerankModel: cfg.RerankModel,
		Model: cfg.AIModel, JudgeModel: cfg.AIModel}
	runner := recipe.EvalRunner{
		Retriever: &recipe.Retriever{Store: db, RerankURL: strings.TrimRight(cfg.RerankBaseURL, "/") + "/rerank",
			RerankModel: cfg.RerankModel, EmbeddingURL: strings.TrimRight(cfg.EmbeddingBaseURL, "/") + "/embeddings",
			EmbeddingModel: cfg.EmbeddingModel, VectorTopK: cfg.RAGVectorTopK, TitleTopK: cfg.RAGTitleTopK,
			KeywordTopK: cfg.RAGKeywordTopK, FusionTopK: cfg.RAGFusionTopK, ContextTopK: cfg.RAGContextTopK},
		Generator: workflow,
		Judge:     recipe.LLMJudge{Client: client, Model: cfg.AIModel},
		Config:    runnerConfig,
	}

	results := runCases(ctx, runner, cases, workers, cfg.Environment)
	summary := recipe.Summarize(filepath.Base(dataset), results)
	outputDir := filepath.Join(outputRoot, time.Now().Format("20060102-150405"))
	if err := recipe.WriteEvalArtifacts(outputDir, runnerConfig, results, summary); err != nil {
		fatal("write artifacts: " + err.Error())
	}
	fmt.Printf("RAG evaluation complete: total=%d succeeded=%d failed=%d output=%s\n", summary.Total, summary.Succeeded, summary.Failed, outputDir)
	if summary.Failed > 0 {
		os.Exit(2)
	}
}

func runCases(ctx context.Context, runner recipe.EvalRunner, cases []recipe.EvalCase, workers int, environment string) []recipe.EvalResult {
	type job struct {
		index int
		item  recipe.EvalCase
	}
	jobs := make(chan job)
	results := make([]recipe.EvalResult, len(cases))
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for current := range jobs {
				caseCtx := ai.WithRequestMetadata(ctx, ai.RequestMetadata{RequestID: current.item.ID,
					RequestType: "rag_eval", Tenant: "offline", Environment: environment})
				results[current.index] = runner.RunCase(caseCtx, current.item)
				slog.Info("rag evaluation case finished", "case_id", current.item.ID,
					"status", results[current.index].Status, "duration_ms", results[current.index].Latencies.TotalMS)
			}
		}()
	}
	for index, item := range cases {
		select {
		case jobs <- job{index: index, item: item}:
		case <-ctx.Done():
			close(jobs)
			wait.Wait()
			return results
		}
	}
	close(jobs)
	wait.Wait()
	return results
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
