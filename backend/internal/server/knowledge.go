package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"cortex/backend/internal/ai"
	"cortex/backend/internal/apierror"
	"cortex/backend/internal/httpx"
	"cortex/backend/internal/knowledge"
	"cortex/backend/internal/store"
	"github.com/google/uuid"
)

func (s *Server) knowledgeLimits() knowledge.Limits {
	return knowledge.Limits{MaxUploadBytes: s.cfg.KnowledgeMaxUploadBytes, MaxExtractedBytes: s.cfg.KnowledgeMaxExtractedBytes, MaxFileBytes: s.cfg.KnowledgeMaxFileBytes, MaxFiles: s.cfg.KnowledgeMaxFiles, MaxDepth: s.cfg.KnowledgeMaxDepth, MaxCompressionRatio: s.cfg.KnowledgeMaxCompressionRatio}
}

func (s *Server) uploadKnowledge(w http.ResponseWriter, r *http.Request) {
	p := principalFrom(r.Context())
	if err := r.ParseMultipartForm(s.cfg.KnowledgeMaxUploadBytes); err != nil {
		httpx.WriteError(w, s.logger, apierror.New("KNOWLEDGE_QUOTA_EXCEEDED", "上传文件过大", 413))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.WriteError(w, s.logger, apierror.Validation(nil))
		return
	}
	defer file.Close()
	uploadID := uuid.New()
	tempRoot := filepath.Join(s.cfg.DataDir, "tmp", uploadID.String())
	sourceRoot := filepath.Join(tempRoot, "source")
	if err := os.MkdirAll(tempRoot, 0o750); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	defer os.RemoveAll(tempRoot)
	tempFile := filepath.Join(tempRoot, "upload.bin")
	dst, err := os.OpenFile(tempFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	_, copyErr := io.Copy(dst, io.LimitReader(file, s.cfg.KnowledgeMaxUploadBytes+1))
	closeErr := dst.Close()
	if copyErr != nil || closeErr != nil {
		httpx.WriteError(w, s.logger, apierror.New("KNOWLEDGE_ARCHIVE_INVALID", "读取上传失败", 400))
		return
	}
	prepared, err := knowledge.Prepare(tempFile, header.Filename, sourceRoot, s.knowledgeLimits())
	if err != nil {
		code := knowledge.ErrorCode(err)
		status := http.StatusBadRequest
		if code == "KNOWLEDGE_QUOTA_EXCEEDED" {
			status = http.StatusConflict
		}
		httpx.WriteError(w, s.logger, apierror.New(code, "知识库文件校验失败", status))
		return
	}
	finalRel := filepath.ToSlash(filepath.Join("knowledge", p.TenantID.String(), uploadID.String(), "source"))
	finalAbs, err := safeDataPath(s.cfg.DataDir, finalRel)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(finalAbs), 0o750); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if err := os.Rename(sourceRoot, finalAbs); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	created, err := s.store.CreateKnowledgeUpload(r.Context(), p, uploadID, strings.TrimSpace(r.Header.Get("Idempotency-Key")), header.Filename, finalRel, prepared)
	if err != nil {
		_ = os.RemoveAll(filepath.Dir(finalAbs))
		httpx.WriteError(w, s.logger, err)
		return
	}
	if created.ID != uploadID {
		_ = os.RemoveAll(filepath.Dir(finalAbs))
	}
	httpx.JSON(w, http.StatusAccepted, created)
}

func (s *Server) getKnowledgeUpload(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("uploadID"))
	if err != nil {
		httpx.WriteError(w, s.logger, apierror.New("KNOWLEDGE_SCOPE_NOT_FOUND", "知识库资源不存在", 404))
		return
	}
	value, err := s.store.GetKnowledgeUpload(r.Context(), principalFrom(r.Context()), id)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, 200, value)
}
func (s *Server) downloadKnowledgeAsset(w http.ResponseWriter, r *http.Request) {
	documentID, e1 := uuid.Parse(r.PathValue("documentID"))
	assetID, e2 := uuid.Parse(r.PathValue("assetID"))
	if e1 != nil || e2 != nil {
		httpx.WriteError(w, s.logger, apierror.New("KNOWLEDGE_SCOPE_NOT_FOUND", "知识库资源不存在", 404))
		return
	}
	asset, err := s.store.GetKnowledgeAsset(r.Context(), principalFrom(r.Context()), documentID, assetID)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	filename, err := safeDataPath(s.cfg.DataDir, asset.StoredPath)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	w.Header().Set("Content-Type", asset.MIME)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=300")
	http.ServeFile(w, r, filename)
}
func (s *Server) retryKnowledgeDocument(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("documentID"))
	if err != nil {
		httpx.WriteError(w, s.logger, apierror.New("KNOWLEDGE_SCOPE_NOT_FOUND", "知识库资源不存在", 404))
		return
	}
	if err := s.store.RetryKnowledgeDocument(r.Context(), principalFrom(r.Context()), id); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusAccepted, map[string]string{"status": "queued"})
}
func (s *Server) listKnowledgeCollections(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListKnowledgeCollections(r.Context(), principalFrom(r.Context()))
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, 200, map[string]any{"items": items})
}
func (s *Server) createKnowledgeCollection(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len([]rune(req.Name)) > 120 {
		httpx.WriteError(w, s.logger, apierror.Validation(nil))
		return
	}
	item, err := s.store.CreateKnowledgeCollection(r.Context(), principalFrom(r.Context()), req.Name, req.Description)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, 201, item)
}
func (s *Server) listKnowledgeDocuments(w http.ResponseWriter, r *http.Request) {
	items, used, reserved, err := s.store.ListKnowledgeDocuments(r.Context(), principalFrom(r.Context()))
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, 200, map[string]any{"items": items, "quota": map[string]int64{"limit_bytes": 3221225472, "used_bytes": used, "reserved_bytes": reserved, "remaining_bytes": 3221225472 - used - reserved}})
}
func (s *Server) deleteKnowledgeDocument(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("documentID"))
	if err != nil {
		httpx.WriteError(w, s.logger, apierror.New("KNOWLEDGE_SCOPE_NOT_FOUND", "知识库资源不存在", 404))
		return
	}
	stored, size, err := s.store.DeleteKnowledgeDocument(r.Context(), principalFrom(r.Context()), id)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	if stored != "" {
		abs, pathErr := safeDataPath(s.cfg.DataDir, stored)
		if pathErr != nil {
			httpx.WriteError(w, s.logger, pathErr)
			return
		}
		if removeErr := os.Remove(abs); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			httpx.WriteError(w, s.logger, apierror.New("KNOWLEDGE_DELETE_PENDING", "文件删除等待重试", 202))
			return
		}
	}
	if err := s.store.FinalizeKnowledgeDeletion(r.Context(), principalFrom(r.Context()), id, size); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) setNoteKnowledge(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	parsedNoteID, err := strconv.ParseInt(r.PathValue("noteID"), 10, 32)
	if err != nil {
		httpx.WriteError(w, s.logger, apierror.Validation(nil))
		return
	}
	noteID := int32(parsedNoteID)
	if err := s.store.SetNoteKnowledge(r.Context(), principalFrom(r.Context()), noteID, req.Enabled); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, 200, map[string]bool{"knowledge_enabled": req.Enabled})
}

func (s *Server) knowledgeChat(w http.ResponseWriter, r *http.Request) {
	requestStarted := time.Now()
	progress := make([]retrievalProgress, 0, 5)
	var req struct {
		Question              string      `json:"question"`
		RequestID             string      `json:"request_id"`
		ConversationID        *int32      `json:"conversation_id"`
		CollectionIDs         []uuid.UUID `json:"collection_ids"`
		ResumeClarificationID string      `json:"resume_clarification_id"`
		Clarification         string      `json:"clarification"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	p := principalFrom(r.Context())
	if req.ResumeClarificationID != "" {
		clarificationID, parseErr := uuid.Parse(req.ResumeClarificationID)
		req.Clarification = strings.TrimSpace(req.Clarification)
		if parseErr != nil || len([]rune(req.Clarification)) == 0 || len([]rune(req.Clarification)) > 1000 {
			httpx.WriteError(w, s.logger, apierror.Validation(nil))
			return
		}
		pending, consumeErr := s.store.ConsumeKnowledgeClarification(r.Context(), p, clarificationID)
		if consumeErr != nil {
			httpx.WriteError(w, s.logger, consumeErr)
			return
		}
		req.Question = pending.OriginalQuestion + "\n用户补充：" + req.Clarification
		req.ConversationID = &pending.ConversationID
		req.CollectionIDs = pending.CollectionIDs
		req.RequestID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(pending.OriginalRequestID)).String() + ":resume"
		if pending.AlreadyResumed {
			previous, found, replayErr := s.store.GetKnowledgeRequest(r.Context(), p, req.RequestID)
			if replayErr != nil {
				httpx.WriteError(w, s.logger, replayErr)
				return
			}
			if found {
				s.writeKnowledgeReplay(w, previous)
				return
			}
			httpx.WriteError(w, s.logger, apierror.New("KNOWLEDGE_CLARIFICATION_IN_PROGRESS", "澄清请求正在处理", http.StatusConflict))
			return
		}
	}
	req.Question = strings.TrimSpace(req.Question)
	if req.RequestID == "" {
		req.RequestID = uuid.NewString()
	}
	if len([]rune(req.Question)) == 0 || len([]rune(req.Question)) > 5000 || (req.RequestID != "" && !requestIDPattern.MatchString(req.RequestID)) {
		httpx.WriteError(w, s.logger, apierror.Validation(nil))
		return
	}
	if previous, found, err := s.store.GetKnowledgeRequest(r.Context(), p, req.RequestID); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	} else if found {
		s.writeKnowledgeReplay(w, previous)
		return
	}
	if req.ResumeClarificationID == "" {
		resumeRequestID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(req.RequestID)).String() + ":resume"
		if previous, found, err := s.store.GetKnowledgeRequest(r.Context(), p, resumeRequestID); err != nil {
			httpx.WriteError(w, s.logger, err)
			return
		} else if found {
			s.writeKnowledgeReplay(w, previous)
			return
		}
	}
	if err := s.store.ValidateKnowledgeCollections(r.Context(), p, req.CollectionIDs); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	conversationContext := ""
	if req.ConversationID != nil {
		history, err := s.store.LoadKnowledgeConversation(r.Context(), p, *req.ConversationID, 5)
		if err != nil {
			httpx.WriteError(w, s.logger, err)
			return
		}
		conversationContext = formatKnowledgeConversation(history, 8000)
	}
	workflow := s.aiWorkflow()
	rewrite, err := workflow.RewriteKnowledgeQuery(s.aiContext(r.Context(), "knowledge_query_rewrite", p), req.Question, conversationContext)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	progress = append(progress, newRetrievalProgress("rewrite", time.Since(requestStarted), func(v *retrievalProgress) {
		v.Rewritten = rewrite.Query != req.Question
	}))
	retrievalQuery := rewrite.Query
	retrievalQueries, planned := planKnowledgeQueries(retrievalQuery, s.cfg.RAGPlannerEnabled, s.cfg.RAGPlannerMaxSubqueries)
	embeddingStarted := time.Now()
	embeddingClient := ai.LocalEmbeddingClient{BaseURL: s.cfg.EmbeddingBaseURL, APIKey: s.cfg.EmbeddingAPIKey, Model: s.cfg.EmbeddingModel, Dimensions: s.cfg.EmbeddingDimensions, SendDimensions: s.cfg.EmbeddingSendDimensions}
	vectors, err := embeddingClient.Embed(r.Context(), retrievalQueries)
	if err != nil {
		httpx.WriteError(w, s.logger, apierror.New("KNOWLEDGE_EMBEDDING_UNAVAILABLE", "知识检索服务暂时不可用", 503))
		return
	}
	progress = append(progress, newRetrievalProgress("embedding", time.Since(embeddingStarted), nil))
	retrievalStarted := time.Now()
	retrievalCtx, cancelRetrieval := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancelRetrieval()
	var candidateMu sync.Mutex
	candidateByParent := make(map[uuid.UUID]store.KnowledgeCandidate)
	var retrievalErr error
	var retrievalWG sync.WaitGroup
	for i, query := range retrievalQueries {
		i, query := i, query
		retrievalWG.Add(1)
		go func() {
			defer retrievalWG.Done()
			items, searchErr := s.store.SearchKnowledge(retrievalCtx, p, query, vectors[i], s.cfg.EmbeddingModel, req.CollectionIDs,
				max(1, s.cfg.RAGVectorTopK/len(retrievalQueries)), max(1, s.cfg.RAGTitleTopK/len(retrievalQueries)), max(1, s.cfg.RAGKeywordTopK/len(retrievalQueries)), max(1, s.cfg.RAGFusionTopK/len(retrievalQueries)))
			candidateMu.Lock()
			defer candidateMu.Unlock()
			if searchErr != nil {
				if retrievalErr == nil {
					retrievalErr = searchErr
				}
				return
			}
			for _, item := range items {
				if current, found := candidateByParent[item.ParentID]; !found || item.Score > current.Score {
					candidateByParent[item.ParentID] = item
				}
			}
		}()
	}
	retrievalWG.Wait()
	if retrievalErr != nil && len(candidateByParent) == 0 {
		httpx.WriteError(w, s.logger, retrievalErr)
		return
	}
	candidates := make([]store.KnowledgeCandidate, 0, len(candidateByParent))
	for _, item := range candidateByParent {
		candidates = append(candidates, item)
	}
	sortKnowledgeCandidates(candidates)
	if len(candidates) > s.cfg.RAGFusionTopK {
		candidates = candidates[:s.cfg.RAGFusionTopK]
	}
	progress = append(progress, newRetrievalProgress("retrieval", time.Since(retrievalStarted), func(v *retrievalProgress) {
		v.CandidateCount = len(candidates)
		v.Planned = planned
		v.SubqueryCount = len(retrievalQueries)
	}))
	if len(candidates) == 0 {
		knowledgeNoEvidence.Add(1)
		httpx.WriteError(w, s.logger, apierror.New("KNOWLEDGE_NO_EVIDENCE", "没有找到当前知识库中的有效依据", 422))
		return
	}
	documents := make([]string, len(candidates))
	for i := range candidates {
		documents[i] = ai.FormatRerankDocument(candidates[i].Title, candidates[i].SourceType, candidates[i].Heading, candidates[i].Content)
	}
	reranker := ai.LocalRerankClient{BaseURL: s.cfg.RerankBaseURL, Model: s.cfg.RerankModel, MaxDocuments: s.cfg.RAGFusionTopK}
	rerankStarted := time.Now()
	scores, err := reranker.Rerank(r.Context(), retrievalQuery, documents)
	if err != nil {
		knowledgeRerankFailed.Add(1)
		httpx.WriteError(w, s.logger, apierror.New("KNOWLEDGE_RERANK_UNAVAILABLE", "知识精排服务暂时不可用", 503))
		return
	}
	for i := range candidates {
		candidates[i].Score = scores[i]
	}
	sortKnowledgeCandidates(candidates)
	gateScores := make([]float64, len(candidates))
	for i := range candidates {
		gateScores[i] = candidates[i].Score
	}
	gate := knowledge.EvaluateEvidenceGate(gateScores, s.cfg.RAGRerankMinScore, s.cfg.RAGRerankMinMargin, s.cfg.RAGMinQualifiedEvidence)
	qualified := make([]store.KnowledgeCandidate, 0, len(gate.QualifiedIndexes))
	for _, index := range gate.QualifiedIndexes {
		qualified = append(qualified, candidates[index])
	}
	candidates = qualified
	progress = append(progress, newRetrievalProgress("rerank", time.Since(rerankStarted), func(v *retrievalProgress) {
		v.CandidateCount = len(documents)
		v.QualifiedCount = len(candidates)
	}))
	if !gate.Passed {
		knowledgeNoEvidence.Add(1)
		decision := decideWeakKnowledgeEvidence(req.Question, gate.MarginConflict)
		if decision != knowledgeDecisionAbsent && req.ResumeClarificationID == "" {
			pending, stateErr := s.store.CreateKnowledgeClarification(r.Context(), p, req.ConversationID, req.RequestID, req.Question, req.CollectionIDs, string(decision), clarificationPrompt(decision), 15*time.Minute)
			if stateErr != nil {
				httpx.WriteError(w, s.logger, stateErr)
				return
			}
			httpx.WriteError(w, s.logger, &apierror.Error{Code: "KNOWLEDGE_CLARIFICATION_REQUIRED", Message: pending.Prompt, StatusCode: http.StatusConflict, Details: pending})
			return
		}
		httpx.WriteError(w, s.logger, apierror.New("KNOWLEDGE_NO_EVIDENCE", "没有找到当前知识库中的有效依据", 422))
		return
	}
	candidates = store.SelectKnowledgeContexts(retrievalQuery, candidates, s.cfg.RAGContextTopK)
	progress = append(progress, newRetrievalProgress("evidence_gate", time.Since(requestStarted), func(v *retrievalProgress) {
		v.QualifiedCount = len(candidates)
		v.SourceCount = len(candidates)
	}))
	evidence := make([]ai.KnowledgeEvidence, len(candidates))
	sources := make([]map[string]any, len(candidates))
	for i := range candidates {
		candidates[i].Rank = i + 1
		citation := fmt.Sprintf("K%d", i+1)
		evidence[i] = ai.KnowledgeEvidence{Citation: citation, Title: candidates[i].Title, Kind: candidates[i].SourceType, Content: candidates[i].Content, Heading: strings.Join(candidates[i].Heading, " / ")}
		sources[i] = map[string]any{"citation": citation, "document_id": candidates[i].DocumentID, "note_id": candidates[i].NoteID, "source_type": candidates[i].SourceType, "title": candidates[i].Title, "heading": candidates[i].Heading, "rank": i + 1}
	}
	workflow.VerifierModel = s.cfg.RAGVerifierModel
	events, err := workflow.AnswerKnowledgeGrounded(s.aiContext(r.Context(), "knowledge_chat", p), ai.KnowledgeInput{Question: req.Question, ConversationContext: conversationContext, Evidence: evidence})
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	s.writeKnowledgeSSE(w, r, events, sources, progress, func(ctx context.Context, answer, status, errorCode, upstreamStage string, outputTokens int) (int32, int32, error) {
		traceConfig := store.KnowledgeTraceConfig{EmbeddingModel: s.cfg.EmbeddingModel, RerankModel: s.cfg.RerankModel, GenerateModel: s.cfg.AIModel, VerifierModel: s.cfg.RAGVerifierModel, VectorTopK: s.cfg.RAGVectorTopK, TitleTopK: s.cfg.RAGTitleTopK, KeywordTopK: s.cfg.RAGKeywordTopK, FusionTopK: s.cfg.RAGFusionTopK, ContextTopK: s.cfg.RAGContextTopK}
		return s.store.SaveKnowledgeAnswerOutcome(ctx, p, req.ConversationID, req.RequestID, req.Question, answer, status, errorCode, upstreamStage, outputTokens, candidates, traceConfig)
	})
}

type retrievalProgress struct {
	SchemaVersion  int    `json:"schema_version"`
	Stage          string `json:"stage"`
	Status         string `json:"status"`
	ElapsedMS      int64  `json:"elapsed_ms"`
	CandidateCount int    `json:"candidate_count,omitempty"`
	QualifiedCount int    `json:"qualified_count,omitempty"`
	SourceCount    int    `json:"source_count,omitempty"`
	Rewritten      bool   `json:"rewritten,omitempty"`
	Planned        bool   `json:"planned,omitempty"`
	SubqueryCount  int    `json:"subquery_count,omitempty"`
}

func newRetrievalProgress(stage string, elapsed time.Duration, enrich func(*retrievalProgress)) retrievalProgress {
	value := retrievalProgress{SchemaVersion: 1, Stage: stage, Status: "completed", ElapsedMS: elapsed.Milliseconds()}
	if enrich != nil {
		enrich(&value)
	}
	return value
}

func (s *Server) createKnowledgeFeedback(w http.ResponseWriter, r *http.Request) {
	requestID := strings.TrimSpace(r.PathValue("requestID"))
	var req struct {
		Category string `json:"category"`
		Comment  string `json:"comment"`
	}
	if !requestIDPattern.MatchString(requestID) || httpx.DecodeJSON(r, &req) != nil {
		httpx.WriteError(w, s.logger, apierror.Validation(nil))
		return
	}
	allowed := map[string]bool{"incorrect_answer": true, "unsupported_citation": true, "missing_knowledge": true, "should_have_refused": true, "high_latency": true}
	req.Category, req.Comment = strings.TrimSpace(req.Category), strings.TrimSpace(req.Comment)
	if !allowed[req.Category] || len([]rune(req.Comment)) > 1000 {
		httpx.WriteError(w, s.logger, apierror.Validation(nil))
		return
	}
	item, err := s.store.CreateKnowledgeFeedback(r.Context(), principalFrom(r.Context()), requestID, req.Category, req.Comment)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	knowledgeFeedbackCreated.Add(1)
	httpx.JSON(w, http.StatusCreated, item)
}

func (s *Server) promoteKnowledgeFeedback(w http.ResponseWriter, r *http.Request) {
	feedbackID, err := strconv.ParseInt(r.PathValue("feedbackID"), 10, 64)
	if err != nil || feedbackID <= 0 {
		httpx.WriteError(w, s.logger, apierror.Validation(nil))
		return
	}
	var req struct {
		DatasetName    string   `json:"dataset_name"`
		DatasetVersion int      `json:"dataset_version"`
		CaseID         string   `json:"case_id"`
		Query          string   `json:"query"`
		ExpectedAnswer string   `json:"expected_answer"`
		EvidenceHashes []string `json:"evidence_hashes"`
		Tags           []string `json:"tags"`
		ReviewSummary  string   `json:"review_summary"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	req.DatasetName, req.CaseID, req.Query = strings.TrimSpace(req.DatasetName), strings.TrimSpace(req.CaseID), strings.TrimSpace(req.Query)
	req.ExpectedAnswer, req.ReviewSummary = strings.TrimSpace(req.ExpectedAnswer), strings.TrimSpace(req.ReviewSummary)
	if req.DatasetName == "" || len([]rune(req.DatasetName)) > 120 || req.DatasetVersion < 1 || !requestIDPattern.MatchString(req.CaseID) || req.Query == "" || len([]rune(req.Query)) > 2000 || req.ExpectedAnswer == "" || len([]rune(req.ExpectedAnswer)) > 4000 || len(req.EvidenceHashes) == 0 || len(req.EvidenceHashes) > 20 || len(req.Tags) > 20 || len([]rune(req.ReviewSummary)) > 1000 {
		httpx.WriteError(w, s.logger, apierror.Validation(nil))
		return
	}
	for i, hash := range req.EvidenceHashes {
		req.EvidenceHashes[i] = strings.ToLower(strings.TrimSpace(hash))
		if !sha256Pattern.MatchString(req.EvidenceHashes[i]) {
			httpx.WriteError(w, s.logger, apierror.Validation(nil))
			return
		}
	}
	for i, tag := range req.Tags {
		req.Tags[i] = strings.TrimSpace(tag)
		if req.Tags[i] == "" || len([]rune(req.Tags[i])) > 60 {
			httpx.WriteError(w, s.logger, apierror.Validation(nil))
			return
		}
	}
	item, err := s.store.PromoteKnowledgeFeedback(r.Context(), principalFrom(r.Context()), feedbackID, store.KnowledgeEvalPromotion{DatasetName: req.DatasetName, DatasetVersion: req.DatasetVersion, CaseID: req.CaseID, Query: req.Query, ExpectedAnswer: req.ExpectedAnswer, EvidenceHashes: req.EvidenceHashes, Tags: req.Tags, ReviewSummary: req.ReviewSummary})
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, item)
}

func (s *Server) freezeKnowledgeEvalDataset(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("datasetID"))
	if err != nil {
		httpx.WriteError(w, s.logger, apierror.New("KNOWLEDGE_DATASET_NOT_FOUND", "评测集不存在", 404))
		return
	}
	item, err := s.store.FreezeKnowledgeEvalDataset(r.Context(), principalFrom(r.Context()), id)
	if err != nil {
		httpx.WriteError(w, s.logger, err)
		return
	}
	httpx.JSON(w, http.StatusOK, item)
}

func formatKnowledgeConversation(messages []store.KnowledgeConversationMessage, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	turns := make([]string, 0, len(messages)/2)
	used := 0
	for i := len(messages) - 2; i >= 0; i -= 2 {
		if messages[i].Role != "user" || messages[i+1].Role != "assistant" {
			continue
		}
		turn := "用户：" + strings.TrimSpace(messages[i].Content) + "\n助手：" + strings.TrimSpace(messages[i+1].Content)
		length := len([]rune(turn))
		if used+length > maxRunes {
			continue
		}
		turns = append(turns, turn)
		used += length
	}
	for left, right := 0, len(turns)-1; left < right; left, right = left+1, right-1 {
		turns[left], turns[right] = turns[right], turns[left]
	}
	return strings.Join(turns, "\n")
}
func (s *Server) writeKnowledgeReplay(w http.ResponseWriter, result store.KnowledgeRequestResult) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	if result.Content != "" {
		_ = writeNamedSSE(w, "delta", map[string]string{"content": result.Content})
	}
	if result.Status == "complete" {
		_ = writeNamedSSE(w, "done", map[string]any{"message_id": result.MessageID, "conversation_id": result.ConversationID, "replayed": true})
	} else {
		_ = writeNamedSSE(w, "error", map[string]any{"code": result.ErrorCode, "message": "生成服务暂时不可用", "incomplete": true, "output_tokens": result.OutputTokens, "upstream_stage": result.UpstreamStage, "message_id": result.MessageID, "conversation_id": result.ConversationID, "replayed": true})
	}
	flusher.Flush()
}
func sortKnowledgeCandidates(items []store.KnowledgeCandidate) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j].Score > items[j-1].Score; j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}
func (s *Server) writeKnowledgeSSE(w http.ResponseWriter, r *http.Request, events <-chan ai.StreamEvent, sources []map[string]any, progress []retrievalProgress, save func(context.Context, string, string, string, string, int) (int32, int32, error)) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	for _, item := range progress {
		_ = writeNamedSSE(w, "retrieval_progress", item)
	}
	_ = writeNamedSSE(w, "retrieval", map[string]any{"count": len(sources), "items": sources})
	flusher.Flush()
	var answer strings.Builder
	for event := range events {
		if event.Err != nil {
			knowledgeStreamIncomplete.Add(1)
			outputTokens := len([]rune(answer.String())) / 4
			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 10*time.Second)
			messageID, conversationID, saveErr := save(ctx, answer.String(), "failed", "AI_REQUEST_FAILED", "generation", outputTokens)
			cancel()
			payload := map[string]any{"code": "AI_REQUEST_FAILED", "message": "生成服务暂时不可用", "incomplete": answer.Len() > 0, "output_tokens": outputTokens, "upstream_stage": "generation"}
			if saveErr == nil {
				payload["message_id"] = messageID
				payload["conversation_id"] = conversationID
			} else {
				s.logger.Error("save incomplete knowledge answer", "error", saveErr)
			}
			_ = writeNamedSSE(w, "error", payload)
			flusher.Flush()
			return
		}
		if event.Type == "verifying" {
			_ = writeNamedSSE(w, "verifying", map[string]string{"status": "verifying"})
			flusher.Flush()
			continue
		}
		if event.Type == "rejected" {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 10*time.Second)
			messageID, conversationID, saveErr := save(ctx, "", "failed", "KNOWLEDGE_NO_EVIDENCE", "verification", 0)
			cancel()
			payload := map[string]any{"code": "KNOWLEDGE_NO_EVIDENCE", "message": "生成内容未通过证据核验"}
			if saveErr == nil {
				payload["message_id"] = messageID
				payload["conversation_id"] = conversationID
			} else {
				s.logger.Error("save rejected knowledge answer", "error", saveErr)
			}
			_ = writeNamedSSE(w, "rejected", payload)
			flusher.Flush()
			return
		}
		answer.WriteString(event.Content)
		eventName := "delta"
		if event.Type == "verified" {
			eventName = "verified"
		}
		if writeNamedSSE(w, eventName, map[string]string{"content": event.Content}) != nil {
			return
		}
		flusher.Flush()
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 10*time.Second)
	defer cancel()
	messageID, conversationID, err := save(ctx, answer.String(), "complete", "", "completed", len([]rune(answer.String()))/4)
	if err != nil {
		var appErr *apierror.Error
		if errors.As(err, &appErr) && appErr.Code == "KNOWLEDGE_SOURCE_INVALID" {
			knowledgeSourceInvalid.Add(1)
			_ = writeNamedSSE(w, "error", map[string]string{"code": "KNOWLEDGE_SOURCE_INVALID", "message": "来源已失效，请重新提问"})
		} else {
			_ = writeNamedSSE(w, "error", map[string]string{"code": "KNOWLEDGE_SAVE_FAILED", "message": "回答保存失败，请重新提问"})
		}
	} else {
		_ = writeNamedSSE(w, "sources", map[string]any{"items": sources})
		_ = writeNamedSSE(w, "done", map[string]any{"message_id": messageID, "conversation_id": conversationID})
	}
	flusher.Flush()
}
func safeDataPath(root, relative string) (string, error) {
	if filepath.IsAbs(relative) {
		return "", apierror.New("KNOWLEDGE_ARCHIVE_UNSAFE", "非法文件路径", 400)
	}
	clean := filepath.Clean(filepath.FromSlash(relative))
	if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", apierror.New("KNOWLEDGE_ARCHIVE_UNSAFE", "非法文件路径", 400)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	target, err := filepath.Abs(filepath.Join(absRoot, clean))
	if err != nil {
		return "", err
	}
	if target == absRoot || !strings.HasPrefix(target, absRoot+string(os.PathSeparator)) {
		return "", apierror.New("KNOWLEDGE_ARCHIVE_UNSAFE", "非法文件路径", 400)
	}
	return target, nil
}

func writeNamedSSE(w http.ResponseWriter, event string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, payload)
	return err
}
