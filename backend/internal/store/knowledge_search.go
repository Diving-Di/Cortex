package store

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"diary-listener/backend/internal/apierror"
	"diary-listener/backend/internal/domain"
	"diary-listener/backend/internal/knowledge"
	"github.com/jackc/pgx/v5"
)

type KnowledgeCandidate struct {
	ChildID      int64
	ParentID     int64
	DocumentID   int64
	Document     string
	Child        string
	Parent       string
	HeadingPath  string
	PageFrom     *int
	PageTo       *int
	IndexVersion int
	Score        float64
}

func (s *Store) SearchKnowledge(
	ctx context.Context,
	principal domain.Principal,
	query string,
	queryVector []float32,
	embeddingModel string,
	collectionIDs []int64,
	documentIDs []int64,
	limit int,
) ([]KnowledgeCandidate, error) {
	var lexical, semantic []KnowledgeCandidate
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		if err := validateKnowledgeScope(ctx, tx, principal, collectionIDs, documentIDs); err != nil {
			return err
		}
		scopeSQL, scopeArgs := knowledgeScopeSQL(collectionIDs, documentIDs, 3)
		lexicalQuery := knowledgeLexicalQuery(query)
		args := []any{principal.TenantID, lexicalQuery}
		args = append(args, scopeArgs...)
		args = append(args, max(30, limit*4))
		rows, err := tx.Query(ctx, `SELECT c.id,c.parent_id,c.document_id,d.original_name,
				c.content,p.content,c.heading_path,c.page_from,c.page_to,c.index_version,
				ts_rank_cd(c.search_vector,to_tsquery('simple',$2)) score
			FROM knowledge_child_chunks c
			JOIN knowledge_parent_chunks p ON p.tenant_id=c.tenant_id AND p.id=c.parent_id
			JOIN knowledge_documents d ON d.tenant_id=c.tenant_id AND d.id=c.document_id
			WHERE c.tenant_id=$1 AND d.status='ready' AND d.deleted_at IS NULL
			  AND c.index_version=d.index_version
			  AND c.search_vector @@ to_tsquery('simple',$2)`+scopeSQL+
			fmt.Sprintf(" ORDER BY score DESC,c.id LIMIT $%d", len(args)), args...)
		if err != nil {
			return err
		}
		lexical, err = scanKnowledgeCandidates(rows)
		if err != nil {
			return err
		}
		if len(queryVector) == 0 {
			return nil
		}
		vectorArgs := []any{principal.TenantID, vectorLiteral(queryVector), embeddingModel}
		vectorScope, vectorScopeArgs := knowledgeScopeSQL(collectionIDs, documentIDs, 4)
		vectorArgs = append(vectorArgs, vectorScopeArgs...)
		vectorArgs = append(vectorArgs, max(30, limit*4))
		rows, err = tx.Query(ctx, `SELECT c.id,c.parent_id,c.document_id,d.original_name,
				c.content,p.content,c.heading_path,c.page_from,c.page_to,c.index_version,
				1-(c.embedding <=> $2::vector) score
			FROM knowledge_child_chunks c
			JOIN knowledge_parent_chunks p ON p.tenant_id=c.tenant_id AND p.id=c.parent_id
			JOIN knowledge_documents d ON d.tenant_id=c.tenant_id AND d.id=c.document_id
			WHERE c.tenant_id=$1 AND d.status='ready' AND d.deleted_at IS NULL
			  AND c.index_version=d.index_version AND c.embedding_model=$3`+vectorScope+
			fmt.Sprintf(" ORDER BY c.embedding <=> $2::vector,c.id LIMIT $%d", len(vectorArgs)), vectorArgs...)
		if err != nil {
			return err
		}
		semantic, err = scanKnowledgeCandidates(rows)
		return err
	})
	if err != nil {
		return nil, err
	}
	return fuseKnowledgeCandidates(lexical, semantic, limit), nil
}

func knowledgeLexicalQuery(query string) string {
	terms := strings.Fields(knowledge.SearchLexicalText(query))
	seen := make(map[string]struct{}, len(terms))
	result := make([]string, 0, min(len(terms), 64))
	for _, term := range terms {
		term = strings.TrimSpace(strings.Map(func(value rune) rune {
			if unicode.IsLetter(value) || unicode.IsNumber(value) || value == '_' || value == '-' {
				return value
			}
			return -1
		}, term))
		if term == "" {
			continue
		}
		if _, exists := seen[term]; exists {
			continue
		}
		seen[term] = struct{}{}
		result = append(result, term)
		if len(result) == 64 {
			break
		}
	}
	if len(result) == 0 {
		return "cortex"
	}
	return strings.Join(result, " | ")
}

func validateKnowledgeScope(
	ctx context.Context, tx pgx.Tx, principal domain.Principal, collectionIDs, documentIDs []int64,
) error {
	for _, id := range collectionIDs {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM knowledge_collections
			WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL)`, principal.TenantID, id).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return apierror.New("COLLECTION_NOT_FOUND", "知识集合不存在", 404)
		}
	}
	for _, id := range documentIDs {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM knowledge_documents
			WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL)`, principal.TenantID, id).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return apierror.New("DOCUMENT_NOT_FOUND", "知识文件不存在", 404)
		}
	}
	return nil
}

func knowledgeScopeSQL(collectionIDs, documentIDs []int64, start int) (string, []any) {
	var sql string
	var args []any
	if len(collectionIDs) > 0 {
		sql += fmt.Sprintf(" AND d.collection_id = ANY($%d::bigint[])", start+len(args))
		args = append(args, collectionIDs)
	}
	if len(documentIDs) > 0 {
		sql += fmt.Sprintf(" AND d.id = ANY($%d::bigint[])", start+len(args))
		args = append(args, documentIDs)
	}
	return sql, args
}

func scanKnowledgeCandidates(rows pgx.Rows) ([]KnowledgeCandidate, error) {
	defer rows.Close()
	var result []KnowledgeCandidate
	for rows.Next() {
		var item KnowledgeCandidate
		if err := rows.Scan(
			&item.ChildID, &item.ParentID, &item.DocumentID, &item.Document,
			&item.Child, &item.Parent, &item.HeadingPath, &item.PageFrom, &item.PageTo,
			&item.IndexVersion, &item.Score,
		); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func fuseKnowledgeCandidates(lexical, semantic []KnowledgeCandidate, limit int) []KnowledgeCandidate {
	type aggregate struct {
		item  KnowledgeCandidate
		score float64
	}
	values := map[int64]*aggregate{}
	add := func(items []KnowledgeCandidate) {
		for rank, item := range items {
			value := values[item.ChildID]
			if value == nil {
				value = &aggregate{item: item}
				values[item.ChildID] = value
			}
			value.score += 1.0 / float64(60+rank+1)
		}
	}
	add(lexical)
	add(semantic)
	ranked := make([]aggregate, 0, len(values))
	for _, value := range values {
		value.item.Score = value.score
		ranked = append(ranked, *value)
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			return ranked[i].item.ChildID < ranked[j].item.ChildID
		}
		return ranked[i].score > ranked[j].score
	})
	seenParent := map[int64]bool{}
	result := make([]KnowledgeCandidate, 0, min(limit, len(ranked)))
	for _, value := range ranked {
		if seenParent[value.item.ParentID] {
			continue
		}
		seenParent[value.item.ParentID] = true
		result = append(result, value.item)
		if len(result) >= limit {
			break
		}
	}
	return result
}

func (s *Store) SaveKnowledgeAnswer(
	ctx context.Context,
	principal domain.Principal,
	conversationID *int32,
	question, answer string,
	sources []KnowledgeCandidate,
) (int32, error) {
	var messageID int32
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		var id int32
		if conversationID != nil {
			if err := tx.QueryRow(ctx, `SELECT id FROM conversations
				WHERE tenant_id=$1 AND user_id=$2 AND id=$3`,
				principal.TenantID, principal.UserID, *conversationID).Scan(&id); err != nil {
				return apierror.New("CONVERSATION_NOT_FOUND", "对话不存在", 404)
			}
		} else {
			title := truncateText(question, 80)
			if err := tx.QueryRow(ctx, `INSERT INTO conversations
				(tenant_id,user_id,title,source_scope) VALUES ($1,$2,$3,'knowledge')
				RETURNING id`, principal.TenantID, principal.UserID, title).Scan(&id); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO messages
			(tenant_id,conversation_id,role,content,status)
			VALUES ($1,$2,'user',$3,'complete')`, principal.TenantID, id, question); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO messages
			(tenant_id,conversation_id,role,content,status)
			VALUES ($1,$2,'assistant',$3,'complete') RETURNING id`,
			principal.TenantID, id, answer).Scan(&messageID); err != nil {
			return err
		}
		for index, source := range sources {
			var valid bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(
				SELECT 1 FROM knowledge_child_chunks c
				JOIN knowledge_parent_chunks p ON p.tenant_id=c.tenant_id AND p.id=c.parent_id
				JOIN knowledge_documents d ON d.tenant_id=c.tenant_id AND d.id=c.document_id
				WHERE c.tenant_id=$1 AND c.id=$2 AND p.id=$3 AND d.id=$4
				  AND d.deleted_at IS NULL AND d.status='ready'
				  AND c.index_version=d.index_version)`,
				principal.TenantID, source.ChildID, source.ParentID, source.DocumentID,
			).Scan(&valid); err != nil {
				return err
			}
			if !valid {
				return apierror.New("KNOWLEDGE_SOURCE_INVALID", "知识来源已经失效", 409)
			}
			if _, err := tx.Exec(ctx, `INSERT INTO knowledge_message_sources
				(tenant_id,message_id,document_id,parent_id,child_id,index_version,
				 snippet,page_from,page_to,score,rank)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
				principal.TenantID, messageID, source.DocumentID, source.ParentID,
				source.ChildID, source.IndexVersion, truncateText(source.Child, 1000),
				source.PageFrom, source.PageTo, source.Score, index+1); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `UPDATE conversations SET updated_at=now()
			WHERE tenant_id=$1 AND id=$2`, principal.TenantID, id)
		return err
	})
	return messageID, err
}

func (s *Store) GetKnowledgeSources(
	ctx context.Context, principal domain.Principal, messageID int32,
) ([]map[string]any, error) {
	var result []map[string]any
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT s.document_id,d.original_name,s.snippet,
			s.page_from,s.page_to,s.rank,s.source_deleted
			FROM knowledge_message_sources s
			JOIN messages m ON m.tenant_id=s.tenant_id AND m.id=s.message_id
			LEFT JOIN knowledge_documents d ON d.tenant_id=s.tenant_id AND d.id=s.document_id
			WHERE s.tenant_id=$1 AND s.message_id=$2 ORDER BY s.rank`,
			principal.TenantID, messageID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var documentID int64
			var name *string
			var snippet *string
			var pageFrom, pageTo *int
			var rank int
			var deleted bool
			if err := rows.Scan(&documentID, &name, &snippet, &pageFrom, &pageTo, &rank, &deleted); err != nil {
				return err
			}
			result = append(result, map[string]any{
				"source_type": "knowledge_document", "document_id": documentID,
				"original_name": name, "snippet": snippet, "page_from": pageFrom,
				"page_to": pageTo, "rank": rank, "source_deleted": deleted,
			})
		}
		return rows.Err()
	})
	return result, err
}
