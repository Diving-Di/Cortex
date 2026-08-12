package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"diary-listener/backend/internal/apierror"
	"diary-listener/backend/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type PublicProfile struct {
	PublicID     uuid.UUID `json:"public_id"`
	Nickname     string    `json:"nickname"`
	Discoverable bool      `json:"discoverable"`
	Version      int       `json:"version"`
}

type WritingTemplate struct {
	ID              int64      `json:"id"`
	Title           string     `json:"title"`
	Description     string     `json:"description"`
	ContentMarkdown string     `json:"content_markdown"`
	Category        string     `json:"category"`
	Status          string     `json:"status"`
	Version         int        `json:"version"`
	PublicID        *uuid.UUID `json:"public_id,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type PublicTemplate struct {
	PublicID        uuid.UUID `json:"public_id"`
	AuthorNickname  string    `json:"author_nickname"`
	Version         int       `json:"version"`
	Title           string    `json:"title"`
	Description     string    `json:"description"`
	ContentMarkdown string    `json:"content_markdown"`
	Category        string    `json:"category"`
	PublishedAt     time.Time `json:"published_at"`
	LikeCount       int64     `json:"like_count"`
	FavoriteCount   int64     `json:"favorite_count"`
	UsageCount      int64     `json:"usage_count"`
	Liked           bool      `json:"liked"`
	Favorited       bool      `json:"favorited"`
}

type TemplateInput struct {
	Title, Description, ContentMarkdown, Category string
}

var templateSecretPattern = regexp.MustCompile(`(?i)(sk-[a-z0-9_-]{16,}|AKIA[0-9A-Z]{16}|AIza[0-9A-Za-z_-]{20,}|-----BEGIN [A-Z ]*PRIVATE KEY-----)`)

func validateTemplateInput(input TemplateInput) error {
	if strings.TrimSpace(input.Title) == "" || utf8.RuneCountInString(input.Title) > 120 ||
		utf8.RuneCountInString(input.Description) > 500 || len(input.ContentMarkdown) == 0 ||
		len(input.ContentMarkdown) > 65536 || strings.TrimSpace(input.Category) == "" ||
		utf8.RuneCountInString(input.Category) > 40 || !safeTemplateText(input.Title, false) ||
		!safeTemplateText(input.Description, false) || !safeTemplateText(input.ContentMarkdown, true) ||
		!safeTemplateMarkdown(input.ContentMarkdown) {
		return apierror.Validation(nil)
	}
	return nil
}

func safeTemplateMarkdown(value string) bool {
	lower := strings.ToLower(value)
	for _, forbidden := range []string{"javascript:", "data:", "file:", "vbscript:", "![", "<img"} {
		if strings.Contains(lower, forbidden) {
			return false
		}
	}
	return !templateSecretPattern.MatchString(value)
}

func safeTemplateText(value string, multiline bool) bool {
	if !utf8.ValidString(value) {
		return false
	}
	depth := 0
	for _, r := range value {
		if unicode.IsControl(r) && !(multiline && (r == '\n' || r == '\r' || r == '\t')) {
			return false
		}
		switch r {
		case '[', '(', '{':
			depth++
		case ']', ')', '}':
			if depth > 0 {
				depth--
			}
		}
		if depth > 64 {
			return false
		}
	}
	return true
}

func validatePublicNickname(value string) bool {
	if !safeTemplateText(value, false) {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(value))
	reserved := []string{"官方", "管理员", "客服", "diary listener", "diary-listener", "admin", "administrator", "support"}
	for _, word := range reserved {
		if lower == word || strings.Contains(lower, word) {
			return false
		}
	}
	return true
}

func (s *Store) GetPublicProfile(ctx context.Context, principal domain.Principal) (PublicProfile, error) {
	var result PublicProfile
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `SELECT public_id,nickname,discoverable,version FROM public_profiles WHERE tenant_id=$1`, principal.TenantID).
			Scan(&result.PublicID, &result.Nickname, &result.Discoverable, &result.Version)
		if errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("PUBLIC_PROFILE_NOT_SET", "尚未设置公开昵称", 404)
		}
		return err
	})
	return result, err
}

func (s *Store) UpsertPublicProfile(ctx context.Context, principal domain.Principal, nickname string, discoverable bool, expectedVersion *int) (PublicProfile, error) {
	nickname = strings.TrimSpace(nickname)
	if utf8.RuneCountInString(nickname) < 2 || utf8.RuneCountInString(nickname) > 40 || !validatePublicNickname(nickname) {
		return PublicProfile{}, apierror.Validation(nil)
	}
	var result PublicProfile
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		if expectedVersion == nil {
			return tx.QueryRow(ctx, `INSERT INTO public_profiles(tenant_id,user_id,public_id,nickname,discoverable)
				VALUES($1,$2,$3,$4,$5) ON CONFLICT(tenant_id) DO UPDATE SET nickname=EXCLUDED.nickname,
				discoverable=EXCLUDED.discoverable,version=public_profiles.version+1,updated_at=now()
				RETURNING public_id,nickname,discoverable,version`, principal.TenantID, principal.UserID, uuid.New(), nickname, discoverable).
				Scan(&result.PublicID, &result.Nickname, &result.Discoverable, &result.Version)
		}
		err := tx.QueryRow(ctx, `UPDATE public_profiles SET nickname=$1,discoverable=$2,version=version+1,updated_at=now()
			WHERE tenant_id=$3 AND version=$4 RETURNING public_id,nickname,discoverable,version`, nickname, discoverable, principal.TenantID, *expectedVersion).
			Scan(&result.PublicID, &result.Nickname, &result.Discoverable, &result.Version)
		if errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("PUBLIC_PROFILE_VERSION_CONFLICT", "公开资料已被修改", 409)
		}
		return err
	})
	return result, err
}

func scanWritingTemplate(row pgx.Row, item *WritingTemplate) error {
	return row.Scan(&item.ID, &item.Title, &item.Description, &item.ContentMarkdown, &item.Category,
		&item.Status, &item.Version, &item.PublicID, &item.CreatedAt, &item.UpdatedAt)
}

const writingTemplateSelect = `SELECT w.id,w.title,w.description,w.content_markdown,w.category,w.status,w.version,
	(SELECT p.public_template_id FROM template_publications tp JOIN published_template_snapshots p ON p.source_publication_id=tp.id
	 WHERE tp.tenant_id=w.tenant_id AND tp.template_id=w.id ORDER BY tp.id DESC LIMIT 1),w.created_at,w.updated_at FROM writing_templates w`

func (s *Store) ListWritingTemplates(ctx context.Context, principal domain.Principal) ([]WritingTemplate, error) {
	result := make([]WritingTemplate, 0)
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, writingTemplateSelect+` WHERE w.tenant_id=$1 AND w.deleted_at IS NULL ORDER BY w.updated_at DESC`, principal.TenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item WritingTemplate
			if err := scanWritingTemplate(rows, &item); err != nil {
				return err
			}
			result = append(result, item)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) GetWritingTemplate(ctx context.Context, principal domain.Principal, id int64) (WritingTemplate, error) {
	var result WritingTemplate
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		err := scanWritingTemplate(tx.QueryRow(ctx, writingTemplateSelect+` WHERE w.tenant_id=$1 AND w.id=$2 AND w.deleted_at IS NULL`, principal.TenantID, id), &result)
		if errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("TEMPLATE_NOT_FOUND", "模板不存在", 404)
		}
		return err
	})
	return result, err
}

func (s *Store) CreateWritingTemplate(ctx context.Context, principal domain.Principal, input TemplateInput) (WritingTemplate, error) {
	if err := validateTemplateInput(input); err != nil {
		return WritingTemplate{}, err
	}
	var result WritingTemplate
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `INSERT INTO writing_templates(tenant_id,created_by,title,description,content_markdown,category)
			VALUES($1,$2,$3,$4,$5,$6) RETURNING id,title,description,content_markdown,category,status,version,NULL::uuid,created_at,updated_at`,
			principal.TenantID, principal.UserID, strings.TrimSpace(input.Title), input.Description, input.ContentMarkdown, input.Category).
			Scan(&result.ID, &result.Title, &result.Description, &result.ContentMarkdown, &result.Category, &result.Status, &result.Version, &result.PublicID, &result.CreatedAt, &result.UpdatedAt)
	})
	return result, err
}

func (s *Store) UpdateWritingTemplate(ctx context.Context, principal domain.Principal, id int64, input TemplateInput, expected int) (WritingTemplate, error) {
	if err := validateTemplateInput(input); err != nil {
		return WritingTemplate{}, err
	}
	var result WritingTemplate
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `UPDATE writing_templates SET title=$1,description=$2,content_markdown=$3,category=$4,version=version+1,updated_at=now()
			WHERE tenant_id=$5 AND id=$6 AND version=$7 AND deleted_at IS NULL RETURNING id,title,description,content_markdown,category,status,version,NULL::uuid,created_at,updated_at`,
			strings.TrimSpace(input.Title), input.Description, input.ContentMarkdown, input.Category, principal.TenantID, id, expected).
			Scan(&result.ID, &result.Title, &result.Description, &result.ContentMarkdown, &result.Category, &result.Status, &result.Version, &result.PublicID, &result.CreatedAt, &result.UpdatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("TEMPLATE_VERSION_CONFLICT", "模板已被修改或不存在", 409)
		}
		return err
	})
	return result, err
}

func (s *Store) DeleteWritingTemplate(ctx context.Context, principal domain.Principal, id int64) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx,
			`UPDATE writing_templates SET deleted_at=now(),status='withdrawn',updated_at=now() WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL`, principal.TenantID, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apierror.New("TEMPLATE_NOT_FOUND", "模板不存在", 404)
		}
		_, err = tx.Exec(ctx, `UPDATE published_template_snapshots SET status='withdrawn',withdrawn_at=now() WHERE source_publication_id IN(SELECT id FROM template_publications WHERE tenant_id=$1 AND template_id=$2) AND status='published'`, principal.TenantID, id)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE template_publications SET status='withdrawn',withdrawn_at=now() WHERE tenant_id=$1 AND template_id=$2 AND status='published'`, principal.TenantID, id); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type) SELECT $3,'template',public_template_id::text,'template.deleted' FROM published_template_snapshots p JOIN template_publications tp ON tp.id=p.source_publication_id WHERE tp.tenant_id=$1 AND tp.template_id=$2 ORDER BY p.id DESC LIMIT 1`, principal.TenantID, id, uuid.New())
		return err
	})
}

func (s *Store) PublishWritingTemplate(ctx context.Context, principal domain.Principal, id int64) (PublicTemplate, error) {
	var result PublicTemplate
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		var profileID uuid.UUID
		var nickname string
		if err := tx.QueryRow(ctx, `SELECT public_id,nickname FROM public_profiles WHERE tenant_id=$1`, principal.TenantID).Scan(&profileID, &nickname); errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("PUBLIC_PROFILE_REQUIRED", "公开前必须设置公开昵称", 409)
		} else if err != nil {
			return err
		}
		var t WritingTemplate
		if err := tx.QueryRow(ctx, `SELECT id,title,description,content_markdown,category,status,version,NULL::uuid,created_at,updated_at FROM writing_templates WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL`, principal.TenantID, id).Scan(&t.ID, &t.Title, &t.Description, &t.ContentMarkdown, &t.Category, &t.Status, &t.Version, &t.PublicID, &t.CreatedAt, &t.UpdatedAt); errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("TEMPLATE_NOT_FOUND", "模板不存在", 404)
		} else if err != nil {
			return err
		}
		digest := sha256.Sum256([]byte(t.ContentMarkdown))
		var publicationID int64
		if err := tx.QueryRow(ctx, `INSERT INTO template_publications(tenant_id,template_id,template_version,title,description,content_markdown,category,content_sha256)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(tenant_id,template_id,template_version) DO UPDATE SET status='published',withdrawn_at=NULL RETURNING id`, principal.TenantID, id, t.Version, t.Title, t.Description, t.ContentMarkdown, t.Category, hex.EncodeToString(digest[:])).Scan(&publicationID); err != nil {
			return err
		}
		var publicID uuid.UUID
		_ = tx.QueryRow(ctx, `SELECT public_template_id FROM published_template_snapshots p JOIN template_publications tp ON tp.id=p.source_publication_id WHERE tp.tenant_id=$1 AND tp.template_id=$2 ORDER BY p.id DESC LIMIT 1`, principal.TenantID, id).Scan(&publicID)
		if publicID == uuid.Nil {
			publicID = uuid.New()
		}
		if _, err := tx.Exec(ctx, `UPDATE published_template_snapshots SET status='withdrawn',withdrawn_at=now() WHERE public_template_id=$1 AND status='published'`, publicID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO published_template_snapshots(public_template_id,source_publication_id,author_public_id,author_nickname,version,title,description,content_markdown,category)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(public_template_id,version) DO UPDATE SET source_publication_id=EXCLUDED.source_publication_id,author_public_id=EXCLUDED.author_public_id,author_nickname=EXCLUDED.author_nickname,title=EXCLUDED.title,description=EXCLUDED.description,content_markdown=EXCLUDED.content_markdown,category=EXCLUDED.category,status='published',published_at=now(),withdrawn_at=NULL`, publicID, publicationID, profileID, nickname, t.Version, t.Title, t.Description, t.ContentMarkdown, t.Category); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT p.public_template_id,p.author_nickname,p.version,p.title,p.description,p.content_markdown,p.category,p.published_at,COALESCE(st.like_count,0),COALESCE(st.favorite_count,0),COALESCE(st.usage_count,0),false,false FROM published_template_snapshots p LEFT JOIN template_public_stats st ON st.public_template_id=p.public_template_id WHERE p.public_template_id=$1 AND p.status='published'`, publicID).Scan(&result.PublicID, &result.AuthorNickname, &result.Version, &result.Title, &result.Description, &result.ContentMarkdown, &result.Category, &result.PublishedAt, &result.LikeCount, &result.FavoriteCount, &result.UsageCount, &result.Liked, &result.Favorited); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE writing_templates SET status='published',updated_at=now() WHERE tenant_id=$1 AND id=$2`, principal.TenantID, id); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type,payload) VALUES($1,'template',$2::varchar,'template.published',jsonb_build_object('public_id',$2::varchar))`, uuid.New(), publicID.String())
		return err
	})
	return result, err
}

func (s *Store) WithdrawWritingTemplate(ctx context.Context, principal domain.Principal, id int64) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE writing_templates SET status='withdrawn',updated_at=now() WHERE tenant_id=$1 AND id=$2 AND status='published' AND deleted_at IS NULL`, principal.TenantID, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apierror.New("TEMPLATE_NOT_FOUND", "公开模板不存在", 404)
		}
		_, err = tx.Exec(ctx, `UPDATE published_template_snapshots SET status='withdrawn',withdrawn_at=now() WHERE source_publication_id IN(SELECT id FROM template_publications WHERE tenant_id=$1 AND template_id=$2) AND status='published'`, principal.TenantID, id)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE template_publications SET status='withdrawn',withdrawn_at=now() WHERE tenant_id=$1 AND template_id=$2 AND status='published'`, principal.TenantID, id); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type) SELECT $3,'template',public_template_id::text,'template.withdrawn' FROM published_template_snapshots p JOIN template_publications tp ON tp.id=p.source_publication_id WHERE tp.tenant_id=$1 AND tp.template_id=$2 ORDER BY p.id DESC LIMIT 1`, principal.TenantID, id, uuid.New())
		return err
	})
}

func (s *Store) ListPublicTemplates(ctx context.Context, principal domain.Principal, query, category, ranking string, limit int, afterScore *float64, afterID *uuid.UUID) ([]PublicTemplate, []float64, error) {
	var items []PublicTemplate
	var scores []float64
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		pattern := "%" + strings.TrimSpace(query) + "%"
		score := `floor(extract(epoch from p.published_at))::double precision`
		switch ranking {
		case "daily":
			score = `(SELECT count(*)::double precision FROM outbox_events oe WHERE oe.aggregate_type='template' AND oe.aggregate_id=p.public_template_id::text AND oe.event_type='template.viewed' AND (oe.occurred_at AT TIME ZONE 'Asia/Shanghai')::date=(now() AT TIME ZONE 'Asia/Shanghai')::date)`
		case "trending":
			score = `((COALESCE(st.view_count,0)+3*COALESCE(st.like_count,0)+5*COALESCE(st.favorite_count,0)+8*COALESCE(st.usage_count,0))/power(2.0,GREATEST(0,extract(epoch FROM now()-p.published_at))/604800.0))::double precision`
		case "recommended":
			score = `((COALESCE(st.like_count,0)+2*COALESCE(st.favorite_count,0)+3*COALESCE(st.usage_count,0)) + CASE WHEN COALESCE((SELECT marketplace_personalization FROM user_preferences WHERE tenant_id=$1 AND user_id=(SELECT user_id FROM tenants WHERE id=$1)),true) AND EXISTS(SELECT 1 FROM template_reactions rr JOIN published_template_snapshots pp ON pp.public_template_id=rr.public_template_id WHERE rr.tenant_id=$1 AND rr.kind IN ('like','favorite') AND pp.category=p.category) THEN 20 ELSE 0 END)::double precision`
		case "new":
		default:
			return apierror.Validation(nil)
		}
		sql := `SELECT p.public_template_id,p.author_nickname,p.version,p.title,p.description,p.content_markdown,p.category,p.published_at,
		COALESCE(st.like_count,0),COALESCE(st.favorite_count,0),COALESCE(st.usage_count,0),
		EXISTS(SELECT 1 FROM template_reactions r WHERE r.tenant_id=$1 AND r.public_template_id=p.public_template_id AND r.kind='like'),
		EXISTS(SELECT 1 FROM template_reactions r WHERE r.tenant_id=$1 AND r.public_template_id=p.public_template_id AND r.kind='favorite'),` + score + `
		FROM published_template_snapshots p LEFT JOIN template_public_stats st ON st.public_template_id=p.public_template_id
		WHERE p.status='published' AND ($2='' OR p.title ILIKE $3 OR p.description ILIKE $3) AND ($4='' OR p.category=$4)
		AND ($8<>'recommended' OR (
			NOT EXISTS(SELECT 1 FROM template_usages tu WHERE tu.tenant_id=$1 AND tu.user_id=$9 AND tu.public_template_id=p.public_template_id AND tu.created_at>=now()-interval '7 days')
		))
		AND ($5::double precision IS NULL OR ` + score + `<$5 OR (` + score + `=$5 AND p.public_template_id<$6))
		ORDER BY ` + score + ` DESC,p.public_template_id DESC LIMIT $7`
		rows, err := tx.Query(ctx, sql, principal.TenantID, strings.TrimSpace(query), pattern, category, afterScore, afterID, limit, ranking, principal.UserID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var x PublicTemplate
			var scoreValue float64
			if err := rows.Scan(&x.PublicID, &x.AuthorNickname, &x.Version, &x.Title, &x.Description, &x.ContentMarkdown, &x.Category, &x.PublishedAt, &x.LikeCount, &x.FavoriteCount, &x.UsageCount, &x.Liked, &x.Favorited, &scoreValue); err != nil {
				return err
			}
			items = append(items, x)
			scores = append(scores, scoreValue)
		}
		return rows.Err()
	})
	return items, scores, err
}

func (s *Store) GetPublicTemplate(ctx context.Context, principal domain.Principal, publicID uuid.UUID) (PublicTemplate, error) {
	var result PublicTemplate
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `SELECT p.public_template_id,p.author_nickname,p.version,p.title,p.description,p.content_markdown,p.category,p.published_at,COALESCE(st.like_count,0),COALESCE(st.favorite_count,0),COALESCE(st.usage_count,0),EXISTS(SELECT 1 FROM template_reactions r WHERE r.tenant_id=$1 AND r.public_template_id=p.public_template_id AND r.kind='like'),EXISTS(SELECT 1 FROM template_reactions r WHERE r.tenant_id=$1 AND r.public_template_id=p.public_template_id AND r.kind='favorite') FROM published_template_snapshots p LEFT JOIN template_public_stats st ON st.public_template_id=p.public_template_id WHERE p.public_template_id=$2 AND p.status='published'`, principal.TenantID, publicID).Scan(&result.PublicID, &result.AuthorNickname, &result.Version, &result.Title, &result.Description, &result.ContentMarkdown, &result.Category, &result.PublishedAt, &result.LikeCount, &result.FavoriteCount, &result.UsageCount, &result.Liked, &result.Favorited)
		if errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("PUBLIC_TEMPLATE_NOT_FOUND", "公开模板不存在", 404)
		}
		return err
	})
	return result, err
}

func (s *Store) GetPublicTemplateVersion(ctx context.Context, publicID uuid.UUID) (int, error) {
	var version int
	err := s.AdminPool.QueryRow(ctx, `SELECT version FROM published_template_snapshots WHERE public_template_id=$1 AND status='published'`, publicID).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, apierror.New("PUBLIC_TEMPLATE_NOT_FOUND", "公开模板不存在", 404)
	}
	return version, err
}

func (s *Store) GetPublicTemplatesByIDs(ctx context.Context, principal domain.Principal, ids []uuid.UUID) ([]PublicTemplate, error) {
	if len(ids) == 0 {
		return []PublicTemplate{}, nil
	}
	var items []PublicTemplate
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT p.public_template_id,p.author_nickname,p.version,p.title,p.description,p.content_markdown,p.category,p.published_at,COALESCE(st.like_count,0),COALESCE(st.favorite_count,0),COALESCE(st.usage_count,0),EXISTS(SELECT 1 FROM template_reactions r WHERE r.tenant_id=$1 AND r.public_template_id=p.public_template_id AND r.kind='like'),EXISTS(SELECT 1 FROM template_reactions r WHERE r.tenant_id=$1 AND r.public_template_id=p.public_template_id AND r.kind='favorite') FROM published_template_snapshots p LEFT JOIN template_public_stats st ON st.public_template_id=p.public_template_id WHERE p.status='published' AND p.public_template_id=ANY($2)`, principal.TenantID, ids)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var x PublicTemplate
			if err := rows.Scan(&x.PublicID, &x.AuthorNickname, &x.Version, &x.Title, &x.Description, &x.ContentMarkdown, &x.Category, &x.PublishedAt, &x.LikeCount, &x.FavoriteCount, &x.UsageCount, &x.Liked, &x.Favorited); err != nil {
				return err
			}
			items = append(items, x)
		}
		return rows.Err()
	})
	return items, err
}

func (s *Store) GetTemplateReactions(ctx context.Context, principal domain.Principal, publicID uuid.UUID) (bool, bool, error) {
	var liked, favorited bool
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM template_reactions WHERE tenant_id=$1 AND user_id=$2 AND public_template_id=$3 AND kind='like'),EXISTS(SELECT 1 FROM template_reactions WHERE tenant_id=$1 AND user_id=$2 AND public_template_id=$3 AND kind='favorite')`, principal.TenantID, principal.UserID, publicID).Scan(&liked, &favorited)
	})
	return liked, favorited, err
}

func (s *Store) SetTemplateReaction(ctx context.Context, principal domain.Principal, publicID uuid.UUID, kind string, enabled bool) error {
	if kind != "like" && kind != "favorite" {
		return apierror.Validation(nil)
	}
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM published_template_snapshots WHERE public_template_id=$1 AND status='published')`, publicID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return apierror.New("PUBLIC_TEMPLATE_NOT_FOUND", "公开模板不存在", 404)
		}
		delta := int64(0)
		if enabled {
			tag, err := tx.Exec(ctx, `INSERT INTO template_reactions(tenant_id,user_id,public_template_id,kind) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, principal.TenantID, principal.UserID, publicID, kind)
			if err != nil {
				return err
			}
			delta = tag.RowsAffected()
		} else {
			tag, err := tx.Exec(ctx, `DELETE FROM template_reactions WHERE tenant_id=$1 AND user_id=$2 AND public_template_id=$3 AND kind=$4`, principal.TenantID, principal.UserID, publicID, kind)
			if err != nil {
				return err
			}
			delta = -tag.RowsAffected()
		}
		column := "like_count"
		if kind == "favorite" {
			column = "favorite_count"
		}
		_, err := tx.Exec(ctx, `INSERT INTO template_public_stats(public_template_id) VALUES($1) ON CONFLICT DO NOTHING`, publicID)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE template_public_stats SET `+column+`=GREATEST(0,`+column+`+$2),updated_at=now() WHERE public_template_id=$1`, publicID, delta); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type,payload) VALUES($1,'template',$2,$3,jsonb_build_object('delta',$4::bigint))`, uuid.New(), publicID.String(), "template."+kind, delta)
		return err
	})
}

func (s *Store) UsePublicTemplate(ctx context.Context, principal domain.Principal, publicID uuid.UUID, idempotencyKey string) (int32, error) {
	if strings.TrimSpace(idempotencyKey) == "" || len(idempotencyKey) > 128 {
		return 0, apierror.Validation(nil)
	}
	var noteID int32
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT note_id FROM template_usages WHERE tenant_id=$1 AND idempotency_key=$2`, principal.TenantID, idempotencyKey).Scan(&noteID); err == nil {
			return nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var title, content string
		var version int
		if err := tx.QueryRow(ctx, `SELECT title,content_markdown,version FROM published_template_snapshots WHERE public_template_id=$1 AND status='published'`, publicID).Scan(&title, &content, &version); errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("PUBLIC_TEMPLATE_NOT_FOUND", "公开模板不存在", 404)
		} else if err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO notes(tenant_id,created_by,updated_by,type,title,content,word_count) VALUES($1,$2,$2,'normal',$3,$4,$5) RETURNING id`, principal.TenantID, principal.UserID, title, content, wordCount(content)).Scan(&noteID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO template_usages(tenant_id,user_id,public_template_id,snapshot_version,note_id,idempotency_key) VALUES($1,$2,$3,$4,$5,$6)`, principal.TenantID, principal.UserID, publicID, version, noteID, idempotencyKey); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO template_public_stats(public_template_id,usage_count) VALUES($1,1) ON CONFLICT(public_template_id) DO UPDATE SET usage_count=template_public_stats.usage_count+1,updated_at=now()`, publicID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type,payload) VALUES($1,'template',$2,'template.used',jsonb_build_object('note_id',$3::int))`, uuid.New(), publicID.String(), noteID); err != nil {
			return err
		}
		return auditResource(ctx, tx, principal, "template.use", "note", strconv.FormatInt(int64(noteID), 10))
	})
	return noteID, err
}

func (s *Store) UseWritingTemplate(ctx context.Context, principal domain.Principal, templateID int64, idempotencyKey string) (int32, error) {
	if strings.TrimSpace(idempotencyKey) == "" || len(idempotencyKey) > 128 {
		return 0, apierror.Validation(nil)
	}
	var noteID int32
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT note_id FROM template_usages WHERE tenant_id=$1 AND idempotency_key=$2`, principal.TenantID, idempotencyKey).Scan(&noteID); err == nil {
			return nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var title, content string
		var version int
		if err := tx.QueryRow(ctx, `SELECT title,content_markdown,version FROM writing_templates WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL`, principal.TenantID, templateID).Scan(&title, &content, &version); errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("TEMPLATE_NOT_FOUND", "模板不存在", 404)
		} else if err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO notes(tenant_id,created_by,updated_by,type,title,content,word_count) VALUES($1,$2,$2,'normal',$3,$4,$5) RETURNING id`, principal.TenantID, principal.UserID, title, content, wordCount(content)).Scan(&noteID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO template_usages(tenant_id,user_id,source_template_id,snapshot_version,note_id,idempotency_key) VALUES($1,$2,$3,$4,$5,$6)`, principal.TenantID, principal.UserID, templateID, version, noteID, idempotencyKey); err != nil {
			return err
		}
		return auditResource(ctx, tx, principal, "template.use_private", "note", strconv.FormatInt(int64(noteID), 10))
	})
	return noteID, err
}

func (s *Store) RecordTemplateView(ctx context.Context, principal domain.Principal, publicID uuid.UUID) error {
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM published_template_snapshots WHERE public_template_id=$1 AND status='published')`, publicID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return apierror.New("PUBLIC_TEMPLATE_NOT_FOUND", "公开模板不存在", 404)
		}
		_, err := tx.Exec(ctx, `INSERT INTO template_public_stats(public_template_id,view_count) VALUES($1,1) ON CONFLICT(public_template_id) DO UPDATE SET view_count=template_public_stats.view_count+1,updated_at=now()`, publicID)
		if err != nil {
			return err
		}
		visitorDigest := sha256.Sum256([]byte(principal.TenantID.String() + ":" + strconv.FormatInt(int64(principal.UserID), 10)))
		_, err = tx.Exec(ctx, `INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type,payload) VALUES($1,'template',$2,'template.viewed',jsonb_build_object('delta',1,'visitor',$3::text))`, uuid.New(), publicID.String(), hex.EncodeToString(visitorDigest[:8]))
		return err
	})
}
