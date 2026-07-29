package server

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

var knowledgeRetrievalCount atomic.Uint64
var knowledgeRetrievalMilliseconds atomic.Uint64
var knowledgeIndexQueue atomic.Int64
var knowledgeIndexFailures atomic.Uint64
var knowledgeIndexProcessingMilliseconds atomic.Uint64
var researchJobsCompleted atomic.Uint64
var researchJobsFailed atomic.Uint64
var researchJobsCreated atomic.Uint64
var researchJobsCancelled atomic.Uint64
var researchSourcesCollected atomic.Uint64
var researchSourcesFailed atomic.Uint64
var researchSourcesSaved atomic.Uint64
var researchAuthorizationsStarted atomic.Uint64

func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = fmt.Fprintf(w, "cortex_knowledge_index_queue %d\n", knowledgeIndexQueue.Load())
	_, _ = fmt.Fprintf(w, "cortex_knowledge_index_failures_total %d\n", knowledgeIndexFailures.Load())
	_, _ = fmt.Fprintf(w, "cortex_knowledge_index_processing_milliseconds_total %d\n", knowledgeIndexProcessingMilliseconds.Load())
	_, _ = fmt.Fprintf(w, "cortex_knowledge_retrieval_requests_total %d\n", knowledgeRetrievalCount.Load())
	_, _ = fmt.Fprintf(w, "cortex_knowledge_retrieval_milliseconds_total %d\n", knowledgeRetrievalMilliseconds.Load())
	_, _ = fmt.Fprintf(w, "cortex_research_jobs_completed_total %d\n", researchJobsCompleted.Load())
	_, _ = fmt.Fprintf(w, "cortex_research_jobs_failed_total %d\n", researchJobsFailed.Load())
	_, _ = fmt.Fprintf(w, "cortex_research_jobs_created_total %d\n", researchJobsCreated.Load())
	_, _ = fmt.Fprintf(w, "cortex_research_jobs_cancelled_total %d\n", researchJobsCancelled.Load())
	_, _ = fmt.Fprintf(w, "cortex_research_sources_collected_total %d\n", researchSourcesCollected.Load())
	_, _ = fmt.Fprintf(w, "cortex_research_sources_failed_total %d\n", researchSourcesFailed.Load())
	_, _ = fmt.Fprintf(w, "cortex_research_sources_saved_total %d\n", researchSourcesSaved.Load())
	_, _ = fmt.Fprintf(w, "cortex_research_authorizations_started_total %d\n", researchAuthorizationsStarted.Load())
	_, _ = fmt.Fprintf(w, "cortex_research_collector_available %d\n", boolMetric(s.cfg.ResearchEnabled))
	_, _ = fmt.Fprintf(w, "cortex_research_ocr_available %d\n", boolMetric(s.cfg.ResearchOCRURL != ""))
	_, _ = fmt.Fprintf(w, "cortex_xhs_authorization_available %d\n", boolMetric(s.cfg.XHSAuthorizationEnabled))
}

func boolMetric(value bool) int {
	if value {
		return 1
	}
	return 0
}
