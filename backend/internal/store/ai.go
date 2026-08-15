package store

import (
	"context"
	"time"

	"cortex/backend/internal/domain"
	"github.com/jackc/pgx/v5"
)

type AIProvider struct {
	ID           int32  `json:"id"`
	DisplayName  string `json:"display_name"`
	BaseURL      string `json:"base_url"`
	DefaultModel string `json:"default_model"`
	Capabilities string `json:"capabilities"`
}

type AIUsage struct {
	RequestType    string
	Model          string
	InputTokens    int
	OutputTokens   int
	Duration       time.Duration
	Status         string
	ErrorCode      *string
	ConversationID *int32
}

func (s *Store) UpsertAIProvider(ctx context.Context, principal domain.Principal, input AIProvider) (AIProvider, error) {
	var result AIProvider
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `SELECT id FROM ai_providers WHERE tenant_id=$1 ORDER BY id LIMIT 1`, principal.TenantID).Scan(&result.ID)
		if err != nil && err != pgx.ErrNoRows {
			return err
		}
		if err == pgx.ErrNoRows {
			return tx.QueryRow(ctx, `
                INSERT INTO ai_providers (tenant_id,display_name,base_url,default_model,capabilities)
                VALUES ($1,$2,$3,$4,$5)
                RETURNING id,display_name,base_url,default_model,capabilities`,
				principal.TenantID, input.DisplayName, input.BaseURL, input.DefaultModel, input.Capabilities,
			).Scan(
				&result.ID, &result.DisplayName, &result.BaseURL,
				&result.DefaultModel, &result.Capabilities,
			)
		}
		return tx.QueryRow(ctx, `UPDATE ai_providers SET
            display_name=$2,base_url=$3,default_model=$4,capabilities=$5
            WHERE tenant_id=$1 AND id=$6
            RETURNING id,display_name,base_url,default_model,capabilities`,
			principal.TenantID, input.DisplayName, input.BaseURL, input.DefaultModel, input.Capabilities,
			result.ID,
		).Scan(
			&result.ID, &result.DisplayName, &result.BaseURL,
			&result.DefaultModel, &result.Capabilities,
		)
	})
	return result, err
}

func (s *Store) RecordAIUsage(ctx context.Context, principal domain.Principal, usage AIUsage) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO ai_usage_records
            (tenant_id,user_id,request_type,input_tokens,output_tokens,model,duration_ms,status,error_code,conversation_id)
            VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			principal.TenantID, principal.UserID, usage.RequestType,
			usage.InputTokens, usage.OutputTokens, usage.Model,
			usage.Duration.Milliseconds(), usage.Status, usage.ErrorCode, usage.ConversationID,
		)
		return err
	})
}
