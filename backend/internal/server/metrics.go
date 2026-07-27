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

func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = fmt.Fprintf(w, "cortex_knowledge_index_queue %d\n", knowledgeIndexQueue.Load())
	_, _ = fmt.Fprintf(w, "cortex_knowledge_index_failures_total %d\n", knowledgeIndexFailures.Load())
	_, _ = fmt.Fprintf(w, "cortex_knowledge_index_processing_milliseconds_total %d\n", knowledgeIndexProcessingMilliseconds.Load())
	_, _ = fmt.Fprintf(w, "cortex_knowledge_retrieval_requests_total %d\n", knowledgeRetrievalCount.Load())
	_, _ = fmt.Fprintf(w, "cortex_knowledge_retrieval_milliseconds_total %d\n", knowledgeRetrievalMilliseconds.Load())
}
