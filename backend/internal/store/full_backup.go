package store

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"diary-listener/backend/internal/apierror"
	"diary-listener/backend/internal/domain"
	"github.com/jackc/pgx/v5"
)

const FullBackupFormat = "cortex-full-backup-v1"

// FullBackup is a logical, tenant-scoped backup. Authentication material, AI
// provider configuration, usage records, audit logs and XHS sessions are
// deliberately absent.
type FullBackup struct {
	Format     string                              `json:"format"`
	ExportedAt time.Time                           `json:"exported_at"`
	Tables     map[string][]map[string]interface{} `json:"tables"`
}

type backupTableSpec struct {
	name       string
	id         string
	userFields []string
	foreign    map[string]string
	withoutID  bool
}

var backupTableSpecs = []backupTableSpec{
	{name: "tags", id: "id"},
	{name: "notes", id: "id", userFields: []string{"created_by", "updated_by"}},
	{name: "note_revisions", id: "id", userFields: []string{"created_by"}, foreign: map[string]string{"note_id": "notes"}},
	{name: "note_tags", withoutID: true, foreign: map[string]string{"note_id": "notes", "tag_id": "tags"}},
	{name: "attachments", id: "id", userFields: []string{"uploaded_by"}, foreign: map[string]string{"note_id": "notes"}},
	{name: "report_sources", id: "id", foreign: map[string]string{"report_note_id": "notes", "source_note_id": "notes"}},
	{name: "conversations", id: "id", userFields: []string{"user_id"}},
	{name: "messages", id: "id", foreign: map[string]string{"conversation_id": "conversations"}},
	{name: "message_sources", id: "id", foreign: map[string]string{"message_id": "messages", "note_id": "notes"}},
	{name: "recipe_message_sources", id: "id", foreign: map[string]string{"message_id": "messages"}},
	{name: "user_preferences", withoutID: true, userFields: []string{"user_id"}},
	{name: "research_jobs", id: "id", userFields: []string{"created_by"}},
	{name: "research_sources", id: "id", foreign: map[string]string{"job_id": "research_jobs"}},
	{name: "research_drafts", id: "id", foreign: map[string]string{"source_id": "research_sources"}},
	{name: "research_draft_revisions", id: "id", userFields: []string{"created_by"}, foreign: map[string]string{"draft_id": "research_drafts"}},
	{name: "research_assets", id: "id", foreign: map[string]string{"source_id": "research_sources"}},
	{name: "scheduled_report_tasks", id: "id", userFields: []string{"created_by"}},
	{name: "scheduled_report_runs", id: "id", foreign: map[string]string{"task_id": "scheduled_report_tasks", "report_note_id": "notes"}},
}

func (s *Store) ExportFullBackup(ctx context.Context, principal domain.Principal) (FullBackup, error) {
	result := FullBackup{
		Format: FullBackupFormat, ExportedAt: time.Now().UTC(),
		Tables: make(map[string][]map[string]interface{}, len(backupTableSpecs)),
	}
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		for _, spec := range backupTableSpecs {
			order := "1"
			if !spec.withoutID {
				order = pgx.Identifier{spec.id}.Sanitize()
			}
			filter := "t.tenant_id=$1"
			switch spec.name {
			case "research_sources":
				filter += " AND t.deleted_at IS NULL"
			case "research_drafts":
				filter += " AND EXISTS(SELECT 1 FROM research_sources s WHERE s.tenant_id=t.tenant_id AND s.id=t.source_id AND s.deleted_at IS NULL)"
			case "research_draft_revisions":
				filter += " AND EXISTS(SELECT 1 FROM research_drafts d JOIN research_sources s ON s.tenant_id=d.tenant_id AND s.id=d.source_id WHERE d.tenant_id=t.tenant_id AND d.id=t.draft_id AND s.deleted_at IS NULL)"
			case "research_assets":
				filter += " AND EXISTS(SELECT 1 FROM research_sources s WHERE s.tenant_id=t.tenant_id AND s.id=t.source_id AND s.deleted_at IS NULL)"
			}
			query := fmt.Sprintf(
				"SELECT COALESCE(jsonb_agg(to_jsonb(t) ORDER BY %s), '[]'::jsonb) FROM %s t WHERE %s",
				order, pgx.Identifier{spec.name}.Sanitize(), filter,
			)
			var encoded []byte
			if err := tx.QueryRow(ctx, query, principal.TenantID).Scan(&encoded); err != nil {
				return fmt.Errorf("export %s: %w", spec.name, err)
			}
			decoder := json.NewDecoder(strings.NewReader(string(encoded)))
			decoder.UseNumber()
			var rows []map[string]interface{}
			if err := decoder.Decode(&rows); err != nil {
				return fmt.Errorf("decode %s: %w", spec.name, err)
			}
			for _, row := range rows {
				delete(row, "tenant_id")
				for _, field := range spec.userFields {
					delete(row, field)
				}
			}
			result.Tables[spec.name] = rows
		}
		return nil
	})
	return result, err
}

func (s *Store) RestoreFullBackup(ctx context.Context, principal domain.Principal, backup FullBackup) error {
	if backup.Format != FullBackupFormat {
		return apierror.New("BACKUP_FORMAT_UNSUPPORTED", "备份格式不受支持", 422)
	}
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		var occupied bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(
			SELECT 1 FROM notes WHERE tenant_id=$1
			UNION ALL SELECT 1 FROM tags WHERE tenant_id=$1
			UNION ALL SELECT 1 FROM attachments WHERE tenant_id=$1
			UNION ALL SELECT 1 FROM conversations WHERE tenant_id=$1
			UNION ALL SELECT 1 FROM user_preferences WHERE tenant_id=$1
			UNION ALL SELECT 1 FROM research_jobs WHERE tenant_id=$1
			UNION ALL SELECT 1 FROM research_sources WHERE tenant_id=$1
			UNION ALL SELECT 1 FROM scheduled_report_tasks WHERE tenant_id=$1
		)`, principal.TenantID).Scan(&occupied); err != nil {
			return err
		}
		if occupied {
			return apierror.New("BACKUP_RESTORE_TENANT_NOT_EMPTY", "仅允许恢复到空的个人空间", 409)
		}
		var noteQuota int
		var attachmentQuota int64
		if err := tx.QueryRow(ctx, `SELECT note_quota,attachment_quota_bytes
			FROM tenants WHERE id=$1`, principal.TenantID).
			Scan(&noteQuota, &attachmentQuota); err != nil {
			return err
		}
		if len(backup.Tables["notes"]) > noteQuota ||
			backupTableBytes(backup.Tables["attachments"], "size") > attachmentQuota {
			return apierror.New("BACKUP_RESTORE_QUOTA_EXCEEDED", "备份内容超过目标空间配额", 409)
		}

		idMaps := make(map[string]map[string]interface{}, len(backupTableSpecs))
		for _, spec := range backupTableSpecs {
			idMaps[spec.name] = make(map[string]interface{})
			for _, original := range backup.Tables[spec.name] {
				row := cloneBackupRow(original)
				oldID := backupIDKey(row[spec.id])
				delete(row, "tenant_id")
				row["tenant_id"] = principal.TenantID
				for _, field := range spec.userFields {
					row[field] = principal.UserID
				}
				if spec.name == "research_jobs" {
					delete(row, "target_collection_id")
				}
				if spec.name == "research_drafts" {
					delete(row, "knowledge_document_id")
				}
				for field, targetTable := range spec.foreign {
					if row[field] == nil {
						continue
					}
					mapped, ok := idMaps[targetTable][backupIDKey(row[field])]
					if !ok {
						return apierror.New("BACKUP_REFERENCE_INVALID", "备份包含无效的资源关联", 422)
					}
					row[field] = mapped
				}
				if !spec.withoutID {
					delete(row, spec.id)
				}
				columns := make([]string, 0, len(row))
				for column := range row {
					columns = append(columns, column)
				}
				sort.Strings(columns)
				values := make([]interface{}, len(columns))
				holders := make([]string, len(columns))
				quoted := make([]string, len(columns))
				for index, column := range columns {
					values[index] = row[column]
					holders[index] = fmt.Sprintf("$%d", index+1)
					quoted[index] = pgx.Identifier{column}.Sanitize()
				}
				query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
					pgx.Identifier{spec.name}.Sanitize(), strings.Join(quoted, ","), strings.Join(holders, ","))
				if spec.withoutID {
					if _, err := tx.Exec(ctx, query, values...); err != nil {
						return fmt.Errorf("restore %s: %w", spec.name, err)
					}
					continue
				}
				query += " RETURNING " + pgx.Identifier{spec.id}.Sanitize()
				var newID interface{}
				if err := tx.QueryRow(ctx, query, values...).Scan(&newID); err != nil {
					return fmt.Errorf("restore %s: %w", spec.name, err)
				}
				idMaps[spec.name][oldID] = newID
			}
		}
		return audit(ctx, tx, principal, "backup.restore", 0)
	})
}

func cloneBackupRow(value map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}

func backupIDKey(value interface{}) string {
	switch typed := value.(type) {
	case json.Number:
		return typed.String()
	default:
		return fmt.Sprint(value)
	}
}

func backupTableBytes(rows []map[string]interface{}, field string) int64 {
	var total int64
	for _, row := range rows {
		value, err := strconv.ParseInt(fmt.Sprint(row[field]), 10, 64)
		if err != nil || value < 0 || total > int64(^uint64(0)>>1)-value {
			return int64(^uint64(0) >> 1)
		}
		total += value
	}
	return total
}
