package store

import (
    "context"
    "errors"
    "fmt"
    "regexp"
    "strings"
    "time"
    "unicode"

    "diary-listener/backend/internal/apierror"
    "diary-listener/backend/internal/domain"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgconn"
)

type SourceNote struct {
    ID       int32   `json:"id"`
    Title    string  `json:"title"`
    NoteDate *string `json:"note_date,omitempty"`
    Snippet  string  `json:"snippet"`
}

func (s *Store) ConfirmOrganize(
    ctx context.Context,
    principal domain.Principal,
    noteID *int32,
    title string,
    content string,
    summary *string,
) (map[string]any, error) {
    result := make(map[string]any)
    err := s.WithTx(ctx, func(tx pgx.Tx) error {
        if err := setTenant(ctx, tx, principal); err != nil {
            return err
        }
        var id int32
        if noteID != nil {
            note, err := getNoteTx(ctx, tx, principal, *noteID)
            if err != nil {
                return err
            }
            if _, err := tx.Exec(ctx, `INSERT INTO note_revisions
                (tenant_id,note_id,created_by,content,reason) VALUES ($1,$2,$3,$4,'ai_before_apply')`,
                principal.TenantID, note.ID, principal.UserID, note.Content,
            ); err != nil {
                return err
            }
            if err := tx.QueryRow(ctx, `UPDATE notes SET title=$1,content=$2,summary=$3,
                word_count=$4,updated_by=$5,updated_at=now()
                WHERE tenant_id=$6 AND id=$7 RETURNING id`,
                title, content, summary, wordCount(content), principal.UserID, principal.TenantID, note.ID,
            ).Scan(&id); err != nil {
                return err
            }
        } else {
            if err := tx.QueryRow(ctx, `INSERT INTO notes
                (tenant_id,created_by,updated_by,type,title,content,summary,word_count)
                VALUES ($1,$2,$2,'normal',$3,$4,$5,$6) RETURNING id`,
                principal.TenantID, principal.UserID, title, content, summary, wordCount(content),
            ).Scan(&id); err != nil {
                return err
            }
        }
        result = map[string]any{"id": id, "title": title, "content": content}
        return nil
    })
    return result, err
}

func (s *Store) ReportSources(
    ctx context.Context,
    principal domain.Principal,
    kind string,
    anchor time.Time,
) (time.Time, time.Time, []SourceNote, error) {
    start, end := periodRange(kind, anchor)
    var sources []SourceNote
    err := s.WithTx(ctx, func(tx pgx.Tx) error {
        if err := setTenant(ctx, tx, principal); err != nil {
            return err
        }
        var err error
        sources, err = reportSourcesTx(ctx, tx, principal, kind, start, end)
        return err
    })
    return start, end, sources, err
}

func reportSourcesTx(
    ctx context.Context,
    tx pgx.Tx,
    principal domain.Principal,
    kind string,
    start time.Time,
    end time.Time,
) ([]SourceNote, error) {
    rows, err := tx.Query(ctx, `SELECT id,title,note_date,content FROM notes
        WHERE tenant_id=$1 AND deleted_at IS NULL AND note_date >= $2 AND note_date <= $3 AND type<>$4
        ORDER BY note_date,id LIMIT 100`, principal.TenantID, start, end, kind,
    )
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var sources []SourceNote
    for rows.Next() {
        var item SourceNote
        var date *time.Time
        var content string
        if err := rows.Scan(&item.ID, &item.Title, &date, &content); err != nil {
            return nil, err
        }
        if date != nil {
            value := date.Format(time.DateOnly)
            item.NoteDate = &value
        }
        item.Snippet = snippet(content, 2000)
        sources = append(sources, item)
    }
    return sources, rows.Err()
}

func (s *Store) ConfirmReport(
    ctx context.Context,
    principal domain.Principal,
    kind string,
    anchor time.Time,
    title string,
    content string,
    sourceIDs []int32,
    overwrite bool,
) (map[string]any, error) {
    result := make(map[string]any)
    start, end := periodRange(kind, anchor)
    err := s.WithTx(ctx, func(tx pgx.Tx) error {
        if err := setTenant(ctx, tx, principal); err != nil {
            return err
        }
        allowedSources, err := reportSourcesTx(ctx, tx, principal, kind, start, end)
        if err != nil {
            return err
        }
        allowed := make(map[int32]bool, len(allowedSources))
        for _, item := range allowedSources {
            allowed[item.ID] = true
        }
        if len(sourceIDs) == 0 {
            return apierror.New("INVALID_REPORT_SOURCES", "报告来源为空或不属于所选周期", 422)
        }
        for _, id := range sourceIDs {
            if !allowed[id] {
                return apierror.New("INVALID_REPORT_SOURCES", "报告来源为空或不属于所选周期", 422)
            }
        }
        var noteID int32
        var previousContent string
        err = tx.QueryRow(ctx, `SELECT id,content FROM notes WHERE tenant_id=$1 AND type=$2
            AND note_date=$3 AND deleted_at IS NULL`, principal.TenantID, kind, start,
        ).Scan(&noteID, &previousContent)
        if err == nil && !overwrite {
            return apierror.New("REPORT_EXISTS", "该周期报告已存在，请明确选择覆盖", 409)
        }
        if err != nil && !errors.Is(err, pgx.ErrNoRows) {
            return err
        }
        if err == nil {
            if _, err := tx.Exec(ctx, `INSERT INTO note_revisions
                (tenant_id,note_id,created_by,content,reason)
                VALUES ($1,$2,$3,$4,'report_before_overwrite')`,
                principal.TenantID, noteID, principal.UserID, previousContent,
            ); err != nil {
                return err
            }
            if _, err := tx.Exec(ctx, `UPDATE notes SET title=$1,content=$2,word_count=$3,
                updated_by=$4,updated_at=now() WHERE tenant_id=$5 AND id=$6`,
                title, content, wordCount(content), principal.UserID, principal.TenantID, noteID,
            ); err != nil {
                return err
            }
            if _, err := tx.Exec(ctx, `DELETE FROM report_sources WHERE tenant_id=$1 AND report_note_id=$2`, principal.TenantID, noteID); err != nil {
                return err
            }
        } else {
            err = tx.QueryRow(ctx, `INSERT INTO notes
                (tenant_id,created_by,updated_by,type,title,content,note_date,word_count)
                VALUES ($1,$2,$2,$3,$4,$5,$6,$7) RETURNING id`,
                principal.TenantID, principal.UserID, kind, title, content, start, wordCount(content),
            ).Scan(&noteID)
            if err != nil {
                var pgErr *pgconn.PgError
                if errors.As(err, &pgErr) && pgErr.Code == "23505" {
                    return apierror.New("REPORT_EXISTS", "该周期报告已存在", 409)
                }
                return err
            }
        }
        for index, sourceID := range sourceIDs {
            if _, err := tx.Exec(ctx, `INSERT INTO report_sources
                (tenant_id,report_note_id,source_note_id,rank) VALUES ($1,$2,$3,$4)`,
                principal.TenantID, noteID, sourceID, index+1,
            ); err != nil {
                return err
            }
        }
        result = map[string]any{"id": noteID, "source_ids": sourceIDs}
        return nil
    })
    return result, err
}

func (s *Store) GetReportSources(ctx context.Context, principal domain.Principal, noteID int32) ([]SourceNote, error) {
    var sources []SourceNote
    err := s.WithTx(ctx, func(tx pgx.Tx) error {
        if err := setTenant(ctx, tx, principal); err != nil {
            return err
        }
        if _, err := getNoteTx(ctx, tx, principal, noteID); err != nil {
            return err
        }
        rows, err := tx.Query(ctx, `SELECT n.id,n.title,n.note_date,n.content FROM report_sources r
            JOIN notes n ON n.id=r.source_note_id AND n.tenant_id=r.tenant_id
            WHERE r.tenant_id=$1 AND r.report_note_id=$2 ORDER BY r.rank`,
            principal.TenantID, noteID,
        )
        if err != nil {
            return err
        }
        defer rows.Close()
        for rows.Next() {
            var item SourceNote
            var date *time.Time
            var content string
            if err := rows.Scan(&item.ID, &item.Title, &date, &content); err != nil {
                return err
            }
            if date != nil {
                value := date.Format(time.DateOnly)
                item.NoteDate = &value
            }
            item.Snippet = snippet(content, 160)
            sources = append(sources, item)
        }
        return rows.Err()
    })
    return sources, err
}

func (s *Store) MemoryCandidates(ctx context.Context, principal domain.Principal, question string, limit int) ([]SourceNote, error) {
    words := memoryWords(question)
    var sources []SourceNote
    err := s.WithTx(ctx, func(tx pgx.Tx) error {
        if err := setTenant(ctx, tx, principal); err != nil {
            return err
        }
        args := []any{principal.TenantID}
        where := []string{"tenant_id=$1", "deleted_at IS NULL"}
        if len(words) > 0 {
            clauses := make([]string, 0, len(words))
            for _, word := range words {
                args = append(args, "%"+word+"%")
                index := len(args)
                clauses = append(clauses, fmt.Sprintf("(title ILIKE $%d OR content ILIKE $%d OR summary ILIKE $%d)", index, index, index))
            }
            where = append(where, "("+strings.Join(clauses, " OR ")+")")
        }
        args = append(args, limit)
        rows, err := tx.Query(ctx, `SELECT id,title,note_date,content FROM notes WHERE `+
            strings.Join(where, " AND ")+fmt.Sprintf(" ORDER BY note_date DESC NULLS LAST LIMIT $%d", len(args)),
            args...,
        )
        if err != nil {
            return err
        }
        defer rows.Close()
        for rows.Next() {
            var item SourceNote
            var date *time.Time
            var content string
            if err := rows.Scan(&item.ID, &item.Title, &date, &content); err != nil {
                return err
            }
            if date != nil {
                value := date.Format(time.DateOnly)
                item.NoteDate = &value
            }
            item.Snippet = snippet(content, 1200)
            sources = append(sources, item)
        }
        return rows.Err()
    })
    return sources, err
}

func (s *Store) SaveMemoryAnswer(
    ctx context.Context,
    principal domain.Principal,
    conversationID *int32,
    question string,
    answer string,
    sources []SourceNote,
) error {
    return s.WithTx(ctx, func(tx pgx.Tx) error {
        if err := setTenant(ctx, tx, principal); err != nil {
            return err
        }
        var id int32
        if conversationID != nil {
            err := tx.QueryRow(ctx, `SELECT id FROM conversations WHERE tenant_id=$1 AND user_id=$2 AND id=$3`,
                principal.TenantID, principal.UserID, *conversationID,
            ).Scan(&id)
            if err != nil && !errors.Is(err, pgx.ErrNoRows) {
                return err
            }
        }
        if id == 0 {
            title := truncateText(question, 80)
            if err := tx.QueryRow(ctx, `INSERT INTO conversations (tenant_id,user_id,title)
                VALUES ($1,$2,$3) RETURNING id`, principal.TenantID, principal.UserID, title,
            ).Scan(&id); err != nil {
                return err
            }
        }
        if _, err := tx.Exec(ctx, `INSERT INTO messages (tenant_id,conversation_id,role,content)
            VALUES ($1,$2,'user',$3)`, principal.TenantID, id, question); err != nil {
            return err
        }
        var messageID int32
        if err := tx.QueryRow(ctx, `INSERT INTO messages (tenant_id,conversation_id,role,content)
            VALUES ($1,$2,'assistant',$3) RETURNING id`, principal.TenantID, id, answer,
        ).Scan(&messageID); err != nil {
            return err
        }
        for index, source := range sources {
            if _, err := tx.Exec(ctx, `INSERT INTO message_sources
                (tenant_id,message_id,note_id,snippet,relevance,rank) VALUES ($1,$2,$3,$4,$5,$6)`,
                principal.TenantID, messageID, source.ID, snippet(source.Snippet, 500),
                len(sources)-index, index+1,
            ); err != nil {
                return err
            }
        }
        return nil
    })
}

func (s *Store) GetMemorySources(ctx context.Context, principal domain.Principal, messageID int32) ([]map[string]any, error) {
    var result []map[string]any
    err := s.WithTx(ctx, func(tx pgx.Tx) error {
        if err := setTenant(ctx, tx, principal); err != nil {
            return err
        }
        rows, err := tx.Query(ctx, `SELECT n.id,n.title,s.snippet,s.rank FROM message_sources s
            JOIN notes n ON n.id=s.note_id AND n.tenant_id=s.tenant_id
            WHERE s.tenant_id=$1 AND s.message_id=$2 ORDER BY s.rank`, principal.TenantID, messageID,
        )
        if err != nil {
            return err
        }
        defer rows.Close()
        for rows.Next() {
            var id, rank int32
            var title, text string
            if err := rows.Scan(&id, &title, &text, &rank); err != nil {
                return err
            }
            result = append(result, map[string]any{"id": id, "title": title, "snippet": text, "rank": rank})
        }
        return rows.Err()
    })
    return result, err
}

func snippet(value string, limit int) string {
    clean := strings.Join(strings.Fields(value), " ")
    return truncateText(clean, limit)
}

func truncateText(value string, limit int) string {
    runes := []rune(value)
    if len(runes) > limit {
        return string(runes[:limit])
    }
    return value
}

var memoryCleanup = regexp.MustCompile(`[？?，,。]`)

func memoryWords(question string) []string {
    cleaned := question
    for _, phrase := range []string{
        "今天", "昨天", "本周", "上周", "做了什么", "发生了什么", "什么",
        "有什么", "有哪些", "记录", "请问", "告诉我", "回忆一下",
    } {
        cleaned = strings.ReplaceAll(cleaned, phrase, " ")
    }
    cleaned = memoryCleanup.ReplaceAllString(cleaned, " ")
    unique := make(map[string]bool)
    var result []string
    for _, field := range strings.FieldsFunc(cleaned, func(r rune) bool {
        return unicode.IsSpace(r) || strings.ContainsRune("我的了在是有和与吗呢", r)
    }) {
        runes := []rune(field)
        if len(runes) < 2 {
            continue
        }
        if !unique[field] {
            result = append(result, field)
            unique[field] = true
        }
        if len(runes) >= 3 {
            for index := 0; index < len(runes)-1; index++ {
                word := string(runes[index : index+2])
                if !unique[word] {
                    result = append(result, word)
                    unique[word] = true
                }
            }
        }
        if len(result) >= 8 {
            break
        }
    }
    if len(result) > 8 {
        result = result[:8]
    }
    return result
}
