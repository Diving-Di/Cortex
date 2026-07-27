package store

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"diary-listener/backend/internal/domain"
	"diary-listener/backend/internal/knowledge"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type KnowledgeIndexJob struct {
	ID                 int64
	TenantID           uuid.UUID
	UserID             int32
	DocumentID         int64
	TargetIndexVersion int
}

func (s *Store) ClaimKnowledgeIndexJobs(
	ctx context.Context, owner string, limit int, lease time.Duration,
) ([]KnowledgeIndexJob, error) {
	tx, err := s.AdminPool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, `WITH candidates AS (
			SELECT j.id
			FROM knowledge_index_jobs j
			WHERE (j.status='queued' AND j.next_attempt_at <= now())
			   OR (j.status='running' AND j.lease_until < now())
			ORDER BY j.next_attempt_at,j.id
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		)
		UPDATE knowledge_index_jobs j SET
			status='running',lease_owner=$2,lease_until=now()+$3::interval,
			attempts=j.attempts+1,updated_at=now()
		FROM candidates c,tenants t
		WHERE j.id=c.id AND t.id=j.tenant_id
		RETURNING j.id,j.tenant_id,t.user_id,j.document_id,j.target_index_version`,
		limit, owner, lease.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []KnowledgeIndexJob
	for rows.Next() {
		var item KnowledgeIndexJob
		if err := rows.Scan(&item.ID, &item.TenantID, &item.UserID, &item.DocumentID, &item.TargetIndexVersion); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Store) CompleteKnowledgeIndex(
	ctx context.Context,
	principal domain.Principal,
	job KnowledgeIndexJob,
	document knowledge.Document,
	parents []knowledge.ParentChunk,
	embeddings [][][]float32,
	embeddingModel string,
) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		var status string
		var currentVersion int
		if err := tx.QueryRow(ctx, `SELECT status,index_version FROM knowledge_documents
			WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, principal.TenantID, job.DocumentID,
		).Scan(&status, &currentVersion); err != nil {
			return err
		}
		if status == "deleting" || currentVersion != job.TargetIndexVersion {
			_, err := tx.Exec(ctx, `UPDATE knowledge_index_jobs SET status='cancelled',
				lease_owner=NULL,lease_until=NULL,updated_at=now()
				WHERE tenant_id=$1 AND id=$2`, principal.TenantID, job.ID)
			return err
		}
		if len(embeddings) != len(parents) {
			return fmt.Errorf("parent embedding groups mismatch")
		}
		if _, err := tx.Exec(ctx, `DELETE FROM knowledge_parent_chunks
			WHERE tenant_id=$1 AND document_id=$2 AND index_version=$3`,
			principal.TenantID, job.DocumentID, job.TargetIndexVersion); err != nil {
			return err
		}
		parentIDs := make([]int64, len(parents))
		childCount := 0
		for parentIndex, parent := range parents {
			heading := strings.Join(parent.HeadingPath, " > ")
			var pageFrom, pageTo *int
			if parent.PageFrom > 0 {
				value := parent.PageFrom
				pageFrom = &value
			}
			if parent.PageTo > 0 {
				value := parent.PageTo
				pageTo = &value
			}
			if err := tx.QueryRow(ctx, `INSERT INTO knowledge_parent_chunks
				(tenant_id,document_id,index_version,parent_index,page_from,page_to,
				 heading_path,content,content_hash,token_count)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
				principal.TenantID, job.DocumentID, job.TargetIndexVersion, parent.Index,
				pageFrom, pageTo, heading, parent.Content, knowledge.ContentHash(parent.Content),
				parent.TokenCount,
			).Scan(&parentIDs[parentIndex]); err != nil {
				return err
			}
			if len(embeddings[parentIndex]) != len(parent.Children) {
				return fmt.Errorf("child embedding count mismatch")
			}
			for childIndex, child := range parent.Children {
				vector := vectorLiteral(embeddings[parentIndex][childIndex])
				childHeading := strings.Join(child.HeadingPath, " > ")
				var childPageFrom, childPageTo *int
				if child.PageFrom > 0 {
					value := child.PageFrom
					childPageFrom = &value
				}
				if child.PageTo > 0 {
					value := child.PageTo
					childPageTo = &value
				}
				if _, err := tx.Exec(ctx, `INSERT INTO knowledge_child_chunks
					(tenant_id,document_id,parent_id,index_version,child_index,page_from,page_to,
					 heading_path,content,embedding_text,content_hash,search_vector,embedding,
					 embedding_model,token_count)
					VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,
					 to_tsvector('simple',$12),$13::vector,$14,$15)`,
					principal.TenantID, job.DocumentID, parentIDs[parentIndex],
					job.TargetIndexVersion, child.Index, childPageFrom, childPageTo, childHeading,
					child.Content, child.EmbeddingText, knowledge.ContentHash(child.Content),
					knowledge.SearchLexicalText(child.EmbeddingText), vector, embeddingModel, child.TokenCount); err != nil {
					return err
				}
				childCount++
			}
		}
		for index, id := range parentIDs {
			var previous, next *int64
			if index > 0 {
				value := parentIDs[index-1]
				previous = &value
			}
			if index+1 < len(parentIDs) {
				value := parentIDs[index+1]
				next = &value
			}
			if _, err := tx.Exec(ctx, `UPDATE knowledge_parent_chunks SET
				previous_parent_id=$3,next_parent_id=$4
				WHERE tenant_id=$1 AND id=$2`, principal.TenantID, id, previous, next); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE knowledge_documents SET
			status='ready',page_count=$3,character_count=$4,parent_chunk_count=$5,
			child_chunk_count=$6,language=$7,error_code=NULL,error_message=NULL,updated_at=now()
			WHERE tenant_id=$1 AND id=$2`,
			principal.TenantID, job.DocumentID, document.PageCount, document.Characters,
			len(parents), childCount, document.Language); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM knowledge_parent_chunks
			WHERE tenant_id=$1 AND document_id=$2 AND index_version<>$3`,
			principal.TenantID, job.DocumentID, job.TargetIndexVersion); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE knowledge_index_jobs SET
			status='success',lease_owner=NULL,lease_until=NULL,last_error_code=NULL,updated_at=now()
			WHERE tenant_id=$1 AND id=$2`, principal.TenantID, job.ID)
		return err
	})
}

func (s *Store) FailKnowledgeIndex(
	ctx context.Context, principal domain.Principal, job KnowledgeIndexJob, code string, retry bool,
) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		status := "failed"
		if retry {
			status = "queued"
		}
		if _, err := tx.Exec(ctx, `UPDATE knowledge_index_jobs SET status=$3,
			lease_owner=NULL,lease_until=NULL,last_error_code=$4,
			next_attempt_at=now()+make_interval(secs => LEAST(3600, attempts*attempts*10)),
			updated_at=now() WHERE tenant_id=$1 AND id=$2`,
			principal.TenantID, job.ID, status, code); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE knowledge_documents SET status='failed',
			error_code=$3,error_message=$4,updated_at=now()
			WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL`,
			principal.TenantID, job.DocumentID, code, knowledgeErrorMessage(code))
		return err
	})
}

func vectorLiteral(values []float32) string {
	var output strings.Builder
	output.WriteByte('[')
	for index, value := range values {
		if index > 0 {
			output.WriteByte(',')
		}
		output.WriteString(strconv.FormatFloat(float64(value), 'g', -1, 32))
	}
	output.WriteByte(']')
	return output.String()
}

func knowledgeErrorMessage(code string) string {
	switch code {
	case "DOCUMENT_ENCRYPTED":
		return "PDF 已加密，无法建立索引"
	case "DOCUMENT_OCR_REQUIRED":
		return "文档没有可提取文本，需要 OCR"
	case "DOCUMENT_PARSE_LIMIT":
		return "文档解析超过安全限制"
	case "EMBEDDING_UNAVAILABLE":
		return "本地语义模型暂时不可用"
	default:
		return "知识文件处理失败"
	}
}
