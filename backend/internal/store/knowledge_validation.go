package store

import (
	"context"
	"cortex/backend/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type CandidateIdentity struct {
	DocumentID, ParentID uuid.UUID
	IndexVersion         int
}

func (s *Store) ValidateKnowledgeCandidateIdentities(ctx context.Context, p domain.Principal, items []CandidateIdentity) (map[CandidateIdentity]bool, error) {
	valid := make(map[CandidateIdentity]bool)
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, p); err != nil {
			return err
		}
		for _, item := range items {
			var ok bool
			err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM knowledge_documents d JOIN knowledge_parent_chunks c ON c.tenant_id=d.tenant_id AND c.document_id=d.id AND c.id=$4 AND c.index_version=d.active_index_version LEFT JOIN notes n ON n.tenant_id=d.tenant_id AND n.id=d.note_id WHERE d.tenant_id=$1 AND d.id=$2 AND d.active_index_version=$3 AND d.status='ready' AND d.deleted_at IS NULL AND d.knowledge_enabled AND (d.source_type<>'note' OR n.deleted_at IS NULL))`, p.TenantID, item.DocumentID, item.IndexVersion, item.ParentID).Scan(&ok)
			if err != nil {
				return err
			}
			if ok {
				valid[item] = true
			}
		}
		return nil
	})
	return valid, err
}
