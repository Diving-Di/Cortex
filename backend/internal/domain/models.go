package domain

import (
    "time"

    "github.com/google/uuid"
)

type Principal struct {
    UserID       int32
    Username     string
    TenantID     uuid.UUID
    TokenID      int32
    TenantActive bool
}

type Note struct {
    ID        int32      `json:"id"`
    Type      string     `json:"type"`
    Title     string     `json:"title"`
    Content   string     `json:"content"`
    NoteDate  *time.Time `json:"-"`
    Summary   *string    `json:"summary"`
    WordCount int32      `json:"word_count"`
    CreatedAt time.Time  `json:"created_at"`
    UpdatedAt time.Time  `json:"updated_at"`
}

type NoteResponse struct {
    ID        int32   `json:"id"`
    Type      string  `json:"type"`
    Title     string  `json:"title"`
    Content   string  `json:"content"`
    NoteDate  *string `json:"note_date"`
    Summary   *string `json:"summary"`
    WordCount int32   `json:"word_count"`
    CreatedAt string  `json:"created_at"`
    UpdatedAt string  `json:"updated_at"`
}

func (n Note) Response() NoteResponse {
    var noteDate *string
    if n.NoteDate != nil {
        value := n.NoteDate.Format(time.DateOnly)
        noteDate = &value
    }
    return NoteResponse{
        ID: n.ID, Type: n.Type, Title: n.Title, Content: n.Content,
        NoteDate: noteDate, Summary: n.Summary, WordCount: n.WordCount,
        CreatedAt: n.CreatedAt.Format(time.RFC3339Nano),
        UpdatedAt: n.UpdatedAt.Format(time.RFC3339Nano),
    }
}

type Revision struct {
    ID        int32     `json:"id"`
    NoteID    int32     `json:"note_id"`
    Content   string    `json:"content"`
    Reason    string    `json:"reason"`
    CreatedAt time.Time `json:"-"`
}

func (r Revision) Response() map[string]any {
    return map[string]any{
        "id": r.ID, "note_id": r.NoteID, "content": r.Content,
        "reason": r.Reason, "created_at": r.CreatedAt.Format(time.RFC3339Nano),
    }
}
