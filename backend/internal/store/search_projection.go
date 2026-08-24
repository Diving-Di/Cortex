package store

import (
	"context"
	"github.com/google/uuid"
	"strconv"
	"strings"
)

type SearchProjectionChunk struct {
	TenantID, DocumentID, ParentID, ChunkID            uuid.UUID
	IndexVersion                                       int
	CollectionID                                       *uuid.UUID
	Title, SourceType, SourcePath, Content, SearchText string
	Heading                                            []string
	Embedding                                          []float32
}

func (s *Store) LoadSearchProjection(ctx context.Context, documentID uuid.UUID) ([]SearchProjectionChunk, error) {
	rows, err := s.AdminPool.Query(ctx, `SELECT d.tenant_id,d.id,p.id,c.id,d.active_index_version,d.collection_id,d.title,d.source_type,coalesce(d.object_key,d.stored_path,''),p.content,p.heading_path,c.embedding_text,c.embedding::text FROM knowledge_documents d JOIN knowledge_parent_chunks p ON p.tenant_id=d.tenant_id AND p.document_id=d.id AND p.index_version=d.active_index_version JOIN knowledge_child_chunks c ON c.tenant_id=p.tenant_id AND c.parent_id=p.id AND c.index_version=p.index_version WHERE d.id=$1 AND d.status='ready' AND d.deleted_at IS NULL AND d.knowledge_enabled ORDER BY p.ordinal,c.ordinal`, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SearchProjectionChunk
	for rows.Next() {
		var v SearchProjectionChunk
		var vectorText string
		if err = rows.Scan(&v.TenantID, &v.DocumentID, &v.ParentID, &v.ChunkID, &v.IndexVersion, &v.CollectionID, &v.Title, &v.SourceType, &v.SourcePath, &v.Content, &v.Heading, &v.SearchText, &vectorText); err != nil {
			return nil, err
		}
		parts := strings.Split(strings.Trim(vectorText, "[]"), ",")
		v.Embedding = make([]float32, len(parts))
		for i, part := range parts {
			value, parseErr := strconv.ParseFloat(part, 32)
			if parseErr != nil {
				return nil, parseErr
			}
			v.Embedding[i] = float32(value)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) CompleteSearchProjection(ctx context.Context, documentID uuid.UUID, version, count int, checksum string, projectionErr error) error {
	if projectionErr == nil {
		_, err := s.AdminPool.Exec(ctx, `UPDATE search_projections SET status='ready',chunk_count=$3,checksum=$4,projected_at=now(),last_error_code=NULL WHERE document_id=$1 AND index_version=$2`, documentID, version, count, checksum)
		return err
	}
	_, err := s.AdminPool.Exec(ctx, `UPDATE search_projections SET status='failed',last_error_code='SEARCH_PROJECTION_FAILED' WHERE document_id=$1 AND index_version=$2`, documentID, version)
	return err
}
