package store

import (
	"context"
	"errors"
	"strings"
	"unicode"

	"diary-listener/backend/internal/apierror"
	"diary-listener/backend/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/text/unicode/norm"
)

type KnowledgeCandidate struct {
	DocumentID                 uuid.UUID `json:"document_id"`
	ParentID                   uuid.UUID `json:"-"`
	NoteID                     *int32    `json:"note_id,omitempty"`
	SourceType, Title, Content string
	SourcePath                 string
	Heading                    []string
	IndexVersion, Rank         int
	Score                      float64
	RerankScore                *float64
	// RouteProvenance tracks which recall routes hit this candidate.
	// Bit flags: 1 = vector, 2 = fulltext, 4 = title.
	RouteProvenance int `json:"route_provenance"`
}

// ResolveKnowledgeEvaluationPrincipal resolves an offline evaluator identity by
// server-owned username. Callers never supply or select a tenant ID.
func (s *Store) ResolveKnowledgeEvaluationPrincipal(ctx context.Context, username string) (domain.Principal, error) {
	var p domain.Principal
	err := s.Pool.QueryRow(ctx, `SELECT u.id,u.username,t.id,(t.status='active' AND t.deleted_at IS NULL)
		FROM users u JOIN tenants t ON t.user_id=u.id WHERE u.username=$1`, username).
		Scan(&p.UserID, &p.Username, &p.TenantID, &p.TenantActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Principal{}, apierror.New("RAG_EVAL_USER_NOT_FOUND", "评测用户不存在", 404)
	}
	if err != nil {
		return domain.Principal{}, err
	}
	if !p.TenantActive {
		return domain.Principal{}, apierror.New("RAG_EVAL_TENANT_INACTIVE", "评测用户的个人空间不可用", 403)
	}
	return p, nil
}

func (s *Store) ValidateKnowledgeEvaluationTitles(ctx context.Context, p domain.Principal, titles []string) (missing, ambiguous []string, err error) {
	missing = make([]string, 0)
	ambiguous = make([]string, 0)
	err = s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		for _, title := range titles {
			var count int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM knowledge_documents d
				WHERE d.tenant_id=$1 AND (d.title=$2 OR split_part(regexp_replace(coalesce(d.stored_path,''),'^.*/',''),'.',1)=$2)
				AND d.status='ready' AND d.deleted_at IS NULL
				AND d.knowledge_enabled AND d.active_index_version>0
				AND EXISTS(SELECT 1 FROM knowledge_child_chunks c WHERE c.tenant_id=d.tenant_id
					AND c.document_id=d.id AND c.index_version=d.active_index_version AND c.embedding IS NOT NULL)`,
				p.TenantID, title).Scan(&count); err != nil {
				return err
			}
			if count == 0 {
				missing = append(missing, title)
			} else if count > 1 {
				ambiguous = append(ambiguous, title)
			}
		}
		return nil
	})
	return missing, ambiguous, err
}

func (s *Store) ValidateKnowledgeCollections(ctx context.Context, p domain.Principal, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM knowledge_collections WHERE tenant_id=$1 AND id=ANY($2) AND deleted_at IS NULL`, p.TenantID, ids).Scan(&count); err != nil {
			return err
		}
		if count != len(ids) {
			return apierror.New("KNOWLEDGE_SCOPE_NOT_FOUND", "知识库范围不存在", 404)
		}
		return nil
	})
}
func (s *Store) SearchKnowledge(ctx context.Context, p domain.Principal, query string, embedding []float32, model string, collections []uuid.UUID, vectorLimit, titleLimit, keywordLimit, fusionLimit int) ([]KnowledgeCandidate, error) {
	var out []KnowledgeCandidate
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `WITH eligible AS (
SELECT c.id,c.parent_id,c.document_id,c.index_version,c.embedding_text,c.embedding,d.title,d.source_type,d.note_id,d.stored_path
FROM knowledge_child_chunks c JOIN knowledge_documents d ON d.tenant_id=c.tenant_id AND d.id=c.document_id
LEFT JOIN notes n ON n.tenant_id=d.tenant_id AND n.id=d.note_id
WHERE c.tenant_id=$1 AND d.status='ready' AND d.deleted_at IS NULL AND d.knowledge_enabled
AND c.index_version=d.active_index_version AND c.embedding_model=$4 AND c.embedding IS NOT NULL
AND (coalesce(cardinality($3::uuid[]),0)=0 OR d.collection_id=ANY($3)) AND (d.source_type<>'note' OR n.deleted_at IS NULL)),
v AS (SELECT id,parent_id,row_number() OVER(ORDER BY embedding <=> $2::vector) rank FROM eligible LIMIT $6),
f AS (SELECT id,parent_id,row_number() OVER(ORDER BY ts_rank_cd(to_tsvector('simple',embedding_text),plainto_tsquery('simple',$5)) DESC) rank FROM eligible WHERE to_tsvector('simple',embedding_text) @@ plainto_tsquery('simple',$5) LIMIT $7),
title_base AS (
  SELECT DISTINCT document_id,title,
    regexp_replace(lower(regexp_replace(normalize(trim(title),NFKC),'\.md$','','i')),'[[:space:][:punct:]，。！？；：、“”‘’（）【】《》·]+','','g') normalized_title,
    regexp_replace(lower(regexp_replace(normalize(trim($5),NFKC),'\.md$','','i')),'[[:space:][:punct:]，。！？；：、“”‘’（）【】《》·]+','','g') normalized_query
  FROM eligible
),
title_docs AS (
  SELECT document_id FROM title_base
  WHERE lower(trim(title))=lower(trim($5))
     OR normalized_title=normalized_query
     OR (char_length(normalized_title)>=4 AND position(normalized_title in normalized_query)>0)
     OR (char_length(normalized_title)>=4 AND char_length(normalized_query)>=4 AND similarity(normalized_title,normalized_query)>=0.35)
  ORDER BY CASE
    WHEN lower(trim(title))=lower(trim($5)) THEN 1
    WHEN normalized_title=normalized_query THEN 2
    WHEN char_length(normalized_title)>=4 AND position(normalized_title in normalized_query)>0 THEN 3
    ELSE 4 END,
    similarity(normalized_title,normalized_query) DESC,
    char_length(normalized_title) DESC,
    document_id
  LIMIT 5
),
t AS (SELECT id,parent_id,row_number() OVER(ORDER BY embedding <=> $2::vector) rank FROM eligible WHERE document_id IN (SELECT document_id FROM title_docs) LIMIT $8),
child_score AS (
  SELECT id,parent_id,sum(value) score,sum(route) route_mask FROM (
    SELECT id,parent_id,1.0/(60+rank) value,1 route FROM v
    UNION ALL SELECT id,parent_id,1.0/(60+rank),2 FROM f
    UNION ALL SELECT id,parent_id,1.0/(60+rank),4 FROM t
  ) routes GROUP BY id,parent_id
),
score AS (SELECT parent_id,max(score) score,max(route_mask) route_mask FROM child_score GROUP BY parent_id ORDER BY score DESC LIMIT $9),
parent_meta AS (SELECT DISTINCT ON (parent_id) parent_id,document_id,note_id,source_type,title,index_version,stored_path FROM eligible ORDER BY parent_id,id)
SELECT e.document_id,score.parent_id,e.note_id,e.source_type,e.title,p.content,p.heading_path,e.index_version,score.score,coalesce(e.stored_path,''),coalesce(score.route_mask,0)
FROM score JOIN parent_meta e ON e.parent_id=score.parent_id JOIN knowledge_parent_chunks p ON p.tenant_id=$1 AND p.id=score.parent_id ORDER BY score.score DESC`, p.TenantID, vectorLiteral(embedding), collections, model, query, vectorLimit, keywordLimit, titleLimit, fusionLimit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c KnowledgeCandidate
			if err := rows.Scan(&c.DocumentID, &c.ParentID, &c.NoteID, &c.SourceType, &c.Title, &c.Content, &c.Heading, &c.IndexVersion, &c.Score, &c.SourcePath, &c.RouteProvenance); err != nil {
				return err
			}
			c.Rank = len(out) + 1
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}

// SelectKnowledgeContexts first selects the strongest documents, then fills
// the context budget with their best distinct parent sections.
func SelectKnowledgeContexts(query string, items []KnowledgeCandidate, limit int) []KnowledgeCandidate {
	if limit <= 0 || len(items) == 0 {
		return nil
	}
	docLimit := min(3, limit)
	queryKey := normalizeKnowledgeTitle(query)
	if titleKey := normalizeKnowledgeTitle(items[0].Title); titleMatchEligible(titleKey) && strings.Contains(queryKey, titleKey) {
		docLimit = 1
	}
	documents := make([]uuid.UUID, 0, docLimit)
	selectedDocs := make(map[uuid.UUID]bool, docLimit)
	seenParents := make(map[uuid.UUID]bool, len(items))
	unique := make([]KnowledgeCandidate, 0, len(items))
	for _, item := range items {
		if item.ParentID != uuid.Nil && seenParents[item.ParentID] {
			continue
		}
		seenParents[item.ParentID] = true
		unique = append(unique, item)
		if len(documents) < docLimit && !selectedDocs[item.DocumentID] {
			documents = append(documents, item.DocumentID)
			selectedDocs[item.DocumentID] = true
		}
	}
	result := make([]KnowledgeCandidate, 0, min(limit, len(unique)))
	usedParents := make(map[uuid.UUID]bool, limit)
	for _, documentID := range documents {
		for _, item := range unique {
			if item.DocumentID == documentID {
				result = append(result, item)
				usedParents[item.ParentID] = true
				break
			}
		}
	}
	for _, item := range unique {
		if len(result) >= limit {
			break
		}
		if selectedDocs[item.DocumentID] && !usedParents[item.ParentID] {
			result = append(result, item)
			usedParents[item.ParentID] = true
		}
	}
	for i := range result {
		result[i].Rank = i + 1
	}
	return result
}

func normalizeKnowledgeTitle(value string) string {
	value = strings.ToLower(norm.NFKC.String(strings.TrimSpace(value)))
	value = strings.TrimSuffix(value, ".md")
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return r
		}
		return -1
	}, value)
}

func titleMatchEligible(value string) bool {
	return len([]rune(value)) >= 4
}

func (s *Store) SaveKnowledgeAnswer(ctx context.Context, p domain.Principal, conversationID *int32, requestID, question, answer, status, errorCode, upstreamStage string, outputTokens int, sources []KnowledgeCandidate) (int32, int32, error) {
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
		_, err := tx.Exec(ctx, `UPDATE conversations SET updated_at=now() WHERE tenant_id=$1 AND id=$2`, p.TenantID, savedConversationID)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		err = apierror.New("CONVERSATION_NOT_FOUND", "对话不存在", 404)
	}
	return messageID, savedConversationID, err
}
