package server

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

var researchJobsCompleted atomic.Uint64
var researchJobsFailed atomic.Uint64
var researchJobsCreated atomic.Uint64
var researchJobsCancelled atomic.Uint64
var researchSourcesCollected atomic.Uint64
var researchSourcesFailed atomic.Uint64
var researchAuthorizationsStarted atomic.Uint64
var templateCacheHits atomic.Uint64
var templateCacheMisses atomic.Uint64
var templateCacheErrors atomic.Uint64
var templatePublicViews atomic.Uint64
var templatePublicLikes atomic.Uint64
var templatePublicFavorites atomic.Uint64
var templatePublicUses atomic.Uint64
var aiEventClaimsReserved atomic.Uint64
var aiEventClaimsSoldOut atomic.Uint64
var aiEventClaimsIneligible atomic.Uint64
var aiEventClaimsDuplicate atomic.Uint64
var aiEventClaimsError atomic.Uint64
var aiEventProjectionBuildSuccess atomic.Uint64
var aiEventProjectionBuildFailed atomic.Uint64
var aiEventProjectionBuildAbandoned atomic.Uint64
var aiEventProjectionBuildDurationNanos atomic.Uint64
var aiEventProjectionVersionChanged atomic.Uint64
var aiEventProjectionBuildSkippedOpen atomic.Uint64
var aiEventClaimRedisErrors atomic.Uint64
var aiEventClaimFallbackBusy atomic.Uint64
var aiEventClaimCapacityBusy atomic.Uint64
var aiEventClaimDBTimeouts atomic.Uint64
var aiEventClaimRateNanos, aiEventClaimRateCount atomic.Uint64
var aiEventClaimAuthNanos, aiEventClaimAuthCount atomic.Uint64
var aiEventClaimRedisNanos, aiEventClaimRedisCount atomic.Uint64
var aiEventClaimDBNanos, aiEventClaimDBCount atomic.Uint64
var aiEventClaimConfirmNanos, aiEventClaimConfirmCount atomic.Uint64
var aiEventClaimTotalNanos, aiEventClaimTotalCount atomic.Uint64
var templateOutboxLeaseRenewed atomic.Uint64
var templateOutboxLeaseLost atomic.Uint64
var templateOutboxFinishFenced atomic.Uint64
var knowledgeFeedbackCreated atomic.Uint64
var knowledgeIndexLeaseLost atomic.Uint64
var knowledgeNoEvidence atomic.Uint64
var knowledgeRerankFailed atomic.Uint64
var knowledgeStreamIncomplete atomic.Uint64
var knowledgeSourceInvalid atomic.Uint64
var scheduledReportLeaseLost atomic.Uint64
var scheduledReportRunsFailed atomic.Uint64

func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = fmt.Fprintf(w, "cortex_research_jobs_completed_total %d\n", researchJobsCompleted.Load())
	_, _ = fmt.Fprintf(w, "cortex_research_jobs_failed_total %d\n", researchJobsFailed.Load())
	_, _ = fmt.Fprintf(w, "cortex_research_jobs_created_total %d\n", researchJobsCreated.Load())
	_, _ = fmt.Fprintf(w, "cortex_research_jobs_cancelled_total %d\n", researchJobsCancelled.Load())
	_, _ = fmt.Fprintf(w, "cortex_research_sources_collected_total %d\n", researchSourcesCollected.Load())
	_, _ = fmt.Fprintf(w, "cortex_research_sources_failed_total %d\n", researchSourcesFailed.Load())
	_, _ = fmt.Fprintf(w, "cortex_research_authorizations_started_total %d\n", researchAuthorizationsStarted.Load())
	_, _ = fmt.Fprintf(w, "cortex_research_collector_available %d\n", boolMetric(s.cfg.ResearchEnabled))
	_, _ = fmt.Fprintf(w, "cortex_research_ocr_available %d\n", boolMetric(s.cfg.ResearchOCRURL != ""))
	_, _ = fmt.Fprintf(w, "cortex_xhs_authorization_available %d\n", boolMetric(s.cfg.XHSAuthorizationEnabled))
	_, _ = fmt.Fprintf(w, "cortex_template_cache_requests_total{result=\"hit\"} %d\n", templateCacheHits.Load())
	_, _ = fmt.Fprintf(w, "cortex_template_cache_requests_total{result=\"miss\"} %d\n", templateCacheMisses.Load())
	_, _ = fmt.Fprintf(w, "cortex_template_cache_requests_total{result=\"error\"} %d\n", templateCacheErrors.Load())
	_, _ = fmt.Fprintf(w, "cortex_template_public_requests_total{operation=\"view\"} %d\n", templatePublicViews.Load())
	_, _ = fmt.Fprintf(w, "cortex_template_public_requests_total{operation=\"like\"} %d\n", templatePublicLikes.Load())
	_, _ = fmt.Fprintf(w, "cortex_template_public_requests_total{operation=\"favorite\"} %d\n", templatePublicFavorites.Load())
	_, _ = fmt.Fprintf(w, "cortex_template_public_requests_total{operation=\"use\"} %d\n", templatePublicUses.Load())
	_, _ = fmt.Fprintf(w, "cortex_ai_event_claim_requests_total{result=\"reserved\"} %d\n", aiEventClaimsReserved.Load())
	_, _ = fmt.Fprintf(w, "cortex_ai_event_claim_requests_total{result=\"sold_out\"} %d\n", aiEventClaimsSoldOut.Load())
	_, _ = fmt.Fprintf(w, "cortex_ai_event_claim_requests_total{result=\"ineligible\"} %d\n", aiEventClaimsIneligible.Load())
	_, _ = fmt.Fprintf(w, "cortex_ai_event_claim_requests_total{result=\"duplicate\"} %d\n", aiEventClaimsDuplicate.Load())
	_, _ = fmt.Fprintf(w, "cortex_ai_event_claim_requests_total{result=\"error\"} %d\n", aiEventClaimsError.Load())
	_, _ = fmt.Fprintf(w, "cortex_ai_event_projection_build_total{result=\"success\"} %d\n", aiEventProjectionBuildSuccess.Load())
	_, _ = fmt.Fprintf(w, "cortex_ai_event_projection_build_total{result=\"failed\"} %d\n", aiEventProjectionBuildFailed.Load())
	_, _ = fmt.Fprintf(w, "cortex_ai_event_projection_build_total{result=\"abandoned\"} %d\n", aiEventProjectionBuildAbandoned.Load())
	_, _ = fmt.Fprintf(w, "cortex_ai_event_projection_build_duration_seconds %.6f\n", float64(aiEventProjectionBuildDurationNanos.Load())/float64(time.Second))
	_, _ = fmt.Fprintf(w, "cortex_ai_event_projection_version_changed_total %d\n", aiEventProjectionVersionChanged.Load())
	_, _ = fmt.Fprintf(w, "cortex_ai_event_projection_build_skipped_total{reason=\"open_frozen\"} %d\n", aiEventProjectionBuildSkippedOpen.Load())
	_, _ = fmt.Fprintf(w, "cortex_ai_event_claim_errors_total{stage=\"redis\"} %d\n", aiEventClaimRedisErrors.Load())
	_, _ = fmt.Fprintf(w, "cortex_ai_event_claim_errors_total{stage=\"fallback_busy\"} %d\n", aiEventClaimFallbackBusy.Load())
	_, _ = fmt.Fprintf(w, "cortex_ai_event_claim_errors_total{stage=\"capacity_busy\"} %d\n", aiEventClaimCapacityBusy.Load())
	_, _ = fmt.Fprintf(w, "cortex_ai_event_claim_errors_total{stage=\"database_timeout\"} %d\n", aiEventClaimDBTimeouts.Load())
	writeAIEventStageMetric(w, "rate_limit", &aiEventClaimRateNanos, &aiEventClaimRateCount)
	writeAIEventStageMetric(w, "authentication", &aiEventClaimAuthNanos, &aiEventClaimAuthCount)
	writeAIEventStageMetric(w, "redis_reserve", &aiEventClaimRedisNanos, &aiEventClaimRedisCount)
	writeAIEventStageMetric(w, "database_finalize", &aiEventClaimDBNanos, &aiEventClaimDBCount)
	writeAIEventStageMetric(w, "redis_confirm", &aiEventClaimConfirmNanos, &aiEventClaimConfirmCount)
	writeAIEventStageMetric(w, "total", &aiEventClaimTotalNanos, &aiEventClaimTotalCount)
	_, _ = fmt.Fprintf(w, "cortex_template_outbox_lease_renew_total %d\n", templateOutboxLeaseRenewed.Load())
	_, _ = fmt.Fprintf(w, "cortex_template_outbox_lease_lost_total %d\n", templateOutboxLeaseLost.Load())
	_, _ = fmt.Fprintf(w, "cortex_template_outbox_finish_fenced_total %d\n", templateOutboxFinishFenced.Load())
	_, _ = fmt.Fprintf(w, "cortex_knowledge_feedback_created_total %d\n", knowledgeFeedbackCreated.Load())
	_, _ = fmt.Fprintf(w, "cortex_knowledge_index_lease_lost_total %d\n", knowledgeIndexLeaseLost.Load())
	_, _ = fmt.Fprintf(w, "cortex_knowledge_no_evidence_total %d\n", knowledgeNoEvidence.Load())
	_, _ = fmt.Fprintf(w, "cortex_knowledge_rerank_failed_total %d\n", knowledgeRerankFailed.Load())
	_, _ = fmt.Fprintf(w, "cortex_knowledge_stream_incomplete_total %d\n", knowledgeStreamIncomplete.Load())
	_, _ = fmt.Fprintf(w, "cortex_knowledge_source_invalid_total %d\n", knowledgeSourceInvalid.Load())
	_, _ = fmt.Fprintf(w, "cortex_scheduled_report_lease_lost_total %d\n", scheduledReportLeaseLost.Load())
	_, _ = fmt.Fprintf(w, "cortex_scheduled_report_runs_failed_total %d\n", scheduledReportRunsFailed.Load())
	ready := 0
	if err := s.store.Pool.Ping(r.Context()); err == nil {
		ready = 1
	}
	_, _ = fmt.Fprintf(w, "cortex_database_ready %d\n", ready)
	pool := s.store.Pool.Stat()
	_, _ = fmt.Fprintf(w, "cortex_database_pool_connections{state=\"acquired\"} %d\n", pool.AcquiredConns())
	_, _ = fmt.Fprintf(w, "cortex_database_pool_connections{state=\"idle\"} %d\n", pool.IdleConns())
	_, _ = fmt.Fprintf(w, "cortex_database_pool_connections{state=\"total\"} %d\n", pool.TotalConns())
	_, _ = fmt.Fprintf(w, "cortex_database_pool_acquire_total %d\n", pool.AcquireCount())
	_, _ = fmt.Fprintf(w, "cortex_database_pool_acquire_duration_seconds_total %.6f\n", pool.AcquireDuration().Seconds())
	_, _ = fmt.Fprintf(w, "cortex_database_pool_empty_acquire_total %d\n", pool.EmptyAcquireCount())
	if s.store.AuthPool != nil {
		authPool := s.store.AuthPool.Stat()
		_, _ = fmt.Fprintf(w, "cortex_auth_database_pool_connections{state=\"acquired\"} %d\n", authPool.AcquiredConns())
		_, _ = fmt.Fprintf(w, "cortex_auth_database_pool_connections{state=\"idle\"} %d\n", authPool.IdleConns())
		_, _ = fmt.Fprintf(w, "cortex_auth_database_pool_connections{state=\"total\"} %d\n", authPool.TotalConns())
		_, _ = fmt.Fprintf(w, "cortex_auth_database_pool_acquire_total %d\n", authPool.AcquireCount())
		_, _ = fmt.Fprintf(w, "cortex_auth_database_pool_acquire_duration_seconds_total %.6f\n", authPool.AcquireDuration().Seconds())
		_, _ = fmt.Fprintf(w, "cortex_auth_database_pool_empty_acquire_total %d\n", authPool.EmptyAcquireCount())
	}
	if s.store.AdminPool != nil {
		if m, err := s.store.GetOperationsMetrics(r.Context()); err == nil {
			_, _ = fmt.Fprintf(w, "cortex_knowledge_index_jobs{status=\"queued\"} %d\n", m.KnowledgeQueued)
			_, _ = fmt.Fprintf(w, "cortex_knowledge_index_jobs{status=\"running\"} %d\n", m.KnowledgeRunning)
			_, _ = fmt.Fprintf(w, "cortex_knowledge_index_jobs{status=\"failed\"} %d\n", m.KnowledgeFailed)
			_, _ = fmt.Fprintf(w, "cortex_knowledge_index_oldest_queued_seconds %.3f\n", m.KnowledgeOldestQueuedSeconds)
			_, _ = fmt.Fprintf(w, "cortex_scheduled_report_tasks_due %d\n", m.ScheduledDue)
			_, _ = fmt.Fprintf(w, "cortex_scheduled_report_runs{status=\"running\"} %d\n", m.ScheduledRunning)
			_, _ = fmt.Fprintf(w, "cortex_scheduled_report_runs{status=\"failed\"} %d\n", m.ScheduledFailed)
			_, _ = fmt.Fprintf(w, "cortex_scheduled_report_oldest_due_seconds %.3f\n", m.ScheduledOldestDueSeconds)
		}
	}
	if s.store.AdminPool != nil {
		if m, err := s.store.GetMarketplaceMetrics(r.Context()); err == nil {
			_, _ = fmt.Fprintf(w, "cortex_template_outbox_pending %d\n", m.PendingOutbox)
			_, _ = fmt.Fprintf(w, "cortex_template_outbox_lag_seconds %.3f\n", m.OutboxLagSeconds)
			_, _ = fmt.Fprintf(w, "cortex_ai_event_claims{status=\"queued\"} %d\n", m.QueuedClaims)
			_, _ = fmt.Fprintf(w, "cortex_ai_event_claims{status=\"running\"} %d\n", m.RunningClaims)
			_, _ = fmt.Fprintf(w, "cortex_ai_event_claims{status=\"failed\"} %d\n", m.FailedClaims)
			_, _ = fmt.Fprintf(w, "cortex_ai_point_accounts_drifted %d\n", m.PointAccountsDrifted)
			_, _ = fmt.Fprintf(w, "cortex_ai_event_slot_records_drifted %d\n", m.EventSlotDrifted)
			_, _ = fmt.Fprintf(w, "cortex_ai_event_succeeded_claims_invalid %d\n", m.SucceededClaimsInvalid)
		}
	}
}

func observeAIEventStage(total, count *atomic.Uint64, started time.Time) {
	total.Add(uint64(time.Since(started)))
	count.Add(1)
}
func writeAIEventStageMetric(w http.ResponseWriter, stage string, total, count *atomic.Uint64) {
	_, _ = fmt.Fprintf(w, "cortex_ai_event_claim_stage_duration_seconds_sum{stage=\"%s\"} %.6f\n", stage, float64(total.Load())/float64(time.Second))
	_, _ = fmt.Fprintf(w, "cortex_ai_event_claim_stage_duration_seconds_count{stage=\"%s\"} %d\n", stage, count.Load())
}

func boolMetric(value bool) int {
	if value {
		return 1
	}
	return 0
}
