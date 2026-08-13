package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"diary-listener/backend/internal/apierror"
	"diary-listener/backend/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type KnowledgeTraceConfig struct {
	EmbeddingModel string `json:"embedding_model"`
	RerankModel    string `json:"rerank_model"`
	GenerateModel  string `json:"generate_model"`
	VerifierModel  string `json:"verifier_model"`
	VectorTopK     int    `json:"vector_top_k"`
	TitleTopK      int    `json:"title_top_k"`
	KeywordTopK    int    `json:"keyword_top_k"`
	FusionTopK     int    `json:"fusion_top_k"`
	ContextTopK    int    `json:"context_top_k"`
}

type KnowledgeRequestResult struct {
	MessageID      int32
	ConversationID int32
	Content        string
	Status         string
	ErrorCode      string
	UpstreamStage  string
	OutputTokens   int
}

func (s *Store) GetKnowledgeRequest(ctx context.Context, p domain.Principal, requestID string) (KnowledgeRequestResult, bool, error) {
	if requestID == "" {
		return KnowledgeRequestResult{}, false, nil
	}
	var result KnowledgeRequestResult
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			SELECT a.id, a.conversation_id, a.content, a.status,
			       coalesce(a.error_code,''), coalesce(a.upstream_stage,''), a.output_tokens
			FROM messages u
			JOIN conversations c ON c.tenant_id=u.tenant_id AND c.id=u.conversation_id
			JOIN LATERAL (
				SELECT m.id,m.conversation_id,m.content,m.status,m.error_code,m.upstream_stage,m.output_tokens
				FROM messages m
				WHERE m.tenant_id=u.tenant_id AND m.conversation_id=u.conversation_id
				  AND m.role='assistant' AND m.id>u.id
				ORDER BY m.id LIMIT 1
			) a ON true
			WHERE u.tenant_id=$1 AND c.user_id=$2 AND c.source_scope='knowledge'
			  AND u.role='user' AND u.request_id=$3`, p.TenantID, p.UserID, requestID).Scan(
			&result.MessageID, &result.ConversationID, &result.Content, &result.Status,
			&result.ErrorCode, &result.UpstreamStage, &result.OutputTokens,
		)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return KnowledgeRequestResult{}, false, nil
	}
	return result, err == nil, err
}

func (s *Store) SaveKnowledgeAnswerOutcome(ctx context.Context, p domain.Principal, conversationID *int32, requestID, question, answer, status, errorCode, upstreamStage string, outputTokens int, sources []KnowledgeCandidate, traceConfig KnowledgeTraceConfig) (int32, int32, error) {
	var messageID, savedConversationID int32
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		if conversationID != nil {
			if err := tx.QueryRow(ctx, `SELECT id FROM conversations WHERE tenant_id=$1 AND user_id=$2 AND id=$3 AND source_scope='knowledge'`, p.TenantID, p.UserID, *conversationID).Scan(&savedConversationID); err != nil {
				return apierror.New("CONVERSATION_NOT_FOUND", "对话不存在", 404)
			}
		} else {
			title := strings.TrimSpace(question)
			r := []rune(title)
			if len(r) > 32 {
				title = string(r[:32]) + "…"
			}
			if title == "" {
				title = "知识问答"
			}
			if err := tx.QueryRow(ctx, `INSERT INTO conversations(tenant_id,user_id,title,source_scope) VALUES($1,$2,$3,'knowledge') RETURNING id`, p.TenantID, p.UserID, title).Scan(&savedConversationID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO messages(tenant_id,conversation_id,role,content,status,request_id) VALUES($1,$2,'user',$3,'complete',nullif($4,''))`, p.TenantID, savedConversationID, question, requestID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO messages(tenant_id,conversation_id,role,content,status,error_code,upstream_stage,output_tokens) VALUES($1,$2,'assistant',$3,$4,nullif($5,''),nullif($6,''),$7) RETURNING id`, p.TenantID, savedConversationID, answer, status, errorCode, upstreamStage, outputTokens).Scan(&messageID); err != nil {
			return err
		}
		for _, src := range sources {
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM knowledge_documents WHERE tenant_id=$1 AND id=$2 AND status='ready' AND deleted_at IS NULL AND active_index_version=$3)`, p.TenantID, src.DocumentID, src.IndexVersion).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return apierror.New("KNOWLEDGE_SOURCE_INVALID", "知识来源已失效", 409)
			}
			snippet := []rune(src.Content)
			if len(snippet) > 500 {
				snippet = snippet[:500]
			}
			if _, err := tx.Exec(ctx, `INSERT INTO knowledge_message_sources(tenant_id,message_id,source_type,document_id,note_id,title,snippet,index_version,rank) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, p.TenantID, messageID, src.SourceType, src.DocumentID, src.NoteID, src.Title, string(snippet), src.IndexVersion, src.Rank); err != nil {
				return err
			}
		}
		if requestID != "" {
			configJSON, err := json.Marshal(traceConfig)
			if err != nil {
				return err
			}
			sourceSnapshot := make([]map[string]any, 0, len(sources))
			for _, src := range sources {
				digest := sha256.Sum256([]byte(src.Content))
				sourceSnapshot = append(sourceSnapshot, map[string]any{"document_id": src.DocumentID, "index_version": src.IndexVersion, "rank": src.Rank, "route": src.RouteProvenance, "content_sha256": hex.EncodeToString(digest[:])})
			}
			sourcesJSON, err := json.Marshal(sourceSnapshot)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO knowledge_rag_traces(tenant_id,user_id,request_id,message_id,status,error_code,upstream_stage,output_tokens,config_snapshot,source_snapshot) VALUES($1,$2,$3,$4,$5,nullif($6,''),nullif($7,''),$8,$9,$10) ON CONFLICT (tenant_id,request_id) DO NOTHING`, p.TenantID, p.UserID, requestID, messageID, status, errorCode, upstreamStage, outputTokens, configJSON, sourcesJSON); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `UPDATE conversations SET updated_at=now() WHERE tenant_id=$1 AND id=$2`, p.TenantID, savedConversationID)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		err = apierror.New("CONVERSATION_NOT_FOUND", "对话不存在", 404)
	}
	return messageID, savedConversationID, err
}

type KnowledgeFeedback struct {
	ID        int64  `json:"id"`
	Category  string `json:"category"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

func (s *Store) CreateKnowledgeFeedback(ctx context.Context, p domain.Principal, requestID, category, comment string) (KnowledgeFeedback, error) {
	var result KnowledgeFeedback
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `INSERT INTO knowledge_rag_feedback(tenant_id,user_id,trace_id,category,comment)
			SELECT $1,$2,t.id,$4,$5 FROM knowledge_rag_traces t WHERE t.tenant_id=$1 AND t.user_id=$2 AND t.request_id=$3
			ON CONFLICT (tenant_id,user_id,trace_id) DO UPDATE SET category=excluded.category,comment=excluded.comment,updated_at=now()
			RETURNING id,category,status,created_at::text`, p.TenantID, p.UserID, requestID, category, comment).Scan(&result.ID, &result.Category, &result.Status, &result.CreatedAt)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return KnowledgeFeedback{}, apierror.New("KNOWLEDGE_TRACE_NOT_FOUND", "知识问答记录不存在", 404)
	}
	return result, err
}

type KnowledgeEvalDataset struct {
	ID             uuid.UUID `json:"id"`
	Name           string    `json:"name"`
	Version        int       `json:"version"`
	Status         string    `json:"status"`
	CaseCount      int       `json:"case_count"`
	ManifestSHA256 string    `json:"manifest_sha256,omitempty"`
}

type KnowledgeEvalPromotion struct {
	DatasetName    string
	DatasetVersion int
	CaseID         string
	Query          string
	ExpectedAnswer string
	EvidenceHashes []string
	Tags           []string
	ReviewSummary  string
}

func canonicalEvalCase(caseID, query, expected string, evidence, tags []string) ([]byte, string, error) {
	sort.Strings(evidence)
	sort.Strings(tags)
	value := struct {
		CaseID         string   `json:"case_id"`
		Query          string   `json:"query"`
		ExpectedAnswer string   `json:"expected_answer"`
		EvidenceHashes []string `json:"evidence_hashes"`
		Tags           []string `json:"tags"`
	}{caseID, query, expected, evidence, tags}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, "", err
	}
	digest := sha256.Sum256(encoded)
	return encoded, hex.EncodeToString(digest[:]), nil
}

func (s *Store) PromoteKnowledgeFeedback(ctx context.Context, p domain.Principal, feedbackID int64, input KnowledgeEvalPromotion) (KnowledgeEvalDataset, error) {
	var result KnowledgeEvalDataset
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM knowledge_rag_feedback WHERE tenant_id=$1 AND user_id=$2 AND id=$3 AND status IN ('pending','reviewed'))`, p.TenantID, p.UserID, feedbackID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return apierror.New("KNOWLEDGE_FEEDBACK_NOT_FOUND", "知识反馈不存在", 404)
		}
		if err := tx.QueryRow(ctx, `INSERT INTO knowledge_eval_datasets(tenant_id,user_id,name,version) VALUES($1,$2,$3,$4)
			ON CONFLICT (tenant_id,user_id,name,version) DO UPDATE SET name=excluded.name
			WHERE knowledge_eval_datasets.status='draft'
			RETURNING id,name,version,status,case_count,coalesce(manifest_sha256,'')`, p.TenantID, p.UserID, input.DatasetName, input.DatasetVersion).Scan(&result.ID, &result.Name, &result.Version, &result.Status, &result.CaseCount, &result.ManifestSHA256); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apierror.New("KNOWLEDGE_DATASET_FROZEN", "评测集版本已冻结", 409)
			}
			return err
		}
		evidenceJSON, _ := json.Marshal(input.EvidenceHashes)
		tagsJSON, _ := json.Marshal(input.Tags)
		_, caseHash, err := canonicalEvalCase(input.CaseID, input.Query, input.ExpectedAnswer, append([]string(nil), input.EvidenceHashes...), append([]string(nil), input.Tags...))
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO knowledge_eval_cases(tenant_id,dataset_id,feedback_id,case_id,query_text,expected_answer,evidence_hashes,tags,case_sha256) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, p.TenantID, result.ID, feedbackID, input.CaseID, input.Query, input.ExpectedAnswer, evidenceJSON, tagsJSON, caseHash); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return apierror.New("KNOWLEDGE_CASE_CONFLICT", "评测用例已存在", 409)
			}
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE knowledge_rag_feedback SET status='promoted',review_summary=$4,reviewed_at=now(),updated_at=now() WHERE tenant_id=$1 AND user_id=$2 AND id=$3`, p.TenantID, p.UserID, feedbackID, input.ReviewSummary); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `UPDATE knowledge_eval_datasets SET case_count=(SELECT count(*) FROM knowledge_eval_cases WHERE tenant_id=$1 AND dataset_id=$2) WHERE tenant_id=$1 AND id=$2 RETURNING case_count`, p.TenantID, result.ID).Scan(&result.CaseCount)
	})
	return result, err
}

func (s *Store) FreezeKnowledgeEvalDataset(ctx context.Context, p domain.Principal, datasetID uuid.UUID) (KnowledgeEvalDataset, error) {
	var result KnowledgeEvalDataset
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT id,name,version,status,case_count,coalesce(manifest_sha256,'') FROM knowledge_eval_datasets WHERE tenant_id=$1 AND user_id=$2 AND id=$3 FOR UPDATE`, p.TenantID, p.UserID, datasetID).Scan(&result.ID, &result.Name, &result.Version, &result.Status, &result.CaseCount, &result.ManifestSHA256); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apierror.New("KNOWLEDGE_DATASET_NOT_FOUND", "评测集不存在", 404)
			}
			return err
		}
		if result.Status == "frozen" {
			return nil
		}
		rows, err := tx.Query(ctx, `SELECT case_id,case_sha256 FROM knowledge_eval_cases WHERE tenant_id=$1 AND dataset_id=$2 ORDER BY case_id`, p.TenantID, datasetID)
		if err != nil {
			return err
		}
		defer rows.Close()
		manifest := strings.Builder{}
		count := 0
		for rows.Next() {
			var id, hash string
			if err := rows.Scan(&id, &hash); err != nil {
				return err
			}
			fmt.Fprintf(&manifest, "%s:%s\n", id, hash)
			count++
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if count == 0 {
			return apierror.New("KNOWLEDGE_DATASET_EMPTY", "评测集没有可冻结的用例", 409)
		}
		digest := sha256.Sum256([]byte(manifest.String()))
		result.ManifestSHA256 = hex.EncodeToString(digest[:])
		result.Status = "frozen"
		result.CaseCount = count
		_, err = tx.Exec(ctx, `UPDATE knowledge_eval_datasets SET status='frozen',manifest_sha256=$3,case_count=$4,frozen_at=now() WHERE tenant_id=$1 AND id=$2`, p.TenantID, datasetID, result.ManifestSHA256, count)
		return err
	})
	return result, err
}
