package knowledge

import (
	"context"
	"time"

	"cortex/backend/internal/domain"
	knowledgecore "cortex/backend/internal/knowledge"
	"cortex/backend/internal/store"
	"github.com/google/uuid"
)

type Repository interface {
	CreateKnowledgeUpload(context.Context, domain.Principal, uuid.UUID, string, string, string, string, knowledgecore.Prepared) (store.KnowledgeUpload, error)
	GetKnowledgeUpload(context.Context, domain.Principal, uuid.UUID) (store.KnowledgeUpload, error)
	GetKnowledgeAsset(context.Context, domain.Principal, uuid.UUID, uuid.UUID) (store.KnowledgeAsset, error)
	RetryKnowledgeDocument(context.Context, domain.Principal, uuid.UUID) error
	ListKnowledgeCollections(context.Context, domain.Principal) ([]store.KnowledgeCollection, error)
	CreateKnowledgeCollection(context.Context, domain.Principal, string, string) (store.KnowledgeCollection, error)
	ListKnowledgeDocuments(context.Context, domain.Principal) ([]store.KnowledgeDocument, int64, int64, error)
	DeleteKnowledgeDocument(context.Context, domain.Principal, uuid.UUID) (string, int64, error)
	SetNoteKnowledge(context.Context, domain.Principal, int32, bool) error
	ConsumeKnowledgeClarification(context.Context, domain.Principal, uuid.UUID) (store.KnowledgeClarification, error)
	GetKnowledgeRequest(context.Context, domain.Principal, string) (store.KnowledgeRequestResult, bool, error)
	ValidateKnowledgeCollections(context.Context, domain.Principal, []uuid.UUID) error
	LoadKnowledgeConversation(context.Context, domain.Principal, int32, int) ([]store.KnowledgeConversationMessage, error)
	CreateKnowledgeClarification(context.Context, domain.Principal, *int32, string, string, []uuid.UUID, string, string, time.Duration) (store.KnowledgeClarification, error)
	SaveKnowledgeAnswerOutcome(context.Context, domain.Principal, *int32, string, string, string, string, string, string, int, []store.KnowledgeCandidate, store.KnowledgeTraceConfig) (int32, int32, error)
	CreateKnowledgeFeedback(context.Context, domain.Principal, string, string, string) (store.KnowledgeFeedback, error)
	PromoteKnowledgeFeedback(context.Context, domain.Principal, int64, store.KnowledgeEvalPromotion) (store.KnowledgeEvalDataset, error)
	FreezeKnowledgeEvalDataset(context.Context, domain.Principal, uuid.UUID) (store.KnowledgeEvalDataset, error)
	SearchKnowledge(context.Context, domain.Principal, string, []float32, string, []uuid.UUID, int, int, int, int) ([]store.KnowledgeCandidate, error)
	ValidateKnowledgeCandidateIdentities(context.Context, domain.Principal, []store.CandidateIdentity) (map[store.CandidateIdentity]bool, error)
	ClaimKnowledgeJobs(context.Context, uuid.UUID, int, time.Duration) ([]store.KnowledgeIndexJob, error)
	UpdateKnowledgeJobProgress(context.Context, store.KnowledgeIndexJob, string, int, int) error
	LoadKnowledgeJobDocument(context.Context, *store.KnowledgeIndexJob) error
	WriteKnowledgeChunks(context.Context, store.KnowledgeIndexJob, []knowledgecore.ParentChunk, [][][]float32, string) error
	FailKnowledgeJob(context.Context, store.KnowledgeIndexJob, string) error
}

type Service struct{ repository Repository }

func NewService(repository Repository) *Service { return &Service{repository: repository} }
func (s *Service) CreateUpload(c context.Context, p domain.Principal, id uuid.UUID, k, n, r, b string, v knowledgecore.Prepared) (store.KnowledgeUpload, error) {
	return s.repository.CreateKnowledgeUpload(c, p, id, k, n, r, b, v)
}
func (s *Service) Upload(c context.Context, p domain.Principal, id uuid.UUID) (store.KnowledgeUpload, error) {
	return s.repository.GetKnowledgeUpload(c, p, id)
}
func (s *Service) Asset(c context.Context, p domain.Principal, d, a uuid.UUID) (store.KnowledgeAsset, error) {
	return s.repository.GetKnowledgeAsset(c, p, d, a)
}
func (s *Service) Retry(c context.Context, p domain.Principal, id uuid.UUID) error {
	return s.repository.RetryKnowledgeDocument(c, p, id)
}
func (s *Service) Collections(c context.Context, p domain.Principal) ([]store.KnowledgeCollection, error) {
	return s.repository.ListKnowledgeCollections(c, p)
}
func (s *Service) CreateCollection(c context.Context, p domain.Principal, n, d string) (store.KnowledgeCollection, error) {
	return s.repository.CreateKnowledgeCollection(c, p, n, d)
}
func (s *Service) Documents(c context.Context, p domain.Principal) ([]store.KnowledgeDocument, int64, int64, error) {
	return s.repository.ListKnowledgeDocuments(c, p)
}
func (s *Service) DeleteDocument(c context.Context, p domain.Principal, id uuid.UUID) (string, int64, error) {
	return s.repository.DeleteKnowledgeDocument(c, p, id)
}
func (s *Service) SetNote(c context.Context, p domain.Principal, id int32, e bool) error {
	return s.repository.SetNoteKnowledge(c, p, id, e)
}
func (s *Service) ConsumeClarification(c context.Context, p domain.Principal, id uuid.UUID) (store.KnowledgeClarification, error) {
	return s.repository.ConsumeKnowledgeClarification(c, p, id)
}
func (s *Service) Request(c context.Context, p domain.Principal, id string) (store.KnowledgeRequestResult, bool, error) {
	return s.repository.GetKnowledgeRequest(c, p, id)
}
func (s *Service) ValidateCollections(c context.Context, p domain.Principal, ids []uuid.UUID) error {
	return s.repository.ValidateKnowledgeCollections(c, p, ids)
}
func (s *Service) Conversation(c context.Context, p domain.Principal, id int32, n int) ([]store.KnowledgeConversationMessage, error) {
	return s.repository.LoadKnowledgeConversation(c, p, id, n)
}
func (s *Service) CreateClarification(c context.Context, p domain.Principal, id *int32, r, q string, ids []uuid.UUID, k, prompt string, ttl time.Duration) (store.KnowledgeClarification, error) {
	return s.repository.CreateKnowledgeClarification(c, p, id, r, q, ids, k, prompt, ttl)
}
func (s *Service) SaveOutcome(c context.Context, p domain.Principal, id *int32, r, q, a, status, code, stage string, tokens int, sources []store.KnowledgeCandidate, trace store.KnowledgeTraceConfig) (int32, int32, error) {
	return s.repository.SaveKnowledgeAnswerOutcome(c, p, id, r, q, a, status, code, stage, tokens, sources, trace)
}
func (s *Service) Feedback(c context.Context, p domain.Principal, r, k, m string) (store.KnowledgeFeedback, error) {
	return s.repository.CreateKnowledgeFeedback(c, p, r, k, m)
}
func (s *Service) Promote(c context.Context, p domain.Principal, id int64, i store.KnowledgeEvalPromotion) (store.KnowledgeEvalDataset, error) {
	return s.repository.PromoteKnowledgeFeedback(c, p, id, i)
}
func (s *Service) Freeze(c context.Context, p domain.Principal, id uuid.UUID) (store.KnowledgeEvalDataset, error) {
	return s.repository.FreezeKnowledgeEvalDataset(c, p, id)
}
func (s *Service) Search(c context.Context, p domain.Principal, q string, e []float32, m string, ids []uuid.UUID, v, t, k, f int) ([]store.KnowledgeCandidate, error) {
	return s.repository.SearchKnowledge(c, p, q, e, m, ids, v, t, k, f)
}
func (s *Service) ValidateCandidates(c context.Context, p domain.Principal, ids []store.CandidateIdentity) (map[store.CandidateIdentity]bool, error) {
	return s.repository.ValidateKnowledgeCandidateIdentities(c, p, ids)
}
func (s *Service) ClaimKnowledgeJobs(c context.Context, o uuid.UUID, n int, d time.Duration) ([]store.KnowledgeIndexJob, error) {
	return s.repository.ClaimKnowledgeJobs(c, o, n, d)
}
func (s *Service) UpdateKnowledgeJobProgress(c context.Context, j store.KnowledgeIndexJob, stage string, p, t int) error {
	return s.repository.UpdateKnowledgeJobProgress(c, j, stage, p, t)
}
func (s *Service) LoadKnowledgeJobDocument(c context.Context, j *store.KnowledgeIndexJob) error {
	return s.repository.LoadKnowledgeJobDocument(c, j)
}
func (s *Service) WriteKnowledgeChunks(c context.Context, j store.KnowledgeIndexJob, p []knowledgecore.ParentChunk, v [][][]float32, m string) error {
	return s.repository.WriteKnowledgeChunks(c, j, p, v, m)
}
func (s *Service) FailKnowledgeJob(c context.Context, j store.KnowledgeIndexJob, code string) error {
	return s.repository.FailKnowledgeJob(c, j, code)
}
