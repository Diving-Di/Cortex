package store

import (
	"context"
	"errors"
	"strings"

	"diary-listener/backend/internal/apierror"
	"diary-listener/backend/internal/domain"
	"github.com/jackc/pgx/v5"
)

// UpsertRecipeDocument inserts or updates a recipe_documents row by source_path.
func (s *Store) UpsertRecipeDocument(ctx context.Context, sourcePath, kind, category, title, summary string,
	ingredients, dietaryTerms []string, difficulty, caloriesText *string, contentMarkdown, contentSHA256, sourceRevision string, isActive bool) (int64, error) {
	var id int64
	err := s.withRecipeAdminTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `INSERT INTO recipe_documents
            (source_path,kind,category,title,summary,ingredients,dietary_terms,difficulty,calories_text,content_markdown,content_sha256,source_revision,is_active,created_at,updated_at)
            VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,now(),now())
            ON CONFLICT (source_path) DO UPDATE SET
                kind=EXCLUDED.kind, category=EXCLUDED.category, title=EXCLUDED.title,
                summary=EXCLUDED.summary, ingredients=EXCLUDED.ingredients, dietary_terms=EXCLUDED.dietary_terms,
                difficulty=EXCLUDED.difficulty, calories_text=EXCLUDED.calories_text, content_markdown=EXCLUDED.content_markdown,
                content_sha256=EXCLUDED.content_sha256, source_revision=EXCLUDED.source_revision, is_active=EXCLUDED.is_active,
                updated_at=now()
            RETURNING id`,
			sourcePath, kind, category, title, summary, ingredients, dietaryTerms,
			difficulty, caloriesText, contentMarkdown, contentSHA256, sourceRevision, isActive,
		).Scan(&id)
	})
	return id, err
}

// InsertRecipeChildChunks inserts multiple child chunks for a document and index version.
type RecipeChildChunk struct {
	ParentID      int64
	ChildIndex    int
	HeadingPath   string
	Content       string
	EmbeddingText string
	ContentHash   string
	TokenCount    int
}

func (s *Store) InsertRecipeChildChunks(ctx context.Context, documentID int64, indexVersion int, chunks []RecipeChildChunk) error {
	return s.withRecipeAdminTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM recipe_parent_chunks WHERE document_id=$1`, documentID); err != nil {
			return err
		}
		var parentID int64
		if err := tx.QueryRow(ctx, `INSERT INTO recipe_parent_chunks
				(document_id,parent_index,heading_path,content,token_count)
			VALUES ($1,0,'',$2,$3) RETURNING id`,
			documentID, chunks[0].Content, chunks[0].TokenCount).Scan(&parentID); err != nil {
			return err
		}
		for _, c := range chunks {
			if _, err := tx.Exec(ctx, `INSERT INTO recipe_child_chunks
				(document_id,parent_id,index_version,child_index,heading_path,content,embedding_text,
				 content_hash,search_vector,embedding_model,token_count,created_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,to_tsvector('simple',$7),NULL,$9,now())`,
				documentID, parentID, indexVersion, c.ChildIndex, c.HeadingPath, c.Content,
				c.EmbeddingText, c.ContentHash, c.TokenCount,
			); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) RecipeDocumentHash(ctx context.Context, sourcePath string) (string, bool, error) {
	var hash string
	err := s.withRecipeAdminTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT content_sha256 FROM recipe_documents WHERE source_path=$1`, sourcePath).
			Scan(&hash)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return hash, err == nil, err
}

func (s *Store) DeactivateMissingRecipeDocuments(ctx context.Context, activePaths []string) (int, error) {
	var count int64
	err := s.withRecipeAdminTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE recipe_documents SET is_active=false,updated_at=now()
			WHERE is_active=true AND NOT (source_path = ANY($1::text[]))`, activePaths)
		if err != nil {
			return err
		}
		count = tag.RowsAffected()
		return nil
	})
	return int(count), err
}

// UserPreferences represents a user's recipe preferences.
type UserPreferences struct {
	TenantID                   string
	UserID                     int32
	DietaryRestrictions        []string
	Timezone                   string
	Version                    int
	MarketplacePersonalization bool
}

func (s *Store) GetUserPreferences(ctx context.Context, principal domain.Principal, defaultTimezone string) (UserPreferences, error) {
	p := UserPreferences{
		TenantID:                   principal.TenantID.String(),
		UserID:                     principal.UserID,
		DietaryRestrictions:        []string{},
		Timezone:                   defaultTimezone,
		Version:                    0,
		MarketplacePersonalization: true,
	}
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `SELECT tenant_id::text,user_id,dietary_restrictions,timezone,version,marketplace_personalization
			FROM user_preferences WHERE tenant_id=$1 AND user_id=$2`,
			principal.TenantID, principal.UserID).
			Scan(&p.TenantID, &p.UserID, &p.DietaryRestrictions, &p.Timezone, &p.Version, &p.MarketplacePersonalization)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	return p, err
}

func (s *Store) UpdateUserPreferences(ctx context.Context, principal domain.Principal, dietary []string, timezone string, personalization bool, version int) (UserPreferences, error) {
	var p UserPreferences
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `INSERT INTO user_preferences
				(tenant_id,user_id,dietary_restrictions,timezone,marketplace_personalization,version,created_at,updated_at)
			VALUES ($1,$2,$3,$4,$5,1,now(),now())
			ON CONFLICT (tenant_id,user_id) DO UPDATE SET
				dietary_restrictions=EXCLUDED.dietary_restrictions,
				timezone=EXCLUDED.timezone,
				marketplace_personalization=EXCLUDED.marketplace_personalization,
				version=user_preferences.version+1,
				updated_at=now()
			WHERE user_preferences.version=$5`,
			principal.TenantID, principal.UserID, dietary, timezone, personalization, version)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apierror.New("VERSION_CONFLICT", "偏好设置已在其他设备更新", 409)
		}
		return tx.QueryRow(ctx, `SELECT tenant_id::text,user_id,dietary_restrictions,timezone,version,marketplace_personalization
			FROM user_preferences WHERE tenant_id=$1 AND user_id=$2`,
			principal.TenantID, principal.UserID).
			Scan(&p.TenantID, &p.UserID, &p.DietaryRestrictions, &p.Timezone, &p.Version, &p.MarketplacePersonalization)
	})
	return p, err
}

// CreateRecipeSyncRun creates a new sync run record with status 'running'.
func (s *Store) CreateRecipeSyncRun(ctx context.Context, sourceRevision string) (int64, error) {
	var id int64
	err := s.withRecipeAdminTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `INSERT INTO recipe_sync_runs (source_revision,status,started_at)
            VALUES ($1,'running',now()) RETURNING id`, sourceRevision).Scan(&id)
	})
	return id, err
}

// UpdateRecipeSyncRun updates a run record with final counts and status.
func (s *Store) UpdateRecipeSyncRun(ctx context.Context, runID int64, status string, scanned, created, updated, deactivated, failed int) error {
	return s.withRecipeAdminTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE recipe_sync_runs SET status=$1, scanned_count=$2, created_count=$3, updated_count=$4, deactivated_count=$5, failed_count=$6, finished_at=now()
            WHERE id=$7`, status, scanned, created, updated, deactivated, failed, runID)
		return err
	})
}

// LatestRecipeCorpusRevision returns the most recent successful source_revision or empty string.
func (s *Store) LatestRecipeCorpusRevision(ctx context.Context) (string, error) {
	var rev *string
	err := s.withRecipeAdminTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT source_revision FROM recipe_sync_runs WHERE status='success' ORDER BY finished_at DESC LIMIT 1`).Scan(&rev)
	})
	if err != nil {
		return "", err
	}
	if rev == nil {
		return "", nil
	}
	return *rev, nil
}

func (s *Store) RecipeIndexReady(ctx context.Context, embeddingModel string) error {
	return s.withRecipeAdminTx(ctx, func(tx pgx.Tx) error {
		var ready bool
		err := tx.QueryRow(ctx, `SELECT
			EXISTS (SELECT 1 FROM recipe_sync_runs WHERE status='success')
			AND EXISTS (
				SELECT 1 FROM recipe_child_chunks
				WHERE embedding IS NOT NULL AND embedding_model=$1
			)
			AND NOT EXISTS (
				SELECT 1 FROM recipe_index_jobs WHERE status IN ('queued','running','failed')
			)`, embeddingModel).Scan(&ready)
		if err != nil {
			return err
		}
		if !ready {
			return errors.New("recipe index is not ready")
		}
		return nil
	})
}

// ListEligibleRecipeIDs returns IDs of active dishes that do not overlap the given dietary restrictions.
func (s *Store) ListEligibleRecipeIDs(ctx context.Context, restrictions []string) ([]int64, error) {
	var ids []int64
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id FROM recipe_documents
			WHERE kind='dish' AND is_active=true
			  AND NOT (dietary_terms && $1::text[])
			ORDER BY id`, restrictions)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return err
			}
			ids = append(ids, id)
		}
		return rows.Err()
	})
	return ids, err
}

func (s *Store) GetRecipeSources(ctx context.Context, principal domain.Principal, messageID int32) ([]map[string]any, error) {
	result := []map[string]any{}
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT rms.document_id,d.title,rms.snippet,rms.rank
			FROM recipe_message_sources rms
			JOIN messages m ON m.id=rms.message_id AND m.tenant_id=rms.tenant_id
			JOIN conversations c ON c.id=m.conversation_id AND c.tenant_id=m.tenant_id
			JOIN recipe_documents d ON d.id=rms.document_id
			WHERE rms.tenant_id=$1 AND rms.message_id=$2 AND c.user_id=$3
			ORDER BY rms.rank`, principal.TenantID, messageID, principal.UserID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			var title, snippet string
			var rank int
			if err := rows.Scan(&id, &title, &snippet, &rank); err != nil {
				return err
			}
			result = append(result, map[string]any{
				"source_type": "recipe_document",
				"source_id":   id,
				"title":       title,
				"snippet":     snippet,
				"rank":        rank,
			})
		}
		return rows.Err()
	})
	return result, err
}

// RecipeCandidate is a minimal candidate returned by recipe search.
type RecipeCandidate struct {
	DocumentID int64
	Title      string
	Kind       string
	Snippet    string
}

// SearchRecipesByQuery performs a simple text search over title and content_markdown.
func (s *Store) SearchRecipesByQuery(ctx context.Context, query string, limit int) ([]RecipeCandidate, error) {
	var result []RecipeCandidate
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id,title,kind,content_markdown FROM recipe_documents
            WHERE is_active=true AND (title ILIKE $1 OR content_markdown ILIKE $1) LIMIT $2`, "%"+query+"%", limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c RecipeCandidate
			if err := rows.Scan(&c.DocumentID, &c.Title, &c.Kind, &c.Snippet); err != nil {
				return err
			}
			result = append(result, c)
		}
		return rows.Err()
	})
	return result, err
}

// SearchRecipesByVector performs semantic retrieval using an embedding vector and embedding_model.
func (s *Store) SearchRecipesByVector(ctx context.Context, queryVector []float32, embeddingModel string, limit int) ([]RecipeCandidate, error) {
	var result []RecipeCandidate
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT c.document_id,d.title,d.kind,c.content FROM recipe_child_chunks c
            JOIN recipe_documents d ON d.id=c.document_id
            WHERE d.is_active=true AND c.embedding_model=$1
            ORDER BY c.embedding <=> $2::vector LIMIT $3`, embeddingModel, vectorLiteral(queryVector), limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c RecipeCandidate
			var snippet string
			if err := rows.Scan(&c.DocumentID, &c.Title, &c.Kind, &snippet); err != nil {
				return err
			}
			c.Snippet = snippet
			result = append(result, c)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) GetRecipeCandidate(ctx context.Context, documentID int64) (RecipeCandidate, error) {
	var result RecipeCandidate
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT id,title,kind,content_markdown
			FROM recipe_documents WHERE id=$1 AND is_active=true`, documentID).
			Scan(&result.DocumentID, &result.Title, &result.Kind, &result.Snippet)
		if errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("RECIPE_NOT_FOUND", "菜谱不存在", 404)
		}
		return err
	})
	return result, err
}

// SaveRecipeAnswer saves a recipe chat answer and its minimal sources. Returns message_id and conversation_id.
func (s *Store) SaveRecipeAnswer(
	ctx context.Context,
	principal domain.Principal,
	conversationID *int32,
	requestID string,
	question, answer string,
	sources []RecipeCandidate,
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
				principal.TenantID, principal.UserID, *conversationID, "recipe").Scan(&id); err != nil {
				return apierror.New("CONVERSATION_NOT_FOUND", "对话不存在", 404)
			}
		} else {
			title := recipeConversationTitle(question)
			if err := tx.QueryRow(ctx, `INSERT INTO conversations
                (tenant_id,user_id,title,source_scope) VALUES ($1,$2,$3,$4) RETURNING id`,
				principal.TenantID, principal.UserID, title, "recipe").Scan(&id); err != nil {
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
            VALUES ($1,$2,'assistant',$3,'complete') RETURNING id`, principal.TenantID, id, answer).Scan(&messageID); err != nil {
			return err
		}
		for idx, src := range sources {
			if _, err := tx.Exec(ctx, `INSERT INTO recipe_message_sources
                (tenant_id,message_id,document_id,snippet,rank)
                VALUES ($1,$2,$3,$4,$5)`, principal.TenantID, messageID, src.DocumentID, truncateText(src.Snippet, 1000), idx+1); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `UPDATE conversations SET updated_at=now() WHERE tenant_id=$1 AND id=$2`, principal.TenantID, id)
		return err
	})
	return messageID, savedConversationID, err
}

func recipeConversationTitle(question string) string {
	title := strings.Join(strings.Fields(question), " ")
	title = strings.TrimSpace(title)
	for _, prefix := range []string{"请问一下", "请问", "麻烦问一下", "麻烦问", "我想知道"} {
		title = strings.TrimSpace(strings.TrimPrefix(title, prefix))
	}
	if index := strings.IndexAny(title, "。！？!?；;\n"); index >= 0 {
		title = strings.TrimSpace(title[:index])
	}
	title = strings.Trim(title, "，,：:。.！？!?；; ")
	if title == "" {
		return "菜谱问答"
	}
	runes := []rune(title)
	if len(runes) > 32 {
		return string(runes[:32]) + "…"
	}
	return title
}

// UpdateRecipeChildEmbeddingModel sets the embedding_model for child chunks matching document_id and content_hash.
func (s *Store) UpdateRecipeChildEmbeddingModel(ctx context.Context, documentID int64, contentHash, model string) error {
	return s.withRecipeAdminTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE recipe_child_chunks SET embedding_model=$1 WHERE document_id=$2 AND content_hash=$3 AND embedding_model IS NULL`, model, documentID, contentHash)
		return err
	})
}

// UpdateRecipeChildEmbedding writes the embedding vector and model for a child chunk.
func (s *Store) UpdateRecipeChildEmbedding(ctx context.Context, documentID int64, contentHash string, vec []float32, model string) error {
	return s.withRecipeAdminTx(ctx, func(tx pgx.Tx) error {
		literal := vectorLiteral(vec)
		_, err := tx.Exec(ctx, `UPDATE recipe_child_chunks SET embedding=$1::vector, embedding_model=$2 WHERE document_id=$3 AND content_hash=$4 AND embedding IS NULL`, literal, model, documentID, contentHash)
		return err
	})
}

func (s *Store) withRecipeAdminTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := s.AdminPool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
