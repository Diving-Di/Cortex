package server

import (
	"context"
	"cortex/backend/internal/apierror"
	"cortex/backend/internal/domain"
	"cortex/backend/internal/store"
	"github.com/google/uuid"
)

func (s *Server) searchKnowledge(ctx context.Context, p domain.Principal, query string, embedding []float32, collections []uuid.UUID, vectorLimit, titleLimit, keywordLimit, fusionLimit int) ([]store.KnowledgeCandidate, error) {
	if s.cfg.RAGRetrievalBackend != "elasticsearch" {
		return s.store.SearchKnowledge(ctx, p, query, embedding, s.cfg.EmbeddingModel, collections, vectorLimit, titleLimit, keywordLimit, fusionLimit)
	}
	if s.search == nil {
		return nil, apierror.New("KNOWLEDGE_RETRIEVAL_UNAVAILABLE", "知识检索暂不可用", 503)
	}
	hits, err := s.search.HybridSearch(ctx, p.TenantID, query, embedding, collections, fusionLimit)
	if err != nil {
		return nil, apierror.New("KNOWLEDGE_RETRIEVAL_UNAVAILABLE", "知识检索暂不可用", 503)
	}
	ids := make([]store.CandidateIdentity, len(hits))
	for i, h := range hits {
		ids[i] = store.CandidateIdentity{DocumentID: h.DocumentID, ParentID: h.ParentID, IndexVersion: h.IndexVersion}
	}
	valid, err := s.store.ValidateKnowledgeCandidateIdentities(ctx, p, ids)
	if err != nil {
		return nil, err
	}
	out := make([]store.KnowledgeCandidate, 0, len(hits))
	for i, h := range hits {
		if !valid[ids[i]] {
			continue
		}
		out = append(out, store.KnowledgeCandidate{DocumentID: h.DocumentID, ParentID: h.ParentID, SourceType: h.SourceType, Title: h.Title, Content: h.Content, SourcePath: h.SourcePath, Heading: h.Heading, IndexVersion: h.IndexVersion, Rank: len(out) + 1, Score: h.Score, RouteProvenance: h.RouteProvenance})
	}
	return out, nil
}
