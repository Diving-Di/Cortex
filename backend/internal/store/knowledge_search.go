package store

import (
	"context"
	"errors"
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

type KnowledgeReplay struct {
	RequestID      string
	ConversationID int32
	MessageID      int32
	Answer         string
	Sources        []domain.Source
}

func (s *Store) FindKnowledgeRequest(
	ctx context.Context, principal domain.Principal, requestID string,
) (KnowledgeReplay, bool, error) {
	var result KnowledgeReplay
	found := false
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		var userMessageID int32
		err := tx.QueryRow(ctx, `SELECT id,conversation_id FROM messages
			WHERE tenant_id=$1 AND request_id=$2 AND role='user'`,
			principal.TenantID, requestID).Scan(&userMessageID, &result.ConversationID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `SELECT id,content FROM messages
			WHERE tenant_id=$1 AND conversation_id=$2 AND role='assistant' AND id>$3
			ORDER BY id LIMIT 1`, principal.TenantID, result.ConversationID, userMessageID,
		).Scan(&result.MessageID, &result.Answer)
		if errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("REQUEST_IN_PROGRESS", "相同请求仍在处理中", 409)
		}
		if err != nil {
			return err
		}
		result.RequestID = requestID
		found = true
		return nil
	})
	if err != nil || !found {
		return result, found, err
	}
	knowledgeSources, err := s.GetKnowledgeSources(ctx, principal, result.MessageID)
	if err != nil {
		return result, false, err
	}
	memorySources, err := s.GetMemorySources(ctx, principal, result.MessageID)
	if err != nil {
		return result, false, err
	}
	for _, value := range knowledgeSources {
		item := domain.Source{Type: "knowledge_document"}
		item.ID, _ = value["document_id"].(int64)
		if title, ok := value["original_name"].(*string); ok && title != nil {
			item.Title = *title
		}
		item.Snippet, _ = value["snippet"].(*string)
		item.PageFrom, _ = value["page_from"].(*int)
		item.PageTo, _ = value["page_to"].(*int)
		item.Rank, _ = value["rank"].(int)
		item.SourceDeleted, _ = value["source_deleted"].(bool)
		result.Sources = append(result.Sources, item)
	}
	for _, value := range memorySources {
		id, _ := value["id"].(int32)
		title, _ := value["title"].(string)
		text, _ := value["snippet"].(string)
		rank, _ := value["rank"].(int32)
		result.Sources = append(result.Sources, domain.Source{
			Type: "growth_note", ID: int64(id), Title: title, Snippet: &text, Rank: int(rank),
		})
	}
	return result, true, nil
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
	semantic = filterKnowledgeSemanticCandidates(semantic, len(lexical) > 0)
	result := fuseKnowledgeCandidates(lexical, semantic, limit)
	err = s.expandAdjacentKnowledgeParents(ctx, principal, result, 1200)
	return result, err
}

const knowledgeSemanticOnlyMinimumScore = 0.70

func filterKnowledgeSemanticCandidates(
	candidates []KnowledgeCandidate, hasLexicalEvidence bool,
) []KnowledgeCandidate {
	if hasLexicalEvidence {
		return candidates
	}
	result := candidates[:0]
	for _, candidate := range candidates {
		if candidate.Score >= knowledgeSemanticOnlyMinimumScore {
			result = append(result, candidate)
		}
	}
	return result
}

func (s *Store) expandAdjacentKnowledgeParents(
	ctx context.Context, principal domain.Principal, candidates []KnowledgeCandidate, tokenBudget int,
) error {
	if len(candidates) == 0 || tokenBudget <= 0 {
		return nil
	}
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		remaining := tokenBudget
		for index := range candidates {
			rows, err := tx.Query(ctx, `SELECT neighbor.content,neighbor.parent_index-current.parent_index direction
				FROM knowledge_parent_chunks current
				JOIN knowledge_parent_chunks neighbor ON neighbor.tenant_id=current.tenant_id
					AND neighbor.document_id=current.document_id
					AND neighbor.index_version=current.index_version
					AND neighbor.parent_index IN (current.parent_index-1,current.parent_index+1)
				JOIN knowledge_documents d ON d.tenant_id=current.tenant_id AND d.id=current.document_id
				WHERE current.tenant_id=$1 AND current.id=$2 AND d.status='ready'
					AND d.deleted_at IS NULL AND current.index_version=d.index_version
				ORDER BY abs(neighbor.parent_index-current.parent_index),neighbor.parent_index`,
				principal.TenantID, candidates[index].ParentID)
			if err != nil {
				return err
			}
			var before, after string
			for rows.Next() {
				var content string
				var direction int
				if err := rows.Scan(&content, &direction); err != nil {
					rows.Close()
					return err
				}
				cost := knowledge.EstimateTokens(content)
				if cost > remaining {
					continue
				}
				remaining -= cost
				if direction < 0 {
					before = content
				} else {
					after = content
				}
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				return err
			}
			rows.Close()
			if before != "" {
				candidates[index].Parent = before + "\n\n" + candidates[index].Parent
			}
			if after != "" {
				candidates[index].Parent += "\n\n" + after
			}
			if remaining <= 0 {
				break
			}
		}
		return nil
	})
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
	requestID string,
	sourceScope string,
	question, answer string,
	sources []KnowledgeCandidate,
	growthSources []SourceNote,
) (int32, int32, error) {
	var messageID int32
	var savedConversationID int32
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		var id int32
		if conversationID != nil {
			if err := tx.QueryRow(ctx, `SELECT id FROM conversations
				WHERE tenant_id=$1 AND user_id=$2 AND id=$3 AND source_scope=$4`,
				principal.TenantID, principal.UserID, *conversationID, sourceScope).Scan(&id); err != nil {
				return apierror.New("CONVERSATION_NOT_FOUND", "对话不存在", 404)
			}
		} else {
			title := truncateText(question, 80)
			if err := tx.QueryRow(ctx, `INSERT INTO conversations
				(tenant_id,user_id,title,source_scope) VALUES ($1,$2,$3,$4)
				RETURNING id`, principal.TenantID, principal.UserID, title, sourceScope).Scan(&id); err != nil {
				return err
			}
		}
		savedConversationID = id
		if _, err := tx.Exec(ctx, `INSERT INTO messages
			(tenant_id,conversation_id,role,content,status,request_id)
			VALUES ($1,$2,'user',$3,'complete',$4)`, principal.TenantID, id, question, requestID); err != nil {
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
		for index, source := range growthSources {
			if _, err := tx.Exec(ctx, `INSERT INTO message_sources
				(tenant_id,message_id,note_id,snippet,relevance,rank)
				SELECT $1,$2,id,$4,$5,$6 FROM notes
				WHERE tenant_id=$1 AND id=$3 AND deleted_at IS NULL`,
				principal.TenantID, messageID, source.ID, truncateText(source.Snippet, 500),
				len(growthSources)-index, index+1); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `UPDATE conversations SET updated_at=now()
			WHERE tenant_id=$1 AND id=$2`, principal.TenantID, id)
		return err
	})
	return messageID, savedConversationID, err
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

func (s *Store) GetChatSources(
	ctx context.Context, principal domain.Principal, messageID int32,
) ([]map[string]any, error) {
	knowledgeSources, err := s.GetKnowledgeSources(ctx, principal, messageID)
	if err != nil {
		return nil, err
	}
	growthSources, err := s.GetMemorySources(ctx, principal, messageID)
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(knowledgeSources)+len(growthSources))
	for _, source := range knowledgeSources {
		result = append(result, map[string]any{
			"source_type":    "knowledge_document",
			"source_id":      source["document_id"],
			"title":          source["original_name"],
			"document_id":    source["document_id"],
			"original_name":  source["original_name"],
			"snippet":        source["snippet"],
			"page_from":      source["page_from"],
			"page_to":        source["page_to"],
			"rank":           source["rank"],
			"source_deleted": source["source_deleted"],
		})
	}
	for _, source := range growthSources {
		result = append(result, map[string]any{
			"source_type":    "growth_note",
			"source_id":      source["id"],
			"title":          source["title"],
			"snippet":        source["snippet"],
			"rank":           source["rank"],
			"source_deleted": false,
		})
	}
	return result, nil
}
