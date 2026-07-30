package recipe

import "time"

// RecipeDocument represents a system recipe or tip.
type RecipeDocument struct {
	ID              int64     `db:"id"`
	SourcePath      string    `db:"source_path"`
	Kind            string    `db:"kind"`
	Category        string    `db:"category"`
	Title           string    `db:"title"`
	Summary         string    `db:"summary"`
	Ingredients     []string  `db:"ingredients"`
	DietaryTerms    []string  `db:"dietary_terms"`
	Difficulty      *string   `db:"difficulty"`
	CaloriesText    *string   `db:"calories_text"`
	ContentMarkdown string    `db:"content_markdown"`
	ContentSHA256   string    `db:"content_sha256"`
	SourceRevision  *string   `db:"source_revision"`
	IsActive        bool      `db:"is_active"`
	CreatedAt       time.Time `db:"created_at"`
	UpdatedAt       time.Time `db:"updated_at"`
}

// ParentChunk represents a higher-level section within a recipe.
type ParentChunk struct {
	ID         int64
	DocumentID int64
	Index      int
	Heading    string
	Content    string
	TokenCount int
}

// ChildChunk is a searchable block with embedding metadata.
type ChildChunk struct {
	ID            int64
	DocumentID    int64
	ParentID      int64
	IndexVersion  int
	ChildIndex    int
	HeadingPath   string
	Content       string
	EmbeddingText string
	ContentHash   string
	TokenCount    int
}
