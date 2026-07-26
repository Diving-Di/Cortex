package store

import (
    "context"
    "time"

    "diary-listener/backend/internal/apierror"
    "diary-listener/backend/internal/domain"
    "github.com/jackc/pgx/v5"
)

type BackupNote struct {
    ID       int32   `json:"id"`
    Type     string  `json:"type"`
    Title    string  `json:"title"`
    Content  string  `json:"content"`
    NoteDate *string `json:"note_date"`
    Summary  *string `json:"summary"`
}

type BackupTag struct {
    ID    int32   `json:"id"`
    Name  string  `json:"name"`
    Color *string `json:"color"`
}

type BackupNoteTag struct {
    NoteID int32 `json:"note_id"`
    TagID  int32 `json:"tag_id"`
}

type BackupAttachment struct {
    ID      int32  `json:"id"`
    NoteID  int32  `json:"note_id"`
    Name    string `json:"name"`
    MIME    string `json:"mime"`
    SHA256  string `json:"sha256"`
    Archive string `json:"archive"`
}

type BackupData struct {
    Version     int                `json:"version"`
    Notes       []BackupNote       `json:"notes"`
    Tags        []BackupTag        `json:"tags"`
    NoteTags    []BackupNoteTag    `json:"note_tags"`
    Attachments []BackupAttachment `json:"attachments"`
}

type RestoredAttachment struct {
    SourceNoteID int32
    Name         string
    StoredPath   string
    MIMEType     string
    Size         int64
    SHA256       string
}

func (s *Store) BackupSnapshot(ctx context.Context, principal domain.Principal) (BackupData, []Attachment, error) {
    data := BackupData{Version: 1}
    var attachments []Attachment
    err := s.WithTx(ctx, func(tx pgx.Tx) error {
        if err := setTenant(ctx, tx, principal); err != nil {
            return err
        }
        noteRows, err := tx.Query(ctx, `SELECT id,type,title,content,note_date,summary FROM notes
            WHERE tenant_id=$1 AND deleted_at IS NULL ORDER BY id`, principal.TenantID)
        if err != nil {
            return err
        }
        for noteRows.Next() {
            var item BackupNote
            var noteDate *time.Time
            if err := noteRows.Scan(&item.ID, &item.Type, &item.Title, &item.Content, &noteDate, &item.Summary); err != nil {
                noteRows.Close()
                return err
            }
            if noteDate != nil {
                value := noteDate.Format(time.DateOnly)
                item.NoteDate = &value
            }
            data.Notes = append(data.Notes, item)
        }
        if err := noteRows.Err(); err != nil {
            noteRows.Close()
            return err
        }
        noteRows.Close()

        tagRows, err := tx.Query(ctx, `SELECT id,name,color FROM tags WHERE tenant_id=$1 ORDER BY id`, principal.TenantID)
        if err != nil {
            return err
        }
        for tagRows.Next() {
            var item BackupTag
            if err := tagRows.Scan(&item.ID, &item.Name, &item.Color); err != nil {
                tagRows.Close()
                return err
            }
            data.Tags = append(data.Tags, item)
        }
        if err := tagRows.Err(); err != nil {
            tagRows.Close()
            return err
        }
        tagRows.Close()

        linkRows, err := tx.Query(ctx, `SELECT note_id,tag_id FROM note_tags WHERE tenant_id=$1`, principal.TenantID)
        if err != nil {
            return err
        }
        for linkRows.Next() {
            var item BackupNoteTag
            if err := linkRows.Scan(&item.NoteID, &item.TagID); err != nil {
                linkRows.Close()
                return err
            }
            data.NoteTags = append(data.NoteTags, item)
        }
        if err := linkRows.Err(); err != nil {
            linkRows.Close()
            return err
        }
        linkRows.Close()

        attachmentRows, err := tx.Query(ctx, `SELECT id,note_id,original_name,stored_path,mime_type,size,sha256,created_at
            FROM attachments WHERE tenant_id=$1 ORDER BY id`, principal.TenantID)
        if err != nil {
            return err
        }
        defer attachmentRows.Close()
        for attachmentRows.Next() {
            var item Attachment
            if err := attachmentRows.Scan(
                &item.ID, &item.NoteID, &item.OriginalName, &item.StoredPath,
                &item.MIMEType, &item.Size, &item.SHA256, &item.CreatedAt,
            ); err != nil {
                return err
            }
            attachments = append(attachments, item)
        }
        return attachmentRows.Err()
    })
    return data, attachments, err
}

func (s *Store) ExportNotes(ctx context.Context, principal domain.Principal) ([]BackupNote, error) {
    data, _, err := s.BackupSnapshot(ctx, principal)
    return data.Notes, err
}

func (s *Store) RestoreSnapshot(
    ctx context.Context,
    principal domain.Principal,
    data BackupData,
    attachments []RestoredAttachment,
) (map[string]int, error) {
    result := map[string]int{}
    err := s.WithTx(ctx, func(tx pgx.Tx) error {
        if err := setTenant(ctx, tx, principal); err != nil {
            return err
        }
        var count int
        if err := tx.QueryRow(ctx, `SELECT count(*) FROM notes WHERE tenant_id=$1 AND deleted_at IS NULL`, principal.TenantID).Scan(&count); err != nil {
            return err
        }
        if count != 0 {
            return apierror.New("RESTORE_TARGET_NOT_EMPTY", "恢复目标必须为空", 409)
        }
        var noteQuota, attachmentQuota int64
        if err := tx.QueryRow(ctx, `SELECT note_quota,attachment_quota_bytes FROM tenants WHERE id=$1 FOR UPDATE`, principal.TenantID).Scan(&noteQuota, &attachmentQuota); err != nil {
            return err
        }
        if int64(len(data.Notes)) > noteQuota {
            return apierror.New("NOTE_QUOTA_EXCEEDED", "笔记数量已达到配额", 409)
        }
        var attachmentBytes int64
        for _, item := range attachments {
            attachmentBytes += item.Size
        }
        if attachmentBytes > attachmentQuota {
            return apierror.New("ATTACHMENT_QUOTA_EXCEEDED", "附件空间配额不足", 409)
        }
        noteMap := make(map[int32]int32, len(data.Notes))
        for _, source := range data.Notes {
            var noteDate *time.Time
            if source.NoteDate != nil {
                parsed, err := time.Parse(time.DateOnly, *source.NoteDate)
                if err != nil {
                    return apierror.New("INVALID_BACKUP", "备份笔记日期无效", 422)
                }
                noteDate = &parsed
            }
            var id int32
            if err := tx.QueryRow(ctx, `INSERT INTO notes
                (tenant_id,created_by,updated_by,type,title,content,note_date,summary,word_count)
                VALUES ($1,$2,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
                principal.TenantID, principal.UserID, source.Type, source.Title,
                source.Content, noteDate, source.Summary, wordCount(source.Content),
            ).Scan(&id); err != nil {
                return err
            }
            noteMap[source.ID] = id
        }
        tagMap := make(map[int32]int32, len(data.Tags))
        for _, source := range data.Tags {
            var id int32
            if err := tx.QueryRow(ctx, `INSERT INTO tags (tenant_id,name,color) VALUES ($1,$2,$3) RETURNING id`,
                principal.TenantID, source.Name, source.Color,
            ).Scan(&id); err != nil {
                return err
            }
            tagMap[source.ID] = id
        }
        for _, link := range data.NoteTags {
            noteID, noteOK := noteMap[link.NoteID]
            tagID, tagOK := tagMap[link.TagID]
            if !noteOK || !tagOK {
                return apierror.New("INVALID_BACKUP", "备份标签关系无效", 422)
            }
            if _, err := tx.Exec(ctx, `INSERT INTO note_tags (tenant_id,note_id,tag_id) VALUES ($1,$2,$3)`, principal.TenantID, noteID, tagID); err != nil {
                return err
            }
        }
        for _, source := range attachments {
            noteID, ok := noteMap[source.SourceNoteID]
            if !ok {
                return apierror.New("INVALID_BACKUP", "备份附件引用无效", 422)
            }
            if _, err := tx.Exec(ctx, `INSERT INTO attachments
                (tenant_id,uploaded_by,note_id,original_name,stored_path,mime_type,size,sha256)
                VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
                principal.TenantID, principal.UserID, noteID, source.Name, source.StoredPath,
                source.MIMEType, source.Size, source.SHA256,
            ); err != nil {
                return err
            }
        }
        result = map[string]int{
            "notes": len(noteMap), "tags": len(tagMap), "attachments": len(attachments),
        }
        return nil
    })
    return result, err
}
