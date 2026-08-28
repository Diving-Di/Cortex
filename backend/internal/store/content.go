package store

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"cortex/backend/internal/apierror"
	"cortex/backend/internal/domain"
	"github.com/jackc/pgx/v5"
)

func (s *Store) ListTags(ctx context.Context, principal domain.Principal) ([]domain.Tag, error) {
	var result []domain.Tag
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id,name,color FROM tags WHERE tenant_id=$1 ORDER BY name`, principal.TenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item domain.Tag
			if err := rows.Scan(&item.ID, &item.Name, &item.Color); err != nil {
				return err
			}
			result = append(result, item)
		}
		return rows.Err()
	})
	return result, err
}

func (s *Store) CreateTag(ctx context.Context, principal domain.Principal, name string, color *string) (domain.Tag, error) {
	var result domain.Tag
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `
            INSERT INTO tags (tenant_id,name,color) VALUES ($1,$2,$3)
            ON CONFLICT (tenant_id,name) DO UPDATE SET name=EXCLUDED.name
            RETURNING id,name,color`, principal.TenantID, name, color,
		).Scan(&result.ID, &result.Name, &result.Color)
		return err
	})
	return result, err
}

func (s *Store) ListNoteTags(ctx context.Context, principal domain.Principal, noteID int32) ([]domain.Tag, error) {
	var result []domain.Tag
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		if _, err := getNoteTx(ctx, tx, principal, noteID); err != nil {
			return err
		}
		var err error
		result, err = listNoteTagsTx(ctx, tx, principal, noteID)
		return err
	})
	return result, err
}

func (s *Store) AssignNoteTags(ctx context.Context, principal domain.Principal, noteID int32, tagIDs []int32) ([]domain.Tag, error) {
	var result []domain.Tag
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		if _, err := getNoteTx(ctx, tx, principal, noteID); err != nil {
			return err
		}
		unique := make(map[int32]struct{}, len(tagIDs))
		for _, id := range tagIDs {
			unique[id] = struct{}{}
		}
		if len(unique) > 0 {
			ids := make([]int32, 0, len(unique))
			for id := range unique {
				ids = append(ids, id)
			}
			var count int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM tags WHERE tenant_id=$1 AND id=ANY($2)`, principal.TenantID, ids).Scan(&count); err != nil {
				return err
			}
			if count != len(ids) {
				return apierror.New("TAG_NOT_FOUND", "标签不存在", 404)
			}
		}
		if _, err := tx.Exec(ctx, `DELETE FROM note_tags WHERE tenant_id=$1 AND note_id=$2`, principal.TenantID, noteID); err != nil {
			return err
		}
		for id := range unique {
			if _, err := tx.Exec(ctx, `INSERT INTO note_tags (tenant_id,note_id,tag_id) VALUES ($1,$2,$3)`, principal.TenantID, noteID, id); err != nil {
				return err
			}
		}
		var err error
		result, err = listNoteTagsTx(ctx, tx, principal, noteID)
		return err
	})
	return result, err
}

func listNoteTagsTx(ctx context.Context, tx pgx.Tx, principal domain.Principal, noteID int32) ([]domain.Tag, error) {
	rows, err := tx.Query(ctx, `
        SELECT t.id,t.name,t.color FROM tags t
        JOIN note_tags nt ON nt.tag_id=t.id AND nt.tenant_id=t.tenant_id
        WHERE nt.tenant_id=$1 AND nt.note_id=$2 ORDER BY t.name`, principal.TenantID, noteID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.Tag
	for rows.Next() {
		var item domain.Tag
		if err := rows.Scan(&item.ID, &item.Name, &item.Color); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) SearchNotes(ctx context.Context, principal domain.Principal, filter domain.SearchFilter) ([]domain.SearchItem, int64, error) {
	var items []domain.SearchItem
	var total int64
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		args := []any{principal.TenantID}
		where := []string{"n.tenant_id=$1", "n.deleted_at IS NULL"}
		if filter.Query != "" {
			args = append(args, "%"+filter.Query+"%")
			index := len(args)
			where = append(where, fmt.Sprintf("(n.title ILIKE $%d OR n.content ILIKE $%d OR n.summary ILIKE $%d)", index, index, index))
		}
		if filter.Type != "" {
			args = append(args, filter.Type)
			where = append(where, fmt.Sprintf("n.type=$%d", len(args)))
		}
		if filter.StartDate != nil {
			args = append(args, *filter.StartDate)
			where = append(where, fmt.Sprintf("n.note_date >= $%d", len(args)))
		}
		if filter.EndDate != nil {
			args = append(args, *filter.EndDate)
			where = append(where, fmt.Sprintf("n.note_date <= $%d", len(args)))
		}
		join := ""
		if filter.TagID != nil {
			join = " JOIN note_tags nt ON nt.note_id=n.id AND nt.tenant_id=n.tenant_id"
			args = append(args, *filter.TagID)
			where = append(where, fmt.Sprintf("nt.tag_id=$%d", len(args)))
		}
		base := " FROM notes n" + join + " WHERE " + strings.Join(where, " AND ")
		if err := tx.QueryRow(ctx, "SELECT count(*)"+base, args...).Scan(&total); err != nil {
			return err
		}
		args = append(args, filter.Limit)
		rows, err := tx.Query(ctx, `SELECT n.id,n.title,n.content,n.summary,n.type,n.note_date,n.updated_at`+
			base+fmt.Sprintf(" ORDER BY n.updated_at DESC LIMIT $%d", len(args)), args...,
		)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item domain.SearchItem
			var content string
			var summary *string
			var noteDate *time.Time
			var updated time.Time
			if err := rows.Scan(&item.ID, &item.Title, &content, &summary, &item.Type, &noteDate, &updated); err != nil {
				return err
			}
			source := content
			if source == "" && summary != nil {
				source = *summary
			}
			item.Snippet = searchSnippet(source, filter.Query)
			if noteDate != nil {
				value := noteDate.Format(time.DateOnly)
				item.NoteDate = &value
			}
			item.UpdatedAt = updated.Format(time.RFC3339Nano)
			items = append(items, item)
		}
		return rows.Err()
	})
	return items, total, err
}

func searchSnippet(source, query string) string {
	start := 0
	if query != "" {
		position := strings.Index(strings.ToLower(source), strings.ToLower(query))
		if position > 40 {
			start = position - 40
		}
	}
	runes := []rune(source)
	byteStart := len([]rune(source[:min(start, len(source))]))
	end := min(byteStart+160, len(runes))
	return string(runes[byteStart:end])
}

func (s *Store) Dashboard(ctx context.Context, principal domain.Principal, timezoneName string) (map[string]any, error) {
	zone, err := time.LoadLocation(timezoneName)
	if err != nil {
		return nil, apierror.New("INVALID_TIMEZONE", "无效的 IANA 时区", 422)
	}
	today := time.Now().In(zone)
	localDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, zone)
	startUTC := localDate.UTC()
	endUTC := localDate.AddDate(0, 0, 1).UTC()
	result := make(map[string]any)
	err = s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := setTenant(ctx, tx, principal); err != nil {
			return err
		}
		var todayCount, totalNotes, totalWords, aiRequests, aiTokens int64
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM notes WHERE tenant_id=$1 AND deleted_at IS NULL AND created_at >= $2 AND created_at < $3`, principal.TenantID, startUTC, endUTC).Scan(&todayCount); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*),COALESCE(sum(word_count),0) FROM notes WHERE tenant_id=$1 AND deleted_at IS NULL`, principal.TenantID).Scan(&totalNotes, &totalWords); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*),COALESCE(sum(input_tokens+output_tokens),0) FROM ai_usage_records WHERE tenant_id=$1`, principal.TenantID).Scan(&aiRequests, &aiTokens); err != nil {
			return err
		}
		recent, err := dashboardRecent(ctx, tx, principal)
		if err != nil {
			return err
		}
		activity, activeDays, err := dashboardActivity(ctx, tx, principal, timezoneName, localDate.AddDate(0, 0, -364).UTC())
		if err != nil {
			return err
		}
		pending, err := pendingReports(ctx, tx, principal, localDate)
		if err != nil {
			return err
		}
		result = map[string]any{
			"date":        localDate.Format(time.DateOnly),
			"timezone":    timezoneName,
			"today":       map[string]int64{"new_notes": todayCount},
			"streak_days": streakDays(activeDays, localDate.Format(time.DateOnly)),
			"statistics": map[string]int64{
				"notes": totalNotes, "words": totalWords,
				"ai_requests": aiRequests, "ai_tokens": aiTokens,
			},
			"recent_notes":    recent,
			"activity":        activity,
			"pending_reports": pending,
		}
		return nil
	})
	return result, err
}

func dashboardRecent(ctx context.Context, tx pgx.Tx, principal domain.Principal) ([]domain.DashboardRecent, error) {
	rows, err := tx.Query(ctx, `SELECT id,title,type,note_date,updated_at,summary FROM notes
        WHERE tenant_id=$1 AND deleted_at IS NULL ORDER BY updated_at DESC,id DESC LIMIT 6`, principal.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]domain.DashboardRecent, 0)
	for rows.Next() {
		var item domain.DashboardRecent
		var date *time.Time
		var updated time.Time
		if err := rows.Scan(&item.ID, &item.Title, &item.Type, &date, &updated, &item.Summary); err != nil {
			return nil, err
		}
		if date != nil {
			value := date.Format(time.DateOnly)
			item.NoteDate = &value
		}
		item.UpdatedAt = updated.Format(time.RFC3339Nano)
		result = append(result, item)
	}
	return result, rows.Err()
}

func dashboardActivity(ctx context.Context, tx pgx.Tx, principal domain.Principal, zone string, since time.Time) ([]map[string]any, map[string]bool, error) {
	rows, err := tx.Query(ctx, `SELECT (created_at AT TIME ZONE $2)::date,count(*) FROM notes
        WHERE tenant_id=$1 AND deleted_at IS NULL AND created_at >= $3 GROUP BY 1 ORDER BY 1`,
		principal.TenantID, zone, since,
	)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	result := make([]map[string]any, 0)
	active := make(map[string]bool)
	for rows.Next() {
		var day time.Time
		var count int64
		if err := rows.Scan(&day, &count); err != nil {
			return nil, nil, err
		}
		value := day.Format(time.DateOnly)
		active[value] = true
		result = append(result, map[string]any{"date": value, "count": count})
	}
	return result, active, rows.Err()
}

func streakDays(active map[string]bool, today string) int {
	current, _ := time.Parse(time.DateOnly, today)
	if !active[today] {
		current = current.AddDate(0, 0, -1)
	}
	streak := 0
	for active[current.Format(time.DateOnly)] {
		streak++
		current = current.AddDate(0, 0, -1)
	}
	return streak
}

func pendingReports(ctx context.Context, tx pgx.Tx, principal domain.Principal, today time.Time) ([]map[string]string, error) {
	labels := map[string]string{"daily": "日报", "weekly": "周报", "monthly": "月报"}
	kinds := []string{"daily", "weekly", "monthly"}
	result := make([]map[string]string, 0)
	for _, kind := range kinds {
		start, end := periodRange(kind, today)
		if end.After(today) {
			end = today
		}
		var reportExists, sourceExists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM notes WHERE tenant_id=$1 AND deleted_at IS NULL AND type=$2 AND note_date=$3)`, principal.TenantID, kind, start).Scan(&reportExists); err != nil {
			return nil, err
		}
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM notes WHERE tenant_id=$1 AND deleted_at IS NULL AND note_date >= $2 AND note_date <= $3 AND type<>$4)`, principal.TenantID, start, end, kind).Scan(&sourceExists); err != nil {
			return nil, err
		}
		if sourceExists && !reportExists {
			result = append(result, map[string]string{
				"type": kind, "label": labels[kind],
				"anchor_date":  today.Format(time.DateOnly),
				"period_start": start.Format(time.DateOnly),
			})
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return i < j })
	return result, nil
}

func periodRange(kind string, value time.Time) (time.Time, time.Time) {
	day := time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, value.Location())
	switch kind {
	case "weekly":
		offset := (int(day.Weekday()) + 6) % 7
		start := day.AddDate(0, 0, -offset)
		return start, start.AddDate(0, 0, 6)
	case "monthly":
		start := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, day.Location())
		return start, start.AddDate(0, 1, -1)
	default:
		return day, day
	}
}
