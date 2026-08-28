package domain

import (
	"time"

	"github.com/google/uuid"
)

type Principal struct {
	UserID         int32
	Username       string
	TenantID       uuid.UUID
	TokenID        int32
	TenantActive   bool
	TokenVersion   int64
	TenantVersion  int64
	TokenExpiresAt time.Time
	AuthCacheKey   string
}

// TenantSummary is the application-facing view of a personal tenant. Keeping
// it in domain prevents HTTP and application layers from depending on the
// PostgreSQL adapter's concrete types.
type TenantSummary struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	Status               string `json:"status"`
	NoteQuota            int64  `json:"note_quota"`
	NoteCount            int64  `json:"note_count"`
	AttachmentQuotaBytes int64  `json:"attachment_quota_bytes"`
	AITokenQuota         int64  `json:"ai_token_quota"`
	AITokensUsed         int64  `json:"ai_tokens_used"`
}

type Tag struct {
	ID    int32   `json:"id"`
	Name  string  `json:"name"`
	Color *string `json:"color"`
}

type SearchFilter struct {
	Query     string
	Type      string
	StartDate *time.Time
	EndDate   *time.Time
	TagID     *int32
	Limit     int
}

type SearchItem struct {
	ID        int32   `json:"id"`
	Title     string  `json:"title"`
	Snippet   string  `json:"snippet"`
	Type      string  `json:"type"`
	NoteDate  *string `json:"note_date"`
	UpdatedAt string  `json:"updated_at"`
}

type DashboardRecent struct {
	ID        int32   `json:"id"`
	Title     string  `json:"title"`
	Type      string  `json:"type"`
	NoteDate  *string `json:"note_date"`
	UpdatedAt string  `json:"updated_at"`
	Summary   *string `json:"summary"`
}

type UserPreferences struct {
	TenantID                   string
	UserID                     int32
	Version                    int
	MarketplacePersonalization bool
}

type ExportNote struct {
	ID       int32
	Type     string
	Title    string
	Content  string
	NoteDate *string
	Summary  *string
}

type Attachment struct {
	ID             int32     `json:"id"`
	NoteID         int32     `json:"note_id"`
	OriginalName   string    `json:"original_name"`
	StoredPath     string    `json:"-"`
	StorageBackend string    `json:"-"`
	ObjectKey      string    `json:"-"`
	ObjectVersion  string    `json:"-"`
	ETag           string    `json:"-"`
	MIMEType       string    `json:"mime_type"`
	Size           int64     `json:"size"`
	SHA256         string    `json:"sha256"`
	CreatedAt      time.Time `json:"-"`
}

type AttachmentResponse struct {
	ID           int32  `json:"id"`
	NoteID       int32  `json:"note_id"`
	OriginalName string `json:"original_name"`
	MIMEType     string `json:"mime_type"`
	Size         int64  `json:"size"`
	SHA256       string `json:"sha256"`
	CreatedAt    string `json:"created_at"`
}

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

type ConversationMessage struct {
	ID        int32  `json:"id"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

type Conversation struct {
	ID           int32                 `json:"id"`
	Title        string                `json:"title"`
	SourceScope  string                `json:"source_scope"`
	CreatedAt    string                `json:"created_at"`
	UpdatedAt    string                `json:"updated_at"`
	Messages     []ConversationMessage `json:"messages,omitempty"`
	Version      int                   `json:"version"`
	MessageCount int                   `json:"message_count"`
	TotalTokens  int64                 `json:"total_tokens"`
	Summary      *string               `json:"summary,omitempty"`
}

func (a Attachment) Response() AttachmentResponse {
	return AttachmentResponse{ID: a.ID, NoteID: a.NoteID, OriginalName: a.OriginalName,
		MIMEType: a.MIMEType, Size: a.Size, SHA256: a.SHA256,
		CreatedAt: a.CreatedAt.Format(time.RFC3339Nano)}
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

type NoteFilter struct {
	Page      int
	PageSize  int
	Type      string
	StartDate *time.Time
	EndDate   *time.Time
	TagID     *int32
}

type NoteInput struct {
	Type     string
	Title    string
	Content  string
	NoteDate *time.Time
	Summary  *string
}

type NotePatch struct {
	Title             *string
	Content           *string
	NoteDate          *time.Time
	SetNoteDate       bool
	Summary           *string
	SetSummary        bool
	ExpectedUpdatedAt *time.Time
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
